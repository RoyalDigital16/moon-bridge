# Guide de démarrage

> 5 minutes pour un premier dialogue. Voir [CookBook.md](../CookBook.md) pour plus d'usages.

## 1. Installation

### Prérequis

- **Go 1.25+** — pour compiler et exécuter
- Une clé API d'un fournisseur LLM amont (DeepSeek, OpenAI, Anthropic, Kimi, etc.)

### Obtenir le code

```bash
git clone https://github.com/moonbridge/moonbridge.git
cd moonbridge
```

### Compiler

```bash
go build -o moonbridge ./cmd/moonbridge
```

Ou exécutez directement :

```bash
go run ./cmd/moonbridge -config config.yml
```

## 2. Configuration

Copiez l'exemple de configuration et éditez-le :

```bash
cp config.example.yml config.yml
# Éditez config.yml pour définir votre api_key et vos modèles
```

Voir [CONFIGURATION.md](CONFIGURATION.md) pour les détails de configuration.

### Configuration minimale (exemple avec DeepSeek)

```yaml
models:
  deepseek-model:
    context_window: 65536
    output_max: 8192

providers:
  deepseek:
    base_url: "https://api.deepseek.com"
    api_key: "sk-votre-clé-API"
    models:
      deepseek-model:
        upstream_model: "deepseek-chat"

routes:
  my-model: deepseek-model@deepseek
```

### Protocoles amont supportés

| Protocole | Valeur `protocol` | Exemple de fournisseur |
|-----------|-------------------|----------------------|
| Anthropic Messages | `anthropic` (défaut) | DeepSeek, Anthropic, Kimi |
| OpenAI Responses | `openai-response` | OpenAI (passage direct) |
| Google GenAI | `google-genai` | Google Gemini |
| OpenAI Chat | `openai-chat` | API compatible OpenAI Chat |

## 3. Démarrage

```bash
go run ./cmd/moonbridge -config config.yml
```

Sortie des logs :

```
INFO Serveur HTTP en écoute addr=127.0.0.1:38440
```

## 4. Tester la connectivité

```bash
curl http://127.0.0.1:38440/health
```

## 5. Vérifier la liste des modèles

```bash
curl http://127.0.0.1:38440/v1/models
```

## Prochaines étapes

- [CookBook.md](../CookBook.md) — Scénarios d'utilisation courants
- [architecture.md](architecture.md) — Architecture détaillée
- [CONFIGURATION.md](CONFIGURATION.md) — Guide de configuration complet
