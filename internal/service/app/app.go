package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"log/slog"
	"moonbridge/internal/config"
	"moonbridge/internal/db"
	"moonbridge/internal/format"
	"moonbridge/internal/logger"
	"moonbridge/internal/protocol/anthropic"
	"moonbridge/internal/protocol/cache"
	"moonbridge/internal/protocol/chat"
	"moonbridge/internal/protocol/google"
	"moonbridge/internal/protocol/openai"
	"moonbridge/internal/service/provider"
	"moonbridge/internal/service/proxy"
	"moonbridge/internal/service/runtime"
	"moonbridge/internal/service/server"
	"moonbridge/internal/service/server/session"
	"moonbridge/internal/service/server/trace"
	"moonbridge/internal/service/server/usage"
	"moonbridge/internal/service/stats"
	"moonbridge/internal/service/store"
	mbtrace "moonbridge/internal/service/trace"
)

const Name = "Moon Bridge"

func Run(output io.Writer) {
	fmt.Fprintln(output, WelcomeMessage())
}

func WelcomeMessage() string {
	return "Bienvenue sur " + Name + " !"
}

func RunServer(ctx context.Context, cfg config.Config, errors io.Writer) error {
	switch cfg.Mode {
	case config.ModeTransform:
		slog.Info("Démarrage du serveur", "mode", cfg.Mode, "addr", cfg.Addr)
		return runTransform(ctx, cfg, errors)
	case config.ModeCaptureResponse:
		slog.Info("Démarrage du serveur", "mode", cfg.Mode, "addr", cfg.Addr)
		return runCaptureResponse(ctx, cfg, errors)
	case config.ModeCaptureAnthropic:
		slog.Info("Démarrage du serveur", "mode", cfg.Mode, "addr", cfg.Addr)
		return runCaptureAnthropic(ctx, cfg, errors)
	default:
		return fmt.Errorf("unsupported mode %q", cfg.Mode)
	}
}

func runTransform(ctx context.Context, cfg config.Config, errors io.Writer) error {
	var rt *runtime.Runtime

	// Construct domain configs from global config.
	serverCfg := config.ServerFromGlobalConfig(&cfg)
	cacheCfg := config.CacheFromGlobalConfig(&cfg)
	proxyCfg := config.ProxyFromGlobalConfig(&cfg)
	storeCfg := config.StoreFromGlobalConfig(&cfg)
	persistCfg := config.PersistenceFromGlobalConfig(&cfg)
	providerCfg := config.ProviderFromGlobalConfig(&cfg)
	_ = persistCfg // used in db init
	_ = storeCfg   // used in config store
	_ = proxyCfg   // used in proxy mode

	// === Phase 1: Bootstrap from YAML ===

	// Build multi-provider infrastructure from YAML config.
	providerDefs := provider.BuildProviderDefsFromConfig(providerCfg)
	modelRoutes := provider.BuildModelRoutesFromConfig(providerCfg)
	providerMgr, err := provider.NewProviderManager(providerDefs, modelRoutes)
	if err != nil {
		return fmt.Errorf("init provider manager: %w", err)
	}

	// Resolve a fallback client for web search probing and server fallback.
	defaultClient := resolveDefaultClient(providerMgr, errors)
	resolvePerProviderWebSearch(ctx, cfg, providerMgr, errors)

	sessionStats := stats.NewSessionStats()
	pricing := provider.BuildPricingFromConfig(providerCfg)
	if len(pricing) > 0 {
		sessionStats.SetPricing(pricing)
	}

	tracer := mbtrace.New(mbtrace.Config{
		Enabled: cfg.TraceRequests,
		Root:    transformTraceRoot(),
	})
	logTrace(errors, "transform", tracer)

	// Determine the default provider to use as the fallback Provider.
	var fallbackProvider provider.ProviderClient
	if defaultClient != nil {
		fallbackProvider = provider.NewAnthropicClientAdapter(defaultClient)
	}

	// Register plugins.
	plugins := BuiltinExtensions().NewRegistry(slog.Default(), cfg)
	plugins.SetCurrentConfigProvider(func() config.Config {
		if rt != nil && rt.Current() != nil {
			return rt.Current().Config
		}
		return cfg
	})
	if err := plugins.InitAll(&cfg); err != nil {
		return fmt.Errorf("init plugins: %w", err)
	}
	defer plugins.ShutdownAll()

	// Wire plugin LogConsumer into the slog consume pipeline.
	logger.SetConsumeFunc(func(entries []logger.LogEntry) []logger.LogEntry {
		return plugins.ConsumeGlobalLog(entries)
	})

	// Initialize persistence layer (db.Registry).
	dbRegistry := db.NewRegistry(slog.Default())
	dbProviders := plugins.DBProviders()
	providers := make([]db.Provider, 0, len(dbProviders))
	for _, p := range dbProviders {
		if prov := p.DBProvider(); prov != nil {
			dbRegistry.RegisterProvider(prov)
			providers = append(providers, prov)
		}
	}
	for _, c := range plugins.DBConsumers() {
		if cons := c.DBConsumer(); cons != nil {
			dbRegistry.RegisterConsumer(cons)
		}
	}
	// Register the config_store consumer for configuration persistence.
	configStoreConsumer := store.NewConfigStoreConsumer(logger.L())
	configStoreConsumer.SetExtensionSpecs(BuiltinExtensions().ConfigSpecs())
	dbRegistry.RegisterConsumer(configStoreConsumer)
	activePersistenceProvider := ResolvePersistenceActiveProvider(cfg.Persistence.ActiveProvider, providers)
	if err := dbRegistry.Init(ctx, activePersistenceProvider); err != nil {
		return fmt.Errorf("init persistence: %w", err)
	}
	defer dbRegistry.Shutdown()

	// === Phase 2: ConfigStore bootstrap ===
	// Check if the store is available and has existing data.
	cs := configStoreConsumer.Store()
	if cs != nil {
		if dbCfg, loadErr := cs.LoadAll(); loadErr == nil {
			if len(dbCfg.ProviderDefs) > 0 || len(dbCfg.Routes) > 0 {
				// DB has existing configuration: use it as the active config.
				logger.Info("Chargement de la configuration depuis le stockage persistant",
					"providers", len(dbCfg.ProviderDefs),
					"routes", len(dbCfg.Routes))
				cfg = *dbCfg
				dbProviderCfg := config.ProviderFromGlobalConfig(&cfg)

				// Rebuild provider manager and pricing from DB-loaded config.
				providerDefs = provider.BuildProviderDefsFromConfig(dbProviderCfg)
				modelRoutes = provider.BuildModelRoutesFromConfig(dbProviderCfg)
				providerMgr, err = provider.NewProviderManager(providerDefs, modelRoutes)
				if err != nil {
					return fmt.Errorf("rebuild provider manager from DB: %w", err)
				}
				_ = resolveDefaultClient(providerMgr, errors)
				resolvePerProviderWebSearch(ctx, cfg, providerMgr, errors)

				pricing = provider.BuildPricingFromConfig(dbProviderCfg)
				if len(pricing) > 0 {
					sessionStats.SetPricing(pricing)
				}
				serverCfg = config.ServerFromGlobalConfig(&cfg)
			} else {
				// DB is empty: seed from YAML config.
				logger.Info("Stockage persistant vide, importation de la configuration initiale depuis YAML")
				if err := cs.SeedFromConfig(&cfg); err != nil {
					logger.Warn("Échec de l'importation initiale dans config store", "error", err)
				}
			}
		} else if loadErr != nil {
			if strings.Contains(loadErr.Error(), "config not seeded") {
				logger.Info("persistence store is empty, skipping DB config load")
			} else {
				logger.Warn("Échec du chargement config store", "error", loadErr)
			}
		}
	} else {
		logger.Warn("config store indisponible, contournement de l'initialisation persistante")
	}

	// === Phase 3: Build Runtime ===
	rt = runtime.NewRuntime(cfg, providerMgr, pricing)

	// === Phase 4: Build Server with Runtime ===
	// Create shared cache registry (used by both Bridge and Adapter paths).
	cacheReg := cache.NewMemoryRegistry()

	// Optionally create the experimental adapter registry.
	// Create the adapter registry for Core format dispatch.
	adapterReg := format.NewRegistry()
	coreHooks := plugins.CorePluginHooks()

	// Inbound: OpenAI Responses client adapter.
	oaiAdapter := openai.NewOpenAIAdapter(coreHooks)
	_ = adapterReg.RegisterClient(oaiAdapter)
	_ = adapterReg.RegisterClientStream(oaiAdapter)

	// Upstream: Anthropic provider adapter with cache manager.
	cacheMgr := anthropic.NewCacheManager(&cfg.Cache, cacheReg)
	anthAdapter := anthropic.NewAnthropicProviderAdapter(cfg.DefaultMaxTokens, cacheMgr, coreHooks)
	_ = adapterReg.RegisterProvider(anthAdapter)
	_ = adapterReg.RegisterProviderStream(anthAdapter)

	// Upstream: Google GenAI provider adapter.
	googleCfg := &cache.PlanCacheConfig{
		Mode:                     cacheCfg.Mode,
		TTL:                      cacheCfg.TTL,
		PromptCaching:            cacheCfg.PromptCaching,
		AutomaticPromptCache:     cacheCfg.AutomaticPromptCache,
		ExplicitCacheBreakpoints: cacheCfg.ExplicitCacheBreakpoints,
		AllowRetentionDowngrade:  cacheCfg.AllowRetentionDowngrade,
		MaxBreakpoints:           cacheCfg.MaxBreakpoints,
		MinCacheTokens:           cacheCfg.MinCacheTokens,
		ExpectedReuse:            cacheCfg.ExpectedReuse,
		MinimumValueScore:        cacheCfg.MinimumValueScore,
		MinBreakpointTokens:      cacheCfg.MinBreakpointTokens,
	}
	googleAdapter := google.NewGeminiProviderAdapter(cfg.DefaultMaxTokens, nil, coreHooks, googleCfg, cacheReg)
	_ = adapterReg.RegisterProvider(googleAdapter)
	_ = adapterReg.RegisterProviderStream(googleAdapter)

	// Upstream: OpenAI Chat provider adapter.
	chatAdapter := chat.NewChatProviderAdapter(cfg.DefaultMaxTokens, nil, coreHooks)
	_ = adapterReg.RegisterProvider(chatAdapter)
	_ = adapterReg.RegisterProviderStream(chatAdapter)

	slog.Info("Adapter dispatch path enabled", "registry", "format.Registry")

	// Build protocol-specific HTTP clients from provider configs.
	chatClients := make(map[string]any, len(cfg.ProviderDefs))
	googleClients := make(map[string]any, len(cfg.ProviderDefs))
	for key, def := range cfg.ProviderDefs {
		switch def.Protocol {
		case config.ProtocolOpenAIChat:
			chatClients[key] = chat.NewClient(chat.ClientConfig{
				BaseURL:   def.BaseURL,
				APIKey:    def.APIKey,
				UserAgent: def.UserAgent,
			})
			slog.Debug("chat client created", "provider", key)
		case config.ProtocolGoogleGenAI:
			googleClients[key] = google.NewClient(google.ClientConfig{
				BaseURL:   def.BaseURL,
				APIKey:    def.APIKey,
				Project:   def.Project,
				Location:  def.Location,
				Version:   def.APIVersion,
				UserAgent: def.UserAgent,
			})
			slog.Debug("google client created", "provider", key)
		}
	}

	// Create sub-package managers for session, usage, and trace.
	sessMgr := session.NewInMemoryManager(server.NewSessionConfigAdapterFromRuntime(rt, serverCfg), plugins)
	usageTrk := usage.NewStatsTracker(sessionStats)
	traceWtr := trace.NewFileWriter(tracer, errors)

	handler := server.New(server.Config{
		ServerCfg:       serverCfg,
		Provider:        fallbackProvider,
		ProviderMgr:     providerMgr,
		ChatClients:     chatClients,
		GoogleClients:   googleClients,
		Tracer:          tracer,
		TraceErrors:     errors,
		Stats:           sessionStats,
		PluginRegistry:  plugins,
		AppConfig:       serverCfg,
		Runtime:         rt,
		AdapterRegistry: adapterReg,
		SessionManager:  sessMgr,
		UsageTracker:    usageTrk,
		TraceWriter:     traceWtr,
	})

	wrapped := handler
	return runHTTPServer(ctx, cfg.Addr, wrapped, errors, sessionStats)
}

// resolveDefaultClient returns the provider client for the default key.
// Returns nil when no default provider is configured (all models use explicit routing).
func resolveDefaultClient(pm *provider.ProviderManager, errors io.Writer) *anthropic.Client {
	if pm.DefaultKey() == "" {
		slog.Warn("Aucun fournisseur par défaut configuré, sondage web search et fallback serveur ignorés")
		return nil
	}
	client, err := pm.ClientForKey(pm.DefaultKey())
	if err != nil {
		slog.Warn("Client fournisseur par défaut indisponible", "error", err)
		return nil
	}
	if acc, ok := client.(provider.AnthropicClientAccessor); ok {
		return acc.AnthropicClient()
	}
	slog.Warn("Le client fournisseur par défaut ne permet pas d'accéder au client sous-jacent")
	return nil
}

// webSearchProber interface and following functions are unchanged.
type webSearchProber interface {
	ProbeWebSearch(context.Context, string) (bool, error)
}

type webSearchCandidateProber interface {
	ProbeWebSearchCandidate(context.Context, string, string) (bool, error)
}

// resolvePerProviderWebSearch resolves web_search support for each provider and
// each model that has a model-level override.
func resolvePerProviderWebSearch(ctx context.Context, cfg config.Config, pm *provider.ProviderManager, errors io.Writer) {
	if pm == nil {
		return
	}
	// 1. Resolve provider-level defaults.
	for _, key := range pm.ProviderKeys() {
		protocol := pm.ProtocolForKey(key)
		support := cfg.WebSearchForProvider(key)
		switch protocol {
		case config.ProtocolAnthropic:
			switch support {
			case config.WebSearchSupportDisabled:
				pm.SetResolvedWebSearch(key, "disabled")
				slog.Info("Web search désactivé par configuration", "provider", key)
			case config.WebSearchSupportEnabled:
				pm.SetResolvedWebSearch(key, "enabled")
				slog.Info("Web search forcé par configuration", "provider", key)
			case config.WebSearchSupportInjected:
				pm.SetResolvedWebSearch(key, "injected")
				slog.Info("Mode injection web search activé", "provider", key)
			default:
				resolved := probeProviderWebSearch(ctx, key, pm, errors)
				if resolved == "disabled" && cfg.TavilyAPIKey != "" {
					resolved = "injected"
					slog.Info("Sondage web search automatique échoué, repli sur mode injection", "provider", key)
				}
				pm.SetResolvedWebSearch(key, resolved)
			}
		case config.ProtocolOpenAIResponse:
			switch support {
			case config.WebSearchSupportDisabled, config.WebSearchSupportInjected:
				pm.SetResolvedWebSearch(key, "disabled")
				slog.Info("Web search côté réponse désactivé", "provider", key, "protocol", protocol, "config", support)
			default:
				pm.SetResolvedWebSearch(key, "enabled")
				slog.Info("Web search côté réponse activé", "provider", key, "protocol", protocol)
			}
		default:
			// openai-chat and google-genai have no native web_search, enable injection mode with API key
			if cfg.TavilyAPIKey != "" {
				pm.SetResolvedWebSearch(key, "injected")
				slog.Info("Web search par injection activé", "provider", key, "protocol", protocol)
			} else {
				pm.SetResolvedWebSearch(key, "disabled")
				slog.Info("Web search ignoré : pas de clé API Tavily", "provider", key, "protocol", protocol)
			}
		}
	}
	// 2. Resolve model-level overrides for provider catalog slugs and route aliases.
	for providerKey, def := range cfg.ProviderDefs {
		for modelName := range def.Models {
			alias := providerKey + "/" + modelName
			newAlias := modelName + "(" + providerKey + ")"
			modelWS := cfg.WebSearchForModel(alias)
			resolveModelWebSearch(ctx, alias, providerKey, modelName, modelWS, pm, cfg, errors)
			resolveModelWebSearch(ctx, newAlias, providerKey, modelName, modelWS, pm, cfg, errors)
			pureWS := cfg.WebSearchForModel(modelName)
			resolveModelWebSearch(ctx, modelName, providerKey, modelName, pureWS, pm, cfg, errors)
		}
	}
	for alias, route := range cfg.Routes {
		modelWS := cfg.WebSearchForModel(alias)
		providerKey := route.Provider
		if providerKey == "" {
			providerKey = pm.DefaultKey()
		}
		resolveModelWebSearch(ctx, alias, providerKey, route.Model, modelWS, pm, cfg, errors)
	}
}

func resolveModelWebSearch(ctx context.Context, alias, providerKey, upstreamModel string, modelWS config.WebSearchSupport, pm *provider.ProviderManager, cfg config.Config, errors io.Writer) {
	if alias == "" || providerKey == "" || upstreamModel == "" {
		return
	}
	modelKey := "model:" + alias
	candidateKey := provider.WebSearchCandidateKey(providerKey, upstreamModel)
	protocol := pm.ProtocolForModel(alias)
	switch protocol {
	case config.ProtocolAnthropic:
	case config.ProtocolOpenAIResponse:
		switch modelWS {
		case config.WebSearchSupportDisabled, config.WebSearchSupportInjected:
			pm.SetResolvedWebSearch(modelKey, "disabled")
			pm.SetResolvedWebSearch(candidateKey, "disabled")
			slog.Info("Web search côté réponse désactivé pour le modèle", "model", alias, "config", modelWS)
		default:
			pm.SetResolvedWebSearch(modelKey, "enabled")
			pm.SetResolvedWebSearch(candidateKey, "enabled")
			slog.Info("Web search côté réponse activé pour le modèle", "model", alias)
		}
		return
	default:
		pm.SetResolvedWebSearch(modelKey, "disabled")
		pm.SetResolvedWebSearch(candidateKey, "disabled")
		slog.Info("Web search niveau modèle ignoré : protocole non supporté", "model", alias, "protocol", protocol)
		return
	}
	switch modelWS {
	case config.WebSearchSupportDisabled:
		pm.SetResolvedWebSearch(modelKey, "disabled")
		pm.SetResolvedWebSearch(candidateKey, "disabled")
		slog.Info("Web search désactivé par configuration du modèle", "model", alias)
	case config.WebSearchSupportEnabled:
		pm.SetResolvedWebSearch(modelKey, "enabled")
		pm.SetResolvedWebSearch(candidateKey, "enabled")
		slog.Info("Web search forcé par configuration du modèle", "model", alias)
	case config.WebSearchSupportInjected:
		pm.SetResolvedWebSearch(modelKey, "injected")
		pm.SetResolvedWebSearch(candidateKey, "injected")
		slog.Info("Mode injection web search activé par configuration du modèle", "model", alias)
	default:
		resolved := resolveModelWebSearchWithProber(ctx, alias, providerKey, upstreamModel, modelWS, pm, cfg, errors, pm)
		pm.SetResolvedWebSearch(modelKey, resolved)
		pm.SetResolvedWebSearch(candidateKey, resolved)
	}
}

func probeProviderWebSearch(ctx context.Context, key string, pm *provider.ProviderManager, errors io.Writer) string {
	pc, err := pm.ClientForKey(key)
	if err != nil {
		slog.Warn("Sondage web search ignoré : client indisponible", "provider", key, "error", err)
		return "disabled"
	}

	upstreamModel := pm.FirstUpstreamModelForKey(key)
	if upstreamModel == "" {
		slog.Warn("Sondage web search automatique ignoré : aucun modèle routé vers le fournisseur", "provider", key)
		return "disabled"
	}

	acc, ok := pc.(provider.AnthropicClientAccessor)
	if !ok {
		slog.Warn("Sondage web search ignoré : client ne supporte pas l'accès", "provider", key)
		return "disabled"
	}
	client := acc.AnthropicClient()
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	supported, err := client.ProbeWebSearch(probeCtx, upstreamModel)
	if err != nil {
		slog.Warn("Sondage web search automatique échoué", "provider", key, "error", err)
		fmt.Fprintf(errors, "Sondage web search automatique échoué (fournisseur %s): %v\n", key, err)
		return "disabled"
	}
	if !supported {
		slog.Warn("Le fournisseur ne supporte pas le web search", "provider", key, "model", upstreamModel)
		fmt.Fprintf(errors, "Le fournisseur %s ne supporte pas le web search\n", key)
		return "disabled"
	}
	slog.Info("Le fournisseur supporte le web search", "provider", key, "model", upstreamModel)
	return "enabled"
}

func probeModelWebSearch(ctx context.Context, modelAlias string, pm *provider.ProviderManager, errors io.Writer) string {
	upstreamModel, pc, err := pm.ClientFor(modelAlias)
	if err != nil {
		slog.Warn("Sondage web search modèle ignoré : client indisponible", "model", modelAlias, "error", err)
		return "disabled"
	}
	acc, ok := pc.(provider.AnthropicClientAccessor)
	if !ok {
		slog.Warn("Sondage web search modèle ignoré : client ne supporte pas l'accès", "model", modelAlias)
		return "disabled"
	}
	client := acc.AnthropicClient()
	if err != nil {
		slog.Warn("Sondage web search modèle ignoré : client indisponible", "model", modelAlias, "error", err)
		return "disabled"
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	supported, err := client.ProbeWebSearch(probeCtx, upstreamModel)
	if err != nil {
		slog.Warn("Sondage web search modèle échoué", "model", modelAlias, "error", err)
		fmt.Fprintf(errors, "Sondage web search modèle échoué (%s): %v\n", modelAlias, err)
		return "disabled"
	}
	if !supported {
		slog.Warn("Le modèle ne supporte pas le web search", "model", modelAlias)
		fmt.Fprintf(errors, "Le modèle %s ne supporte pas le web search\n", modelAlias)
		return "disabled"
	}
	slog.Info("Le modèle supporte le web search", "model", modelAlias)
	return "enabled"
}

func probeModelWebSearchCandidate(ctx context.Context, modelAlias, providerKey, upstreamModel string, pm *provider.ProviderManager, cfg config.Config, errors io.Writer) string {
	return resolveModelWebSearchWithProber(ctx, modelAlias, providerKey, upstreamModel, config.WebSearchSupportAuto, pm, cfg, errors, pm)
}

func resolveModelWebSearchWithProber(ctx context.Context, modelAlias, providerKey, upstreamModel string, modelWS config.WebSearchSupport, pm *provider.ProviderManager, cfg config.Config, errors io.Writer, prober webSearchCandidateProber) string {
	switch modelWS {
	case config.WebSearchSupportDisabled:
		return "disabled"
	case config.WebSearchSupportEnabled:
		return "enabled"
	case config.WebSearchSupportInjected:
		return "injected"
	}
	if prober == nil {
		if injectedSearchConfigured(cfg, modelAlias, providerKey) {
			return "injected"
		}
		return "disabled"
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	supported, err := prober.ProbeWebSearchCandidate(probeCtx, providerKey, upstreamModel)
	if err != nil {
		slog.Warn("Sondage web search modèle échoué", "model", modelAlias, "provider", providerKey, "upstream_model", upstreamModel, "error", err)
		fmt.Fprintf(errors, "Sondage web search modèle échoué (%s via %s/%s): %v\n", modelAlias, providerKey, upstreamModel, err)
		if injectedSearchConfigured(cfg, modelAlias, providerKey) {
			slog.Info("Sondage web search modèle échoué, repli sur mode injection", "model", modelAlias, "provider", providerKey, "upstream_model", upstreamModel)
			return "injected"
		}
		return "disabled"
	}
	if supported {
		slog.Info("Le modèle supporte le web search", "model", modelAlias, "provider", providerKey, "upstream_model", upstreamModel)
		return "enabled"
	}
	if injectedSearchConfigured(cfg, modelAlias, providerKey) {
		slog.Info("Le modèle ne supporte pas le web search natif, repli sur mode injection", "model", modelAlias, "provider", providerKey, "upstream_model", upstreamModel)
		return "injected"
	}
	slog.Warn("Le modèle ne supporte pas le web search", "model", modelAlias, "provider", providerKey, "upstream_model", upstreamModel)
	fmt.Fprintf(errors, "Le modèle %s (%s/%s) ne supporte pas le web search\n", modelAlias, providerKey, upstreamModel)
	return "disabled"
}

func injectedSearchConfigured(cfg config.Config, modelAlias, providerKey string) bool {
	if cfg.WebSearchTavilyKeyForModel(modelAlias) != "" || cfg.WebSearchFirecrawlKeyForModel(modelAlias) != "" {
		return true
	}
	if providerKey == "" {
		return false
	}
	return cfg.WebSearchTavilyKeyForProvider(providerKey) != "" || cfg.WebSearchFirecrawlKeyForProvider(providerKey) != ""
}

func runCaptureResponse(ctx context.Context, cfg config.Config, errors io.Writer) error {
	tracer := mbtrace.New(captureResponseTraceConfig(cfg.TraceRequests))
	logTrace(errors, "response proxy", tracer)
	handler, err := proxy.NewResponse(proxy.ResponseConfig{
		UpstreamBaseURL: cfg.ResponseProxy.ProviderBaseURL,
		APIKey:          cfg.ResponseProxy.ProviderAPIKey,
		Tracer:          tracer,
		TraceErrors:     errors,
	})
	if err != nil {
		return err
	}
	slog.Info("Proxy de réponse initialisé", "upstream", cfg.ResponseProxy.ProviderBaseURL)
	return runHTTPServer(ctx, cfg.Addr, handler, errors, nil)
}

func runCaptureAnthropic(ctx context.Context, cfg config.Config, errors io.Writer) error {
	tracer := mbtrace.New(captureAnthropicTraceConfig(cfg.TraceRequests))
	logTrace(errors, "anthropic proxy", tracer)
	handler, err := proxy.NewAnthropic(proxy.AnthropicConfig{
		UpstreamBaseURL: cfg.AnthropicProxy.ProviderBaseURL,
		APIKey:          cfg.AnthropicProxy.ProviderAPIKey,
		Version:         cfg.AnthropicProxy.ProviderVersion,
		Tracer:          tracer,
		TraceErrors:     errors,
	})
	if err != nil {
		return err
	}
	slog.Info("Proxy Anthropic initialisé", "upstream", cfg.AnthropicProxy.ProviderBaseURL)
	return runHTTPServer(ctx, cfg.Addr, handler, errors, nil)
}

func logTrace(errors io.Writer, label string, tracer *mbtrace.Tracer) {
	if !tracer.Enabled() {
		fmt.Fprintf(errors, "Traçage %s désactivé\n", label)
		return
	}
	slog.Info("Traçage activé", "label", label, "dir", tracer.Directory())
	fmt.Fprintf(errors, "Traçage %s activé dans %s\n", label, tracer.Directory())
}

func transformTraceRoot() string {
	return filepath.Join(mbtrace.DefaultRoot, "Transform")
}

func captureResponseTraceConfig(enabled bool) mbtrace.Config {
	return mbtrace.Config{
		Enabled: enabled,
		Root:    filepath.Join(mbtrace.DefaultRoot, "Capture", "Response"),
	}
}

func captureAnthropicTraceConfig(enabled bool) mbtrace.Config {
	return mbtrace.Config{
		Enabled: enabled,
		Root:    filepath.Join(mbtrace.DefaultRoot, "Capture", "Anthropic"),
	}
}

func runHTTPServer(ctx context.Context, addr string, handler http.Handler, errors io.Writer, sessionStats *stats.SessionStats) error {
	httpServer := &http.Server{Addr: addr, Handler: handler}
	defer func() {
		if closer, ok := handler.(io.Closer); ok {
			_ = closer.Close()
		}
	}()
	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(errors, "%s écoute sur %s\n", Name, addr)
		slog.Info("Serveur HTTP en écoute", "addr", addr)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		if sessionStats != nil {
			summary := sessionStats.Summary()
			slog.Info(stats.FormatSummaryLine(summary))
			fmt.Fprintln(errors)
			stats.WriteSummary(errors, summary)
		}
		shutdownCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		slog.Error("Erreur du serveur HTTP", "error", err)
		return err
	}
}

// DumpConfigSchema dumps JSON Schema files alongside the config file,
// including known plugin config types. Call via --dump-config-schema flag.
func DumpConfigSchema(configPath string) error {
	return config.DumpConfigSchemaWithOptions(configPath, config.SchemaOptions{
		ExtensionSpecs: BuiltinExtensions().ConfigSpecs(),
	})
}
