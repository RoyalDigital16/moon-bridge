# Configuration

> Voir l'exemple complet dans [`config.example.yml`](config.example.yml), et le JSON Schema dans [`config.schema.json`](config.schema.json)

Moon Bridge utilise un fichier de configuration YAML. Le chemin par défaut est `config.yml` dans le répertoire courant, mais vous pouvez spécifier un chemin avec `-config <path>`.

## Structure racine

```yaml
mode: "Transform"  # Transform / CaptureAnthropic / CaptureResponse

log:
  level: "info"    # debug / info / warn / error
  format: "text"   # text / json

server:
  addr: "127.0.0.1:38440"
  auth_token: ""

system_prompt: ""  # Prompt système global (optionnel)

defaults:
  model: "moonbridge"
  max_tokens: 65536
```

## Mode

| Valeur | Comportement |
|--------|-------------|
| `Transform` | Reçoit les requêtes OpenAI Responses, les convertit selon le protocole du fournisseur et les transmet |
| `CaptureAnthropic` | Proxy transparent vers Anthropic (sans conversion) |
| `CaptureResponse` | Proxy transparent vers OpenAI (sans conversion) |

## Server

```yaml
server:
  addr: "127.0.0.1:38440"    # Adresse d'écoute
  auth_token: ""              # Jeton d'authentification Bearer (vide = pas d'authentification)
```

## Models

Les définitions de modèles contiennent la fenêtre de contexte, les capacités d'inférence, le support d'extensions, etc. :

```yaml
models:
  my-model:
    context_window: 1000000
    max_output_tokens: 384000
    display_name: "My Model"
    default_reasoning_level: "high"
    supported_reasoning_levels:
      - effort: "low"
        description: "Raisonnement faible"
      - effort: "medium"
        description: "Raisonnement moyen"
      - effort: "high"
        description: "Raisonnement élevé"
      - effort: "xhigh"
        description: "Raisonnement très élevé"
    supports_reasoning_summaries: true
    input_modalities:
      - "text"
      - "image"
    web_search:
      support: "auto"     # auto / enabled / disabled / injected
    extensions:
      deepseek_v4:
        enabled: true
      visual:
        enabled: true
```

## Providers

Les fournisseurs définissent les informations de connexion à l'API amont et le type de protocole.

```yaml
providers:
  my-provider:
    base_url: "https://api.example.com"
    api_key: "sk-..."
    version: "2023-06-01"
    user_agent: "moonbridge/1.0"
    protocol: "anthropic"         # anthropic par défaut

    # Champs spécifiques Google GenAI (protocol: google-genai)
    project: "my-gcp-project"
    location: "us-central1"
    api_version: "v1beta"

    web_search:
      support: "auto"
      max_uses: 1
      tavily_api_key: "tvly-..."
      firecrawl_api_key: "fc-..."
      search_max_rounds: 3

    offers:
      - model: my-model
        pricing:
          input_price: 2
          output_price: 8
          cache_write_price: 1
          cache_read_price: 0.25
```

### Types de protocole

| Valeur | Format amont | Adapter correspondant |
|--------|-------------|----------------------|
| `anthropic` (défaut) | API Anthropic Messages | `internal/protocol/anthropic` |
| `openai-response` | API OpenAI Responses | `internal/protocol/openai` (passage direct) |
| `google-genai` | API Google Generative AI (Gemini) | `internal/protocol/google` |
| `openai-chat` | API OpenAI Chat Completions | `internal/protocol/chat` |

## Routes

Les routes mappent les alias de modèles vers un modèle amont spécifique d'un fournisseur :

```yaml
routes:
  nom-alias:              # Nom de modèle utilisé par le client
    model: my-model        # Nom du modèle défini dans la section models
    provider: my-provider  # Nom du fournisseur défini dans la section providers
```

## Web Search

Le support Web Search peut être configuré à trois niveaux : modèle, fournisseur et global (priorité : modèle > fournisseur > global).

| Mode | Comportement |
|------|-------------|
| `auto` | Utilise d'abord l'API web_search native du fournisseur, repli sur le mode injection si non supporté |
| `enabled` | Active le web_search natif du fournisseur |
| `disabled` | Désactive la recherche Web |
| `injected` | Injecte les résultats de recherche via le backend Tavily/Firecrawl |

## Cache

```yaml
cache:
  mode: "explicit"              # off / explicit / automatic / hybrid
  ttl: "5m"
  prompt_caching: true
  automatic_prompt_cache: false
  explicit_cache_breakpoints: true
  allow_retention_downgrade: false
  max_breakpoints: 4
  min_cache_tokens: 1024
  expected_reuse: 2
  minimum_value_score: 2048
  min_breakpoint_tokens: 1024
```

## Extensions

```yaml
extensions:
  deepseek_v4:
    enabled: true
    config:
      reinforce_instructions: true
  visual:
    enabled: true
    config:
      provider: "kimi"
      model: "kimi-for-coding"
      max_rounds: 4
      max_tokens: 2048
  db_sqlite:
    enabled: true
    config:
      path: ./data/moonbridge.db
      wal: true
      busy_timeout_ms: 5000
      max_open_conns: 1
  metrics:
    enabled: true
    config:
      default_limit: 100
      max_limit: 1000
```

## Proxy (mode Capture)

Valable uniquement en mode Capture :

```yaml
proxy:
  response:
    base_url: "https://api.openai.com"
    api_key: "sk-..."
  anthropic:
    base_url: "https://provider.example.com"
    api_key: "sk-..."
    version: "2023-06-01"
```

## Flags CLI

| Flag | Valeur par défaut | Description |
|------|-------------------|-------------|
| `-config` | `${XDG_CONFIG_HOME}/moonbridge/config.yml` | Chemin du fichier de configuration |
| `-addr` | Depuis la configuration | Surcharge l'adresse d'écoute |
| `-mode` | Depuis la configuration | Surcharge le mode (Transform/CaptureAnthropic/CaptureResponse) |
| `-print-addr` | — | Affiche l'adresse d'écoute configurée puis quitte |
| `-print-mode` | — | Affiche le mode configuré puis quitte |
| `-print-default-model` | — | Affiche l'alias de modèle par défaut puis quitte |
| `-print-codex-model` | — | Affiche le modèle Codex puis quitte |
| `-print-codex-config <model>` | — | Génère config.toml pour le modèle spécifié puis quitte |
| `-dump-config-schema` | — | Génère config.schema.json puis quitte |
