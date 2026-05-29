package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"log/slog"
	"moonbridge/internal/config"
	"moonbridge/internal/extension/codex"
	"moonbridge/internal/logger"
	"moonbridge/internal/service/app"
)

const (
	exitOK          = 0
	exitRuntimeErr  = 1
	exitStartupErr  = 2
	defaultProgName = "moonbridge"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet(defaultProgName, flag.ContinueOnError)
	flags.SetOutput(stderr)

	configPath := flags.String("config", "", "Path to config.yml")
	addr := flags.String("addr", "", "Override server listen address")
	mode := flags.String("mode", "", "Override mode: CaptureAnthropic, CaptureResponse, or Transform")
	printAddr := flags.Bool("print-addr", false, "Print configured listen address and exit")
	printMode := flags.Bool("print-mode", false, "Print configured mode and exit")
	printDefaultModel := flags.Bool("print-default-model", false, "Print configured default model alias and exit")
	printCodexModel := flags.Bool("print-codex-model", false, "Print configured Codex model and exit")
	printClaudeModel := flags.Bool("print-claude-model", false, "Print configured Claude Code model and exit")
	printCodexConfig := flags.String("print-codex-config", "", "Print Codex config.toml for the model alias and exit")
	dumpConfigSchema := flags.Bool("dump-config-schema", false, "Generate config.schema.json alongside config and exit")
	codexBaseURL := flags.String("codex-base-url", "", "Base URL to write in generated Codex config")
	codexHome := flags.String("codex-home", "", "CODEX_HOME directory; when set, writes models_catalog.json there")
	if err := flags.Parse(args); err != nil {
		return exitStartupErr
	}

	var cfg config.Config
	var err error
	extensions := app.BuiltinExtensions()
	resolvedConfigPath, err := config.ResolveConfigPath(*configPath)
	if err != nil {
		writeStartupError(stderr, "Échec de l'analyse du chemin du fichier de configuration", "", err,
			"Définissez XDG_CONFIG_HOME, ou utilisez -config pour spécifier le chemin du fichier de configuration.")
		return exitStartupErr
	}
	if *dumpConfigSchema {
		if err := app.DumpConfigSchema(resolvedConfigPath); err != nil {
			writeStartupError(stderr, "Échec du vidage du schéma", resolvedConfigPath, err)
			return exitStartupErr
		}
		fmt.Fprintln(stdout, resolvedConfigPath)
		return exitOK
	}

	cfg, err = config.LoadFromFileWithOptions(resolvedConfigPath, config.LoadOptions{
		ExtensionSpecs: extensions.ConfigSpecs(),
	})
	if err != nil {
		writeStartupError(stderr, "Échec du chargement de la configuration", resolvedConfigPath, err,
			"Par défaut (sans -config) : ${XDG_CONFIG_HOME:-$HOME/.config}/moonbridge/config.yml.",
			"Vérifiez la syntaxe YAML, l'orthographe des champs et l'indentation.",
			"Assurez-vous que les configurations obligatoires (provider, routes, developer.proxy) sont toutes présentes.",
			"Pour le champ protocol, utilisez openai-response pour le passage direct Responses.")
		return exitStartupErr
	}
	if err := logger.Init(logger.Config{Level: logger.Level(cfg.LogLevel), Format: cfg.LogFormat, Output: stderr}); err != nil {
		writeStartupError(stderr, "Échec de l'initialisation du journal", resolvedConfigPath, err,
			"Vérifiez que log.level et log.format sont des valeurs supportées.")
		return exitStartupErr
	}
	slog.Info("Configuration chargée", "path", resolvedConfigPath, "mode", cfg.Mode, "addr", cfg.Addr)
	if *mode != "" {
		cfg.Mode = config.Mode(*mode)
		if err := cfg.Validate(); err != nil {
			writeStartupError(stderr, "Échec de la validation de la configuration", resolvedConfigPath, fmt.Errorf("-mode %q: %w", *mode, err),
				"Vérifiez que -mode est Transform, CaptureResponse ou CaptureAnthropic.",
				"Les configurations provider / developer.proxy pour ce mode doivent également être complètes.")
			return exitStartupErr
		}
	}
	if *addr != "" {
		cfg.OverrideAddr(*addr)
	}
	if *printAddr {
		fmt.Fprintln(stdout, cfg.Addr)
		return exitOK
	}
	if *printMode {
		fmt.Fprintln(stdout, cfg.Mode)
		return exitOK
	}
	if *printDefaultModel {
		fmt.Fprintln(stdout, cfg.DefaultModelAlias())
		return exitOK
	}
	if *printCodexModel {
		fmt.Fprintln(stdout, cfg.CodexModel())
		return exitOK
	}
	if *printClaudeModel {
		fmt.Fprintln(stdout, cfg.AnthropicProxy.Model)
		return exitOK
	}
	if *printCodexConfig != "" {
		if err := codex.GenerateConfigToml(stdout, *printCodexConfig, *codexBaseURL, *codexHome,
			config.ProviderFromGlobalConfig(&cfg), config.PluginFromGlobalConfig(&cfg), config.ServerFromGlobalConfig(&cfg)); err != nil {
			writeStartupError(stderr, "Échec de la génération de la configuration Codex", resolvedConfigPath, err,
				"Assurez-vous que le répertoire -codex-home est accessible en écriture, ou omettez -codex-home pour afficher seulement config.toml.")
			return exitRuntimeErr
		}
		return exitOK
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	if err := app.RunServer(ctx, cfg, stderr); err != nil {
		writeStartupError(stderr, "Échec de l'exécution du service", resolvedConfigPath, err,
			"Vérifiez si l'adresse d'écoute est déjà utilisée et si la configuration du fournisseur amont est disponible.")
		return exitRuntimeErr
	}
	return exitOK
}

func writeStartupError(output io.Writer, title string, configPath string, err error, hints ...string) {
	fmt.Fprintf(output, "Échec du démarrage de Moon Bridge : %s\n", title)
	if configPath != "" {
		fmt.Fprintf(output, "Fichier de configuration : %s\n", configPath)
	}
	fmt.Fprintln(output, "Détails de l'erreur :")
	for i, msg := range errorChain(err) {
		fmt.Fprintf(output, "  %d. %s\n", i+1, msg)
	}
	if len(hints) == 0 {
		return
	}
	fmt.Fprintln(output, "Suggestions :")
	for _, hint := range hints {
		fmt.Fprintf(output, "  - %s\n", hint)
	}
}

func errorChain(err error) []string {
	if err == nil {
		return []string{"<nil>"}
	}
	var messages []string
	for current := err; current != nil; current = errors.Unwrap(current) {
		messages = append(messages, current.Error())
	}
	return messages
}
