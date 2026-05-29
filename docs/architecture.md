# Architecture du système

## Aperçu du projet

Moon Bridge est un serveur de proxy/conversion de protocole écrit en Go. Il expose une **API OpenAI Responses** (`/v1/responses`) en externe et supporte en interne quatre protocoles amont : **Anthropic Messages**, **Google Gemini (GenAI)**, **OpenAI Chat Completions**, ainsi que le passage direct OpenAI Responses.

Positionnement clé : permettre à Codex CLI (ou tout autre client API OpenAI Responses) d'accéder via un point d'entrée unique à différents fournisseurs LLM amont avec des protocoles différents, sans que le client ait à connaître les différences de protocole.

## Architecture en quatre couches

```
┌──────────────────────────────────────────────────┐
│                  Couche Service                    │
│  server(routage/traitement)  adapter_dispatch     │
│  provider(routage)     stats(statistiques)        │
│  proxy(proxy Capture)  api(API de gestion)        │
│  store(persistance)    runtime(exécution)         │
├──────────────────────────────────────────────────┤
│                  Couche Protocol                   │
│  format(types/registre)  anthropic(adapt.)        │
│  openai(adapt. OpenAI)    google(adapt. GenAI)    │
│  chat(adapt. OpenAI Chat) cache(mise en cache)    │
├──────────────────────────────────────────────────┤
│           Composants de base (sous internal/)     │
│  config(configuration)  logger(journalisation)    │
│  openai_dto(DTO partagés)  modelref(réf. modèle)  │
│  session(session)  db(base de données)            │
├──────────────────────────────────────────────────┤
│                  Couche Extension                  │
│  kimi_workaround  metrics  codex(catalogue)       │
│  plugin(interface plugin)  db(backends: SQLite/D1)│
└──────────────────────────────────────────────────┘
```

### Composants de base (packages internes sous `internal/`)

Sans dépendance aux composants Protocol ou Service, packages directement sous `internal/` :

- `internal/config` — Chargement YAML, validation, génération Schema, rechargement à chaud. Supporte `config.schema.json` et `config.example.yml`
- `internal/logger` — Système de journalisation basé sur l'interface `slog.Handler`, support du mode consumer
- `internal/openai_dto` — Types de base OpenAI partagés (DTO, énumérations), réutilisés par plusieurs protocoles
- `internal/modelref` — Analyse et normalisation des références de modèle (`model(provider)`)
- `internal/session` — Gestion des sessions et liaison de contexte
- `internal/db` — Registre des fournisseurs de base de données

### Couche Protocol

Noyau de conversion de protocole, chaque adaptateur implémente l'interface unifiée `format.ProviderAdapter` (définie dans `internal/format/adapter.go`) :

- `internal/format` — Définitions de types principaux (`CoreRequest`, `CoreResponse`, `CoreTool`, `CoreContentBlock` dans `types.go`) + Registry (`registry.go`)
- `internal/protocol/openai` — Adaptateur OpenAI Responses : Core ⇄ format OpenAI Responses
- `internal/protocol/anthropic` — Adaptateur Anthropic Messages : conversion d'événements streaming, mapping d'appels d'outils, contrôle de cache
- `internal/protocol/google` — Adaptateur Google GenAI : conversion Gemini → Core
- `internal/protocol/chat` — Adaptateur OpenAI Chat : conversion Chat → Core
- `internal/protocol/cache` — Planification du cache Prompt (injection de breakpoints, gestion TTL, suivi du taux de succès)

### Couche Service

Couche d'orchestration métier, combine les composants de base et les protocoles :

- `internal/service/server` — Serveur HTTP, routage (`/v1/responses`, `/v1/models`, `/health`), authentification
- `internal/service/server/adapter_dispatch.go` — Chemin de distribution Adapter (switch type de protocole → appel de l'Adapter correspondant)
- `internal/service/provider` — Gestionnaire de fournisseurs (routage multi-fournisseur, rechargement à chaud)
- `internal/service/proxy` — Proxy transparent en mode Capture
- `internal/service/app` — Cycle de vie de l'application (initialisation, enregistrement des Adapters, démarrage HTTP)
- `internal/service/api` — API REST de gestion (CRUD de configuration runtime, routage dans `router.go`)
- `internal/service/stats` — Statistiques d'utilisation (agrégation de tokens et coûts par session)
- `internal/service/trace` — Traçage des requêtes (capture de la chaîne complète requête/réponse, persistance dans `data/trace/`)
- `internal/service/store` — Stockage persistant de configuration (SQLite / D1)
- `internal/service/runtime` — Contexte d'exécution
- `internal/service/bridge` — Couche de pont de secours

### Couche Extension

Extensions fonctionnelles enfichables, situées dans `internal/extension/` :

- `internal/extension/deepseek_v4` — Intégration DeepSeek V4 (reinforce instructions, rejeu de chaîne CoT)
- `internal/extension/visual` — Distribution de tâches visuelles (routage automatique quand le modèle principal ne supporte pas les images)
- `internal/extension/websearch` — Mode automatique Web Search
- `internal/extension/websearchinjected` — Mode injecté Web Search
- `internal/extension/metrics` — Collecte et interrogation des métriques de requêtes
- `internal/extension/plugin` — Gestion d'enregistrement des plugins tiers (`PluginRegistry` + `CorePluginHooks`)
- `internal/extension/codex` — Catalogue de modèles Codex
- `internal/extension/db` — Fournisseurs de persistance (SQLite local / Cloudflare D1 Worker)

## Trois modes de fonctionnement

| Mode | Protocole d'entrée → Protocole amont | Description |
|------|--------------------------------------|-------------|
| `Transform` (défaut) | OpenAI Responses → tout Adapter | Pipeline complet de conversion de protocole |
| `CaptureAnthropic` | Anthropic Messages → Anthropic | Livraison transparente |
| `CaptureResponse` | OpenAI Responses → OpenAI | Livraison transparente |

## Flux de données du cycle de vie d'une requête (mode Transform)

```
Client (Codex CLI)
    │ POST /v1/responses (format OpenAI Responses)
    ▼
internal/service/server (dispatch.go)
    │ Authentification / Journalisation / Initialisation stats / Résolution routage
    ▼
adapter_dispatch.go (Distribution Adapter)
    │ preferred.Protocol détermine le protocole amont
    │
    ├── ProtocolAnthropic    → anthropic.ProviderAdapter (conversion Core → Anthropic Messages)
    ├── ProtocolOpenAIChat   → chat.ProviderAdapter     (conversion Core → OpenAI Chat)
    ├── ProtocolGoogleGenAI  → google.ProviderAdapter   (conversion Core → Google GenAI)
    ├── ProtocolOpenAIResponse → passage direct (sans conversion)
    │
    ├── Interception plugin (PluginHooks)
    │
    ▼
Fournisseur amont (API externe)
    │
    ▼ (conversion inverse)
Client ←── Réponse OpenAI Responses
```

## Routage des modèles

Priorité de résolution du routage :

1. Client spécifie directement le nom qualifié du fournisseur (format `model(provider)`)
2. Correspondance d'alias dans la configuration `routes` de Moon Bridge
3. Correspondance du nom de modèle dans la liste `offers` du fournisseur

## Champ protocol du fournisseur

Chaque fournisseur déclare son protocole amont via le champ `protocol` :

| Valeur | Format amont | Adapter correspondant |
|--------|-------------|----------------------|
| `anthropic` (défaut) | API Anthropic Messages | `internal/protocol/anthropic` |
| `openai-response` | API OpenAI Responses | `internal/protocol/openai` (passage direct) |
| `openai-chat` | API OpenAI Chat | `internal/protocol/chat` |
| `google-genai` | API Google Gemini | `internal/protocol/google` |

## Système d'Adapter

Tous les Adapters implémentent les interfaces définies dans `internal/format/adapter.go`, et sont gérés par le `Registry` dans `internal/format/registry.go` :

- `format.ProviderAdapter` — Convertit un `CoreRequest` en requête spécifique au protocole, appelle l'API, retourne `CoreResponse`
- `format.ProviderStreamAdapter` — Version streaming, retourne un canal `*CoreStreamEvent`

### Appels d'outils inter-protocoles

Le défi principal des appels d'outils entre protocoles réside dans les différences de format. Moon Bridge utilise `CoreTool` / `CoreContentBlock` comme représentation intermédiaire pour masquer ces différences :

- OpenAI Responses : `tool_use` est un type d'item de sortie
- Anthropic : `tool_use` est un type de `ContentBlock`
- Google Gemini : appel de fonction via `FunctionCall` dans `content.parts`
- OpenAI Chat : `tool_calls` dans le message assistant

### Injection d'outil Web Search

`InjectWebSearchTool` (défini dans `internal/service/server/server.go`) injecte dynamiquement l'outil `web_search` dans les requêtes en mode Transform. Supporte quatre modes : `auto` / `enabled` / `disabled` / `injected`. La recherche par injection est implémentée dans `adapter_dispatch.go` via `websearchinjected.WrapProvider()` pour une orchestration automatique.

## Système de cache

Implémenté via `internal/protocol/cache` pour le cache prompt de l'API Anthropic Messages. Supporte quatre modes : `off` / `explicit` / `automatic` / `hybrid`, avec configuration du TTL, du nombre minimum de tokens de cache, de la limite de breakpoints, etc.

## Système de traçage des requêtes

Le traçage est implémenté via `internal/service/trace` et `internal/service/server/trace`. Les fichiers de trace sont organisés par `session/nom_modèle/catégorie/numéro.json`, chaque enregistrement contient les données complètes de requête/réponse, supportant trois catégories : Chat, Response, Anthropic.

## API de gestion

Quand `persistence.active_provider` est activé (SQLite ou D1), l'API de gestion est disponible sous `/api/v1/` (routage dans `internal/service/api/router.go`) :

| Point d'accès | Méthode | Fonction |
|---------------|---------|----------|
| `/api/v1/config` | GET/PUT | Obtenir/mettre à jour la configuration runtime |
| `/api/v1/models` | GET | Lister les définitions de modèles dans la configuration |
| `/api/v1/models/{slug}` | GET/PUT/DELETE | Gérer une définition de modèle |
| `/api/v1/providers` | GET/POST/DELETE | Gérer les fournisseurs |
| `/api/v1/providers/{key}/offers/{model}` | PATCH/DELETE | Gérer les offres de modèles d'un fournisseur |

De plus, l'activation de l'extension metrics enregistre le point d'accès `/v1/admin/metrics` pour l'interrogation des métriques de requêtes.

La configuration Codex TOML est générée via le flag CLI `-print-codex-config <model>`, ce n'est pas un point d'accès API.
