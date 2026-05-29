# Moon Bridge

Moon Bridge est un proxy de conversion de protocole et de routage de modèles écrit en Go. Il expose une **API OpenAI Responses** (`/v1/responses`) en externe et supporte en interne plusieurs protocoles amont : **Anthropic Messages**, **Google Gemini (GenAI)**, **OpenAI Chat Completions**, etc. Lorsque le client spécifie un alias de modèle différent, la requête est automatiquement routée vers le fournisseur amont correspondant avec conversion automatique entre les protocoles.

> 🍳 **Débutant ? Commencez ici** → [CookBook.md](CookBook.md) : un recueil de recettes pour atteindre vos objectifs, 5 minutes pour un premier dialogue.

---

## Démarrage rapide

```bash
# 1. Éditer la configuration (déjà présente : config.yml)
#    → Remplacer les clés API (voir tableau plus bas)

# 2. Démarrer Moon Bridge
go run ./cmd/moonbridge -config config.yml

# 3. Tester avec curl
curl -X POST http://127.0.0.1:38440/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"model": "moonbridge", "input": "Bonjour !"}'
```

Nécessite Go 1.25+.

## Fonctionnalités principales

- **Conversion de protocole** : OpenAI Responses → Anthropic Messages / Google Gemini / OpenAI Chat, compatible avec quatre protocoles amont
- **Routage de modèles** : via la configuration `routes`, les alias de modèles sont mappés à différents noms de modèles amont
- **Extensions par plugins** : interface `CorePluginHooks`, support du prétraitement des requêtes, post-traitement des réponses, interception de flux
- **Traçage des requêtes** : enregistrement complet de la chaîne, chaque étape de conversion est traçable
- **Statistiques d'utilisation** : agrégation des tokens et des coûts par session
- **API de gestion** : rechargement à chaud de la configuration (nécessite la persistance)
- **Injection Web Search** : modes automatique / injecté, support de Tavily et Firecrawl
- **Cache Prompt** : trois modes : explicit / automatic / hybrid

## Trois modes de fonctionnement

| Mode | Comportement |
|------|-------------|
| `Transform` (défaut) | Reçoit les requêtes OpenAI Responses → conversion de protocole → transfert → reconversion et retour |
| `CaptureAnthropic` | Reçoit les requêtes Anthropic Messages → transfert transparent vers Anthropic |
| `CaptureResponse` | Reçoit les requêtes OpenAI Responses → transfert transparent vers OpenAI |

## Configuration

Au format YAML, structure centrale en trois sections : `models`, `providers`, `routes`. Voir [CONFIGURATION.md](docs/CONFIGURATION.md) pour la documentation complète.

### Fournisseurs configurés

Moon Bridge supporte actuellement **5 fournisseurs** dans `config.yml` :

| Fournisseur | Protocole | URL | Modèles |
|---|---|---|---|
| **deepseek** | Anthropic | `api.deepseek.com/anthropic` | DeepSeek V4 Pro, DeepSeek V4 Flash |
| **opencode-go** | OpenAI Chat | `opencode.ai/zen/go` | GLM 5.1, GLM 5, Kimi K2.5/K2.6, MiMo V2.5/Pro, DeepSeek V4 Pro/Flash |
| **opencode-go-anthropic** | Anthropic | `opencode.ai/zen/go` | MiniMax M2.7/M2.5, Qwen3.7 Max/3.6 Plus |
| **openrouter** | OpenAI Chat | `openrouter.ai/api/v1` | GPT-4o, Claude Opus 4, Gemini 2.5 Pro, DeepSeek Chat |
| **ollama** *(local)* | OpenAI Chat | `localhost:11434` | Llama 3.3, DeepSeek R1, Qwen 2.5, Mistral |

→ Les modèles `deepseek-v4-pro` et `deepseek-v4-flash` sont partagés entre les fournisseurs **deepseek** et **opencode-go** via leurs offres respectives.

### 🔑 Clés API à modifier

Avant de pouvoir utiliser Moon Bridge, remplacez les clés API fictives dans `config.yml` par vos vraies clés :

| Clé à remplacer | Obtenir la clé | Fournisseur concerné |
|---|---|---|
| `replace-with-deepseek-api-key` | [platform.deepseek.com](https://platform.deepseek.com) | deepseek |
| `replace-with-opencode-go-api-key` | [opencode.ai/auth](https://opencode.ai/auth) (abonnement Go) | opencode-go, opencode-go-anthropic |
| `replace-with-openrouter-api-key` | [openrouter.ai](https://openrouter.ai) | openrouter |

> **Ollama** ne nécessite pas de clé API : le champ `api_key` est défini à `ollama` (valeur factice requise par la validation).

### Routes disponibles

```bash
# Modèle par défaut : DeepSeek V4 Pro (route "moonbridge")
curl -X POST http://127.0.0.1:38440/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"model": "moonbridge", "input": "Bonjour !"}'

# OpenRouter — GPT-4o
curl -X POST http://127.0.0.1:38440/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"model": "openrouter-gpt-4o", "input": "Bonjour !"}'

# Ollama local — Llama 3.3
curl -X POST http://127.0.0.1:38440/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"model": "ollama-llama3.3", "input": "Bonjour !"}'
```

Toutes les routes disponibles sont listées dans la section `routes` de `config.yml`.

## Utilisation avec Codex CLI

```bash
# Générer la configuration Codex pour le modèle par défaut
go run ./cmd/moonbridge -config config.yml -print-codex-config moonbridge -codex-home ~/.config/codex

# Ou définir manuellement l'URL de base
export CODEX__OPENAI__BASE_URL="http://127.0.0.1:38440/v1"
export CODEX__OPENAI__API_KEY="any-non-empty-value"
```

Puis démarrez Codex CLI, il utilisera Moon Bridge comme proxy pour accéder aux modèles configurés.

## Utilisation avec Claude Code

```bash
claude --model moonbridge --api-url http://127.0.0.1:38440 --api-key any-value
```

## Déploiement Docker

```bash
docker build -t moonbridge .
docker run -p 38440:38440 -v $(pwd)/config.yml:/config/config.yml moonbridge
```

## Options en ligne de commande

| Option | Valeur par défaut | Description |
|--------|-------------------|-------------|
| `-config` | `${XDG_CONFIG_HOME}/moonbridge/config.yml` | Chemin du fichier de configuration |
| `-addr` | Depuis la configuration | Surcharge l'adresse d'écoute |
| `-mode` | Depuis la configuration | Surcharge le mode (Transform/CaptureAnthropic/CaptureResponse) |
| `-print-addr` | — | Affiche l'adresse d'écoute configurée puis quitte |
| `-print-mode` | — | Affiche le mode configuré puis quitte |
| `-print-default-model` | — | Affiche l'alias de modèle par défaut puis quitte |
| `-print-codex-model` | — | Affiche le modèle Codex puis quitte |
| `-print-codex-config <model>` | — | Génère config.toml pour Codex puis quitte |
| `-dump-config-schema` | — | Génère config.schema.json puis quitte |

## Points d'accès HTTP API

| Point d'accès | Méthode | Description |
|---------------|---------|-------------|
| `/v1/responses` | POST | Point d'entrée principal de l'API OpenAI Responses |
| `/responses` | POST | Idem (sans préfixe `/v1`) |
| `/v1/models` | GET | Liste les modèles disponibles |
| `/models` | GET | Idem |
| `/api/v1/` | — | API de gestion (nécessite persistance activée) |
| `/health` | GET | Vérification de l'état de santé |

Documentation API détaillée dans [API.md](docs/api.md).

## Traçage des requêtes

Activez le traçage via `trace.enabled` dans la configuration ou via un mode de fonctionnement spécifique. La chaîne complète requête/réponse est enregistrée dans des fichiers organisés par `session/nom_du_modèle/catégorie/numéro.json`, supportant trois catégories : Chat, Response, Anthropic.

## Licence

[GPL v3](LICENSE)
