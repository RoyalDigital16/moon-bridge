# Guide de développement

## Prérequis

- Go 1.25+
- Clé API d'un fournisseur LLM amont (optionnelle, pour les tests E2E)

## Structure du projet

```
cmd/
  moonbridge/    # Point d'entrée principal (binaire)
  cloudflare/    # Point d'entrée Cloudflare Worker
internal/
  e2e/                    # Tests d'intégration de bout en bout (conversion de protocole)
  extension/              # Extensions enfichables
    codex/                # Catalogue de modèles Codex
    db/                   # Fournisseurs de base de données (SQLite / D1)
    deepseek_v4/          # Optimisation d'inférence DeepSeek V4
    kimi_workaround/      # Limitation des tours d'appels d'outils Kimi
    metrics/              # Métriques d'utilisation
    plugin/               # Interface et registre des plugins
    visual/               # Distribution de modèles visuels (mode CoreProvider)
    websearch/            # Orchestrateur Web Search
    websearchinjected/    # Mode injection Web Search
  config/                 # Chargement et validation de configuration YAML
  logger/                 # Système de journalisation (encapsulation slog)
  openai_dto/             # Types DTO OpenAI partagés
  modelref/               # Résolution de références de modèle
  session/                # Gestion de session
  db/                     # Abstraction et registre de base de données
  format/                 # Types Core + interface Adapter + Registry
  protocol/               # Couche de conversion de protocole
    anthropic/            # Adaptateur Anthropic Messages
    openai/               # Adaptateur OpenAI Responses
    google/               # Adaptateur Google Gemini/GenAI
    chat/                 # Adaptateur OpenAI Chat
    cache/                # Planification du cache Prompt
  service/                # Couche d'orchestration métier
    api/                  # API REST de gestion (routage dans router.go)
    app/                  # Cycle de vie de l'application + répertoire d'extensions
    bridge/               # (Répertoire vide, réservé)
    e2e/                  # Tests E2E de la couche service
    provider/             # Gestionnaire de fournisseurs
    proxy/                # Proxy en mode Capture
    runtime/              # Contexte d'exécution
    server/               # Serveur HTTP + routage + authentification
    stats/                # Statistiques d'utilisation
    store/                # Persistance de configuration
    trace/                # Traçage des requêtes
```

## Construction

```bash
# Construire le binaire
go build -o moonbridge ./cmd/moonbridge

# Construire le Cloudflare Worker (WASM)
GOOS=wasip1 GOARCH=wasm go build -o build/cloudflare.wasm ./cmd/cloudflare
```

## Exécution

```bash
go run ./cmd/moonbridge -config config.yml
```

Support du rechargement à chaud : après modification de la configuration, appliquez les changements via l'API de gestion ou redémarrez l'application.

## Commandes courantes

```bash
# Tests unitaires complets
go test ./...

# Tests au niveau d'un package
go test ./internal/protocol/anthropic/...

# Tests E2E (mode Mock, sans clé API)
go test ./internal/e2e/ -mock

# Tests E2E pour un fournisseur spécifique
go test ./internal/e2e/ -run TestDeepSeek -v

# Utiliser le Makefile pour construire et tester
make build
make test
```

## Ajouter un nouvel adaptateur fournisseur

1. Ajoutez une constante de protocole dans `internal/config/config.go` (ex: `ProtocolMyAdapter`)
2. Créez le package `internal/protocol/<adapter>/` implémentant les interfaces `ProviderAdapter` et `ProviderStreamAdapter` de `internal/format/adapter.go`
3. Enregistrez l'adaptateur dans le Registry via `internal/service/app/app.go`
4. Ajoutez une branche de protocole dans `internal/service/server/adapter_dispatch.go`
5. Ajoutez des tests E2E correspondants dans `internal/e2e/`

## Développement de l'API de gestion

Les points d'accès de l'API de gestion sont définis dans `internal/service/api/`, le routage est créé via `NewRouter` (`router.go`).

## Conventions de code

- Les noms de fichiers reflètent leur responsabilité (ex: `candidate_routing_test.go`), pas de numéros de gestion de projet
- Utilisez `log/slog` pour la journalisation structurée
- La configuration au niveau package est gérée via `internal/config`
- La conversion de protocole utilise `CoreRequest` / `CoreResponse` de `internal/format` comme représentation intermédiaire
