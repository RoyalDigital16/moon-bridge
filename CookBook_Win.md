# Moon Bridge CookBook (Windows)

> **Note importante** Windows n'a pas de variable d'environnement XDG_CONFIG_HOME par défaut. Lancez avec `-config chemin\vers\votre\config.yml`.

---

## Index des recettes

| # | Recette | Durée | Difficulté |
|---|--------|-------|------------|
| 0 | [Avant de commencer](#0-avant-de-commencer) | 2 min | ⭐ |
| 1 | [Premier dialogue en 5 minutes](#1-premier-dialogue-en-5-minutes) | 5 min | ⭐ |
| 2 | [Connecter Codex CLI à Moon Bridge](#2-connecter-codex-cli-à-moon-bridge) | 3 min | ⭐⭐ |
| 3 | [Changer de fournisseur](#3-changer-de-fournisseur) | 3 min | ⭐⭐ |
| 4 | [Activer les capacités de raisonnement DeepSeek V4](#4-activer-les-capacités-de-raisonnement-deepseek-v4) | 2 min | ⭐ |
| 5 | [Permettre au modèle de voir des images (extension Visual)](#5-permettre-au-modèle-de-voir-des-images-extension-visual) | 5 min | ⭐⭐⭐ |
| 6 | [Activer la recherche Web](#6-activer-la-recherche-web) | 5 min | ⭐⭐ |
| 7 | [Activer le cache Prompt](#7-activer-le-cache-prompt) | 2 min | ⭐ |
| 8 | [Dépannage rapide](#8-dépannage-rapide) | — | — |

---

## 0. Avant de commencer

**Ingrédients :**

- **Go 1.25+** — Vérifiez avec `go version`. Si absent, téléchargez depuis [go.dev](https://go.dev/dl/) ou via [scoop](https://scoop.sh/) (`scoop install go`).
- **Clé API** — DeepSeek recommandé, créez une clé sur [platform.deepseek.com](https://platform.deepseek.com).
- **Un terminal (PowerShell 7 recommandé : https://github.com/powershell/powershell/releases)**

**Vérification :**

```powershell
go version
```

**Problèmes courants :**

| Problème | Cause | Solution |
|----------|-------|----------|
| `command not found: go` | Go non installé | Télécharger depuis golang.org/dl |
| `go: command not found` | Pas dans le PATH | Redémarrer le terminal après installation |

---

## 1. Premier dialogue en 5 minutes

**Objectif :** Envoyer un texte et recevoir une réponse de l'IA.

**Ingrédients :**
- [Avant de commencer](#0-avant-de-commencer) terminé

**Étapes :**

### 1.1 Créer le fichier de configuration

Créez `config.yml` à la racine du projet, modifiez seulement `api_key` :

```yaml
models:
  deepseek-model:
    context_window: 65536
    output_max: 8192

providers:
  deepseek:
    base_url: "https://api.deepseek.com"
    api_key: "sk-votre-clé-DeepSeek"
    models:
      deepseek-model:
        upstream_model: "deepseek-chat"

routes:
  moonbridge: deepseek-model@deepseek
```

### 1.2 Démarrer

```powershell
go run .\cmd\moonbridge -config config.yml
```

Si vous voyez `Serveur HTTP en écoute addr=127.0.0.1:38440`, c'est réussi. Laissez le terminal ouvert et ouvrez-en un nouveau.

### 1.3 Tester

```powershell
curl.exe -X POST http://127.0.0.1:38440/v1/responses `
  -H "Content-Type: application/json" `
  -d '{\"model\": \"moonbridge\", \"input\": \"Bonjour, présentez-vous en une phrase.\"}'
```

**Validation :** La réponse contient `"status": "completed"`.

**Problèmes courants :**

| Problème | Cause | Solution |
|----------|-------|----------|
| `command not found: go` | Go non installé | Voir recette 0 |
| `connection refused` | Service pas démarré | Vérifier le premier terminal |
| `invalid yaml` / `cannot unmarshal` | Erreur d'indentation | YAML : 2 espaces par niveau |
| `401 unauthorized` | api_key incorrecte | Vérifier la clé DeepSeek |
| `402 payment required` | Solde insuffisant | Recharger DeepSeek |
| Le service plante | Dépendances manquantes | Premier lancement nécessite internet |

---

## 2. Connecter Codex CLI à Moon Bridge

**Objectif :** Codex CLI utilise Moon Bridge pour appeler DeepSeek.

**Ingrédients :**
- Recette 1 réussie
- Codex CLI installé (`npm install -g @openai/codex`)

**Étapes :**

Moon Bridge intègre un générateur de configuration Codex. Vérifiez d'abord qu'il tourne :

```powershell
curl.exe http://127.0.0.1:38440/health
```

Générez `config.toml` et `models_catalog.json` :

```powershell
$CODEX_HOME_DIR = "$env:CODEX_HOME\config"
New-Item -ItemType Directory -Force -Path "$CODEX_HOME_DIR"
go run .\cmd\moonbridge -config config.yml `
  -print-codex-config moonbridge `
  -codex-home "$CODEX_HOME_DIR"
```

**Note :** Entrez les commandes une par une, ne les copiez pas toutes en même temps, pour éviter des erreurs de formatage du toml.

Cela crée deux fichiers dans `$CODEX_HOME_DIR` :
- `config.toml` — Configuration du fournisseur Codex
- `models_catalog.json` — Description des capacités du modèle

Démarrez Codex :

```powershell
codex "Bonjour"
```

**Validation :** Codex démarre normalement, Moon Bridge affiche `POST /v1/responses`.

**Problèmes courants :**

| Problème | Cause | Solution |
|----------|-------|----------|
| `connection refused` | Moon Bridge pas démarré | D'abord exécuter la recette 1 |
| Erreur incompréhensible | Pas de `models_catalog.json` | Vérifier le chemin `--codex-home` |

---

## 3. Changer de fournisseur

**Objectif :** Passer de DeepSeek à un autre modèle (ex: Anthropic).

**Ingrédients :** Recette 1 réussie + Clé API du nouveau fournisseur.

**Étapes :**

Remplacez le contenu de `providers` dans `config.yml` :

```yaml
providers:
  anthropic:
    base_url: "https://api.anthropic.com"
    api_key: "sk-ant-votre-clé"
    models:
      claude-model:
        upstream_model: "claude-sonnet-4-20250514"
```

```yaml
routes:
  moonbridge: claude-model@anthropic
```

Redémarrez Moon Bridge (Ctrl+C, puis `go run`), testez avec curl.

**Validation :** Même requête, la réponse a le ton de Claude.

> Modifiez seulement `config.yml`, pas besoin de modifier la configuration Codex.

---

## 4. Activer les capacités de raisonnement DeepSeek V4

**Objectif :** Activer le thinking_mode (raisonnement profond) de DeepSeek V4.

**Ingrédients :** Accès DeepSeek V4 + Recette 1 réussie.

**Étapes :**

```yaml
models:
  deepseek-v4:
    context_window: 65536
    output_max: 8192
    default_reasoning_level: "high"
    supported_reasoning_levels:
      - effort: "low"    description: "Raisonnement faible"
      - effort: "medium" description: "Raisonnement moyen"
      - effort: "high"   description: "Raisonnement élevé"
      - effort: "xhigh"  description: "Raisonnement très élevé"
    supports_reasoning_summaries: true
    extensions:
      deepseek_v4:
        enabled: true

providers:
  deepseek:
    base_url: "https://api.deepseek.com"
    api_key: "sk-votre-clé"
    models:
      deepseek-v4:
        upstream_model: "deepseek-chat"

routes:
  moonbridge: deepseek-v4@deepseek
```

Redémarrez Moon Bridge.

**Validation :** Ajoutez `"reasoning": {"effort": "high"}` à la requête curl, les réponses aux questions complexes incluent le raisonnement.

> `xhigh` est mappé au niveau `max` de DeepSeek, raisonnement plus profond, plus lent et plus coûteux.

---

## 5. Permettre au modèle de voir des images (extension Visual)

**Objectif :** Un modèle principal textuel délègue via Visual le traitement des images à un modèle visuel.

**Ingrédients :**
- Recette 1 réussie
- Un fournisseur visuel supportant Anthropic (ex: Kimi `api.moonshot.cn`)
- Deux clés API

**Étapes :**

```yaml
models:
  deepseek-model:
    context_window: 65536
    output_max: 8192
    input_modalities:
      - "text"
      - "image"
    extensions:
      visual:
        enabled: true

providers:
  deepseek:
    base_url: "https://api.deepseek.com"
    api_key: "sk-votre-clé-DeepSeek"
    models:
      deepseek-model:
        upstream_model: "deepseek-chat"

  kimi:
    base_url: "https://api.moonshot.cn"
    api_key: "sk-votre-clé-Kimi"
    models:
      kimi-model:
        upstream_model: "kimi-for-coding"

routes:
  moonbridge: deepseek-model@deepseek

extensions:
  visual:
    enabled: true
    config:
      provider: "kimi"
      model: "kimi-for-coding"
      max_rounds: 4
      max_tokens: 2048
```

Redémarrez Moon Bridge.

**Validation :** Envoyez une requête avec une image, le modèle décrit le contenu.

---

## 6. Activer la recherche Web

**Objectif :** Le modèle peut effectuer des recherches en ligne.

**Ingrédients :** Recette 1 réussie + Clé API Tavily ([tavily.com](https://tavily.com)).

**Étapes :**

```yaml
web_search:
  support: auto
  tavily_api_key: "tvly-votre-clé"

providers:
  deepseek:
    base_url: "https://api.deepseek.com"
    api_key: "sk-votre-clé"
    models:
      deepseek-model:
        upstream_model: "deepseek-chat"
        web_search:
          support: auto

routes:
  moonbridge: deepseek-model@deepseek
```

Redémarrez Moon Bridge.

**Validation :** Question d'actualité (ex: "météo aujourd'hui"), réponse avec sources.

> `support` : `auto` (auto), `enabled` (forcé), `disabled` (désactivé), `injected` (Tavily/Firecrawl).

---

## 7. Activer le cache Prompt

**Objectif :** Réduire la consommation de tokens.

**Ingrédients :** Un fournisseur avec protocole Anthropic.

**Étapes :**

```yaml
cache:
  mode: "explicit"
  ttl: "5m"
```

Ajoutez à `config.yml`, redémarrez.

> `mode` : `off` (désactivé), `automatic` (auto), `explicit` (manuel, recommandé), `hybrid` (tout).

---

## 8. Dépannage rapide

### Indentation YAML

2 espaces, pas de Tab :

```yaml
# Erreur
  base_url: "..."    # 4 espaces

# Correct
  base_url: "..."    # 2 espaces
```

### Le service ne démarre pas

| Erreur | Cause |
|--------|-------|
| `no such file or directory` | Chemin config.yml incorrect |
| `cannot unmarshal` | Erreur format YAML |
| `unsupported protocol` | protocol invalide |
| `connection refused` | base_url fournisseur inaccessible |
| `401` / `403` | Clé API incorrecte |
| `402` | Solde DeepSeek insuffisant |
| `rate limit` | Trop de requêtes |

### curl ne fonctionne pas

```powershell
curl.exe http://127.0.0.1:38440/health
curl.exe http://127.0.0.1:38440/v1/models
```

Pas de sortie = Moon Bridge ne tourne pas.

### Visual ne fonctionne pas

- Vérifier que `extensions.visual.config.provider` existe
- Vérifier que le fournisseur visuel supporte Anthropic
- Vérifier `visual.enabled: true` sur le modèle principal

---

## Contribuer une recette

Vous avez une configuration utile ? Proposez une PR.
