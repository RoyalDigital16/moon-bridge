# Moon Bridge

Moon Bridge est un proxy de conversion de protocole et de routage de modèles écrit en Go. Il expose une **API OpenAI Responses** (`/v1/responses`) en externe et supporte en interne plusieurs protocoles amont : **Anthropic Messages**, **Google Gemini (GenAI)**, **OpenAI Chat Completions**, etc. Lorsque le client spécifie un alias de modèle différent, la requête est automatiquement routée vers le fournisseur amont correspondant avec conversion automatique entre les protocoles.

> 🍳 **Débutant ? Commencez ici** → [CookBook.md](CookBook.md) : un recueil de recettes pour atteindre vos objectifs, 5 minutes pour un premier dialogue.

---

## Démarrage rapide

```bash
# Copier et éditer la configuration
cp config.example.yml config.yml
# Modifier api_key dans config.yml

# Démarrer
go run ./cmd/moonbridge -config config.yml

# Voir CookBook.md pour les scénarios d'utilisation détaillés
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

## Utilisation avec Codex CLI

Définissez l'adresse Moon Bridge comme URL de base de l'API OpenAI de Codex :

```toml
[openai]
base_url = "http://127.0.0.1:38440/v1"
api_key = "any-non-empty-value"
```

Puis définissez dans la configuration Moon Bridge des routes portant le même nom que les modèles Codex.

## Utilisation avec Claude Code

```bash
claude --model your-alias --api-url http://127.0.0.1:38440 --api-key any-value
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
