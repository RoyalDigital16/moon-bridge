# Système d'Extension

Le système d'Extension de Moon Bridge est basé sur une architecture de plugins utilisant des interfaces de capacité (capability interfaces). Les plugins étendent les capacités du pont en implémentant l'interface de base `Plugin` et zéro ou plusieurs interfaces de capacité.

## Interfaces principales

### Plugin (interface de base)

Tous les plugins doivent implémenter l'interface `plugin.Plugin` :

```go
// internal/extension/plugin/plugin.go
type Plugin interface {
    Name() string                    // Identifiant unique (ex: "deepseek_v4")
    Init(ctx PluginContext) error    // Initialisation, reçoit la configuration
    Shutdown() error                 // Arrêt, libère les ressources
    EnabledForModel(modelAlias string) bool  // Actif pour le modèle spécifié
}
```

```go
type PluginContext struct {
    Config    any           // Configuration typée décodée selon la spec d'extension config
    AppConfig config.Config  // Configuration globale (lecture seule)
    Logger    *slog.Logger   // Logger avec le nom du plugin
}
```

Outil intégré : `BasePlugin` fournit des implémentations par défaut no-op pour toutes les méthodes, le plugin n'a qu'à surcharger celles nécessaires.

### RequestContext et StreamContext

Le premier paramètre des méthodes de capacité du plugin est généralement `*RequestContext` ou `*StreamContext`, définis dans `internal/extension/plugin/context.go` :

```go
type RequestContext struct {
    ModelAlias  string               // Alias du modèle (ex: "moonbridge")
    SessionData map[string]any       // Données de session inter-requêtes, indexées par nom de plugin
    Reasoning   map[string]any       // Configuration OpenAI reasoning
    WebSearch   WebSearchInfo        // Paramètres Web Search résolus
}

type StreamContext struct {
    RequestContext
    StreamState any  // État per-stream de ce plugin pour ce flux
}

func (ctx *RequestContext) SessionState(pluginName string) any {
    // Retourne l'état de session pour le plugin spécifié
}
```

L'isolation des données de session est garantie par `session.Session` — les sessions différentes (identifiées par `session_id` ou l'en-tête `X-Codex-Window-Id`) utilisent des mappages `ExtensionData` différents.

### ConfigSpecProvider

Le plugin déclare sa structure de configuration via `ConfigSpecProvider`, supportant la configuration multi-scope (global/Provider/Model/Route) :

```go
type ConfigSpecProvider interface {
    ConfigSpecs() []config.ExtensionConfigSpec
}
```

### Interfaces de capacité (Capability Interfaces)

Les plugins peuvent implémenter les interfaces de capacité suivantes selon leurs besoins. `plugin.Registry` les détecte automatiquement par assertion de type lors de l'enregistrement et les chaîne dans la méthode `CorePluginHooks()`.

#### Pipeline de requête (Request Pipeline)

| Interface | Méthode | Moment d'intervention |
|-----------|---------|-----------------------|
| `InputPreprocessor` | `PreprocessInput(ctx, raw) RawMessage` | Avant la désérialisation JSON d'entrée |
| `MessageRewriter` | `RewriteMessages(ctx, messages) []CoreMessage` | Après la transformation de la liste des messages |
| `RequestMutator` | `MutateRequest(ctx, req)` | Après la construction de CoreRequest, avant envoi au Provider Adapter |
| `ToolInjector` | `InjectTools(ctx) []CoreTool` | Injection d'outils supplémentaires lors de la conversion (retourne une liste CoreTool) |

#### Pipeline fournisseur (Provider Pipeline)

| Interface | Méthode | Moment d'intervention |
|-----------|---------|-----------------------|
| `ProviderWrapper` | `WrapProvider(ctx, provider) any` | Encapsulation du client fournisseur amont |

#### Pipeline de réponse (Response Pipeline)

| Interface | Méthode | Moment d'intervention |
|-----------|---------|-----------------------|
| `ContentFilter` | `FilterContent(ctx, block) bool` | Vérification bloc par bloc du contenu de la réponse, retourne true pour ignorer le bloc |
| `ResponsePostProcessor` | `PostProcessResponse(ctx, resp)` | Après la construction finale de la réponse OpenAI |
| `ContentRememberer` | `RememberContent(ctx, content)` | Quand le contenu complet de la réponse est disponible (ex. streaming terminé) |

#### Pipeline streaming (Streaming Pipeline)

| Interface | Méthode | Moment d'intervention |
|-----------|---------|-----------------------|
| `StreamInterceptor` | `NewStreamState() any` | Crée un état de flux per-request |
| | `OnStreamEvent(ctx, event) (consumed, emit)` | Chaque événement de flux ; si consumed=true, le bridge ignore le traitement normal |
| | `OnStreamComplete(ctx, outputText)` | Flux terminé |

```go
type StreamEvent struct {
    Type  string  // "block_start", "block_delta", "block_stop"
    Index int
    Block *format.CoreContentBlock  // pour block_start
    Delta anthropic.StreamDelta     // pour block_delta
}
```

#### Reconstruction d'historique (History Reconstruction)

| Interface | Méthode | Moment d'intervention |
|-----------|---------|-----------------------|
| `ThinkingPrepender` | `PrependThinkingForToolUse(messages, toolCallID, summary, state) []CoreMessage` | Ajoute un bloc thinking avant l'appel d'outil |
| | `PrependThinkingForAssistant(blocks, summary, state) []CoreContentBlock` | Ajoute un bloc thinking avant le message assistant |
| `ReasoningExtractor` | `ExtractThinkingBlock(ctx, summary) (CoreContentBlock, bool)` | Restaure un bloc thinking à partir d'un résumé reasoning |

#### Gestion des erreurs

| Interface | Méthode | Moment d'intervention |
|-----------|---------|-----------------------|
| `ErrorTransformer` | `TransformError(ctx, msg) string` | Transformation des messages d'erreur amont |

#### État de session

| Interface | Méthode | Moment d'intervention |
|-----------|---------|-----------------------|
| `SessionStateProvider` | `NewSessionState() any` | Création d'une nouvelle session |

#### Journalisation

| Interface | Méthode | Moment d'intervention |
|-----------|---------|-----------------------|
| `LogConsumer` | `ConsumeLog(ctx, entries) []LogEntry` | Chaque journal slog est distribué via le pipeline consume, peut intercepter, modifier ou supprimer |

#### Achèvement de requête et routage HTTP

| Interface | Méthode | Moment d'intervention |
|-----------|---------|-----------------------|
| `RequestCompletionHook` | `OnRequestCompleted(ctx, result)` | Après chaque requête terminée, reçoit le modèle, tokens, coût, statut et durée |
| `RouteRegistrar` | `RegisterRoutes(register)` | Enregistre des gestionnaires HTTP supplémentaires lors de l'initialisation du serveur |

#### Persistance

| Interface | Méthode | Moment d'intervention |
|-----------|---------|-----------------------|
| `DBProvider` | `DBProvider() db.Provider` | Déclare un backend de base de données, comme SQLite ou D1 |
| `DBConsumer` | `DBConsumer() db.Consumer` | Déclare un besoin de base de données, comme metrics |

#### Interfaces adaptateur au format Core (chemin Adapter)

Les interfaces suivantes sont dédiées au chemin Adapter (lors de la conversion d'une réponse OpenAI vers un autre protocole), définies dans `internal/extension/plugin/capabilities.go` :

| Interface | Méthode | Moment d'intervention |
|-----------|---------|-----------------------|
| `CoreRequestMutator` | `MutateCoreRequest(ctx, req)` | Après la construction de CoreRequest (contexte standard) |
| `CoreContentFilter` | `FilterCoreContent(ctx, block) bool` | Filtre les blocs de contenu Core |
| `CoreContentRememberer` | `RememberCoreContent(ctx, content)` | Mémorise les blocs de contenu Core |

## Registre (Registry)

`plugin.Registry` gère tous les plugins enregistrés, stockés par type de capacité.

```go
// internal/extension/plugin/registry.go
type Registry struct {
    plugins            []Plugin
    inputPreprocessors []InputPreprocessor
    requestMutators    []RequestMutator
    toolInjectors      []ToolInjector
    dbProviders        []DBProvider
    dbConsumers        []DBConsumer
    requestCompletionHooks []RequestCompletionHook
    routeRegistrars    []RouteRegistrar
    // ... autres listes de capacités
}
```

### Processus d'enregistrement

```go
// 1. Créer le registre
registry := plugin.NewRegistry(logger.L())

// 2. Enregistrer les plugins (détection automatique des capacités)
registry.Register(deepseekv4.NewPlugin())
registry.Register(visual.NewPlugin())
registry.Register(dbsqlite.NewPlugin())
registry.Register(metrics.NewPlugin())

// 3. Initialiser (transmet AppConfig et la configuration typée d'extension)
if err := registry.InitAll(&cfg); err != nil {
    // cfg.ExtensionConfig("deepseek_v4", "") → décodage en *deepseekv4.Config
}

// 4. Construire CorePluginHooks (chaîne toutes les capacités des plugins)
hooks := registry.CorePluginHooks()
// Retourne la structure format.CorePluginHooks, transmise à chaque Adapter

// 5. Nettoyage à l'arrêt de l'application
defer registry.ShutdownAll()
```

La méthode `Registry.CorePluginHooks()` (`registry.go:486`) parcourt les plugins enregistrés et pour ceux qui implémentent `CoreRequestMutator`, `CoreContentFilter`, `CoreContentRememberer`, les chaîne séquentiellement dans les champs correspondants de `format.CorePluginHooks`.

## Intégration avec l'Adapter

Le Plugin s'intègre avec le chemin Adapter via `format.CorePluginHooks` (défini dans `internal/format/adapter.go`). C'est une structure de fonctions, automatiquement construite par `Registry.CorePluginHooks()` :

```go
type CorePluginHooks struct {
    PreprocessInput        func(ctx context.Context, model string, raw json.RawMessage) json.RawMessage
    RewriteMessages        func(ctx context.Context, req *CoreRequest)
    InjectTools            func(ctx context.Context) []CoreTool
    MutateCoreRequest      func(ctx context.Context, req *CoreRequest)
    PostProcessCoreResponse func(ctx context.Context, resp *CoreResponse)
    TransformError         func(ctx context.Context, model string, msg string) string
    OnStreamEvent          func(ctx context.Context, event CoreStreamEvent) (skip bool)
    OnStreamComplete       func(ctx context.Context, model string, outputText string)
    FilterContent          func(ctx context.Context, block *CoreContentBlock) (skip bool)
    RememberContent        func(ctx context.Context, content []CoreContentBlock)
    NewStreamState         func(ctx context.Context, model string) any
    PrependThinkingToAssistant func(ctx context.Context, req *CoreRequest)
}

func (hooks CorePluginHooks) WithDefaults() CorePluginHooks {
    // Remplace toutes les fonctions nil par des no-op, garantit un appel sécurisé
}
```

L'Adapter appelle ces hooks pendant la conversion :

```go
// Dans l'Adapter fournisseur amont :
a.hooks.MutateCoreRequest(ctx, req)  // Modifie CoreRequest
a.hooks.RememberContent(ctx, content) // Enregistre le contenu de la réponse

// Dans l'Adapter client OpenAI :
a.hooks.PreprocessInput(ctx, model, raw)      // Prétraite l'entrée
a.hooks.PostProcessCoreResponse(ctx, resp)     // Post-traite la réponse
```

La couche Serveur utilise également directement les capacités des plugins :

- `LogConsumer` : connecté via `logger.SetConsumeFunc()` au tampon de journal.
- `DBProvider` / `DBConsumer` : initialisé par `db.Registry` pour lier la base de données et les consommateurs.
- `RequestCompletionHook` : déclenché par `server.onRequestCompleted()` après chaque requête.
- `RouteRegistrar` : monté sur `http.ServeMux` par `server.registerPluginRoutes()`.

Le module `websearchinjected` dans le répertoire des extensions intégrées a une implémentation d'interface plugin, mais dans le chemin d'exécution actuel, la recherche par injection est appelée directement par bridge/server selon le mode web search résolu du modèle, en utilisant les fonctions d'outil et d'encapsulation Provider de `websearch` / `websearchinjected`, sans être enregistrée dans `BuiltinExtensions()`.

## Configuration

Les paramètres d'extension sont configurés dans la section `extensions` de `config.yml`. Les paramètres propres à l'extension vont dans `config:`, l'état d'activation dans le champ `enabled` du scope correspondant :

```yaml
extensions:
  deepseek_v4:
    config:
      reinforce_instructions: true
      reinforce_prompt: "[System Reminder]: ...\n[User]:"
```

Le plugin déclare sa structure de configuration via `ConfigSpecProvider` :

```go
func (p *DSPlugin) ConfigSpecs() []config.ExtensionConfigSpec {
    return []config.ExtensionConfigSpec{{
        Name: "deepseek_v4",
        Scopes: []config.ExtensionScope{
            config.ExtensionScopeGlobal,
            config.ExtensionScopeProvider,
            config.ExtensionScopeModel,
            config.ExtensionScopeRoute,
        },
        Factory: func() any { return &Config{} },
    }}
}

func (p *DSPlugin) Init(ctx plugin.PluginContext) error {
    p.cfg = plugin.Config[Config](ctx)  // Décodage depuis PluginContext
    p.appCfg = ctx.AppConfig
    return nil
}

func (p *DSPlugin) EnabledForModel(model string) bool {
    return p.appCfg.ExtensionEnabled("deepseek_v4", model)
}
```

## Démo d'implémentation

### Plugin minimal

```go
package demo

import (
    "moonbridge/internal/extension/plugin"
)

const PluginName = "demo"

type DemoConfig struct {
    Prefix string `json:"prefix,omitempty" yaml:"prefix"`
}

type DemoPlugin struct {
    plugin.BasePlugin
    prefix string
}

func NewPlugin() *DemoPlugin {
    return &DemoPlugin{}
}

func (p *DemoPlugin) Name() string { return PluginName }

func (p *DemoPlugin) Init(ctx plugin.PluginContext) error {
    cfg := plugin.Config[DemoConfig](ctx)
    if cfg != nil {
        p.prefix = cfg.Prefix
    }
    ctx.Logger.Info("plugin demo initialisé", "prefix", p.prefix)
    return nil
}

func (p *DemoPlugin) EnabledForModel(model string) bool {
    return true  // Activé pour tous les modèles
}
```

### Plugin avec capacités

```go
package demo

import (
    "moonbridge/internal/extension/plugin"
    "moonbridge/internal/format"
    "moonbridge/internal/protocol/openai"
)

// Plugin qui injecte des outils supplémentaires
type SystemInjectionPlugin struct {
    plugin.BasePlugin
    systemMessage string
}

func (p *SystemInjectionPlugin) Name() string { return "system_inject" }

// --- RequestMutator (modifie CoreRequest) ---
func (p *SystemInjectionPlugin) MutateRequest(ctx *plugin.RequestContext, req *format.CoreRequest) {
    // Ajoute une instruction système
    req.System = append(req.System, format.CoreContentBlock{
        Type: "text",
        Text: p.systemMessage,
    })
}

// --- ToolInjector (injecte des outils supplémentaires) ---
func (p *SystemInjectionPlugin) InjectTools(ctx *plugin.RequestContext) []format.CoreTool {
    return []format.CoreTool{{
        Name:        "get_current_time",
        Description: "Obtenir l'heure système actuelle",
        InputSchema: map[string]any{"type": "object"},
    }}
}

// Assertion d'interface à la compilation
var (
    _ plugin.Plugin           = (*SystemInjectionPlugin)(nil)
    _ plugin.ToolInjector     = (*SystemInjectionPlugin)(nil)
    _ plugin.RequestMutator   = (*SystemInjectionPlugin)(nil)
)
```

### Enregistrer le plugin de démo

```go
// Dans runTransform() de service/app/app.go :
registry.Register(demo.NewPlugin())
if err := registry.InitAll(&cfg); err != nil {
    return fmt.Errorf("init plugins: %w", err)
}
defer registry.ShutdownAll()
```

Ensuite, `registry.CorePluginHooks()` construit automatiquement `format.CorePluginHooks` pour l'Adapter.
