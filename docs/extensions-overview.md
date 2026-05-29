# Aperçu des Extensions existantes

## deepseek_v4 (Extension DeepSeek V4)

Extension permettant aux modèles DeepSeek V4 de fonctionner correctement via un point d'accès compatible Anthropic. DeepSeek V4 implémente un sous-ensemble de l'API Anthropic Messages, avec quelques différences notables à gérer.

**Emplacement** : `internal/extension/deepseek_v4/`

**Fichiers** :

| Fichier | Rôle |
|---------|------|
| `plugin.go` | Implémentation du plugin, enregistre toutes les capacités |
| `deepseek_v4.go` | Fonctions de conversion principales (gestion de reasoning_content) |
| `state.go` | Gestion de l'état du cache thinking |

**Capacités implémentées** :

```go
var _ plugin.InputPreprocessor   = (*DSPlugin)(nil)
var _ plugin.RequestMutator      = (*DSPlugin)(nil)
var _ plugin.MessageRewriter     = (*DSPlugin)(nil)
var _ plugin.ContentFilter       = (*DSPlugin)(nil)
var _ plugin.ContentRememberer   = (*DSPlugin)(nil)
var _ plugin.ThinkingPrepender   = (*DSPlugin)(nil)
var _ plugin.ReasoningExtractor  = (*DSPlugin)(nil)
var _ plugin.StreamInterceptor   = (*DSPlugin)(nil)
var _ plugin.ErrorTransformer    = (*DSPlugin)(nil)
var _ plugin.SessionStateProvider = (*DSPlugin)(nil)
// Assertion d'interface à la compilation (plugin.go)
```

**Détail des capacités** :

`PreprocessInput()` — Supprime le champ `reasoning_content` des messages d'entrée. DeepSeek retourne une erreur 400 si `reasoning_content` apparaît dans les messages d'entrée, car ce champ est réservé à la sortie.

`MutateRequest()` — Appelle `ToAnthropicRequest()` pour adapter la requête à DeepSeek :
- Vide `Temperature` et `TopP` (DeepSeek peut refuser ces paramètres)
- Mappe OpenAI `reasoning.effort` vers Anthropic `output_config.effort` (`high` → `high`, `xhigh`/`max` → `max`)

`RewriteMessages()` — Injecte optionnellement des instructions renforcées (reinforce prompt) avant les messages utilisateur, pour rappeler au modèle de respecter le system prompt et AGENTS.md.

**Reconstruction de l'historique thinking** :
C'est la partie la plus essentielle de l'extension DeepSeek V4, résolvant le problème de **reconstruction de l'historique thinking**.

**Problème** : Lors d'une conversation continue avec DeepSeek V4, l'API exige que l'historique d'entrée contienne les blocs `thinking` précédents (blocs `ContentBlock` de type `"thinking"` dans le protocole Anthropic), sinon une erreur est retournée.

**Limitation de Codex** : Codex ne conserve dans l'API Conversations que le résumé `reasoning` (`OutputItem.Type: "reasoning"`), pas le texte thinking complet.

**Solution** (en quatre étapes) :
1. Lors de la réponse (ContentFilter) → Intercepte les blocs thinking amont, les extrait comme résumé reasoning
2. Lors de la mémorisation (ContentRememberer) → Met en cache les blocs thinking dans SessionData par tool_call_id / text_hash
3. Lors du rejeu (ThinkingPrepender + ReasoningExtractor) → À la requête suivante :
   a. Tente d'abord de restaurer le bloc thinking original depuis le résumé reasoning (Encode/DecodeThinkingSummary)
   b. Sinon, cherche le thinking mis en cache dans SessionData par tool_call_id
   c. En dernier recours, insère un bloc thinking vide
4. Apprentissage continu (StreamInterceptor) → Capture également le thinking en mode streaming et le met en cache

**StreamInterceptor** :
Intercepte les événements `thinking_delta` / `reasoning_content_delta` en streaming, accumule le texte thinking complet, et le met en cache dans l'état de session à la fin du flux.

**ErrorTransformer** :
Traite les messages d'erreur spécifiques à DeepSeek. Convertit les erreurs concernant le "thinking mode" en messages plus lisibles.

**SessionStateProvider** :
Crée une instance `*State` pour le cache inter-requêtes des blocs thinking. L'état maintient deux mappages LRU :
- `records` : blocs thinking indexés par `tool_use_id` (max 1024 entrées)
- `textRecords` : blocs thinking indexés par SHA256 du texte assistant (max 1024 entrées)

### Activation

Dans la configuration du modèle, définissez `extensions.deepseek_v4.enabled: true` :

```yaml
models:
  my-model:
    extensions:
      deepseek_v4:
        enabled: true
```

Ou via les routes :

```yaml
routes:
  my-alias:
    model: my-model
    provider: my-provider
    # les routes héritent automatiquement des paramètres deepseek_v4 de la configuration du modèle
```

La fonction `EnabledForModel` du plugin vérifie via `Config.ExtensionEnabled("deepseek_v4", model)` si l'alias du modèle active cette extension.

## web_search_injected (Module Web Search injecté)

Quand le fournisseur amont ne supporte pas le server tool natif Anthropic `web_search_20250305`, Moon Bridge peut utiliser le mode "injecté" — les outils `tavily_search` et `firecrawl_fetch` sont injectés comme des function-type tools dans la requête, et le serveur exécute automatiquement la recherche.

**Emplacement** : `internal/extension/websearchinjected/`

Dans le chemin d'exécution actuel, ce n'est pas un plugin interne indépendant enregistré par `BuiltinExtensions()` ; bridge/server appelle directement `InjectTools()` et `WrapProvider()` de ce module selon le mode web search résolu du modèle. `plugin.go` conserve une implémentation d'interface plugin, principalement pour les limites du module et les tests.

**Fichiers** :

| Fichier | Rôle |
|---------|------|
| `plugin.go` | Implémentation du plugin |
| `websearchinjected.go` | Fonctions d'outil principales |

**Capacités implémentées** :

```go
var _ plugin.ToolInjector     = (*Plugin)(nil)
var _ plugin.ProviderWrapper  = (*Plugin)(nil)
```

### Flux de travail

1. La requête Codex contient l'outil `web_search_preview`
2. Bridge vérifie le mode Web Search du modèle → "injected"
3. Bridge appelle `websearch.Tools()` / `websearchinjected.InjectTools()` pour injecter :
   - tavily_search (function tool)
   - firecrawl_fetch (function tool, si une clé Firecrawl est configurée)
4. Le `maybeWrapProvider()` du serveur, quand le mode résolu est `injected`, appelle `websearchinjected.WrapProvider()` pour encapsuler le client amont en Orchestrator
5. Après l'envoi de la requête :
   a. Si l'amont retourne des appels d'outils (tavily_search/firecrawl_fetch)
   b. L'Orchestrator exécute automatiquement la recherche Tavily ou le crawlage Firecrawl
   c. Les résultats sont ajoutés comme tool_result à la requête suivante
   d. Le processus se répète jusqu'à ce que le modèle soit satisfait ou que le nombre maximum de tours soit atteint

`websearch.NewInjectedOrchestrator()` crée un orchestrateur de recherche qui encapsule `*anthropic.Client` et expose les mêmes interfaces `CreateMessage` / `StreamMessage`. L'orchestrateur exécute les outils de recherche en boucle jusqu'à ce que le modèle n'en demande plus ou que `SearchMaxRounds` soit atteint.

### Configuration

```yaml
providers:
  my-provider:
    web_search:
      support: injected
      tavily_api_key: "tvly-..."
      firecrawl_api_key: "fc-..."
      search_max_rounds: 3
```

Ou globalement :

```yaml
web_search:
  support: injected
  tavily_api_key: "tvly-..."
  firecrawl_api_key: "fc-..."
  search_max_rounds: 3
```

Surcharge au niveau du modèle :

```yaml
models:
  my-model:
    web_search:
      support: auto   # auto / enabled / disabled / injected
```

## kimi_workaround (Limitation des tours d'appels d'outils Kimi)

Les modèles Kimi peuvent parfois tomber dans une boucle infinie de collecte d'informations lors des appels d'outils. Le plugin `kimi_workaround` injecte des indicateurs de progression et des limites pour rappeler au modèle de résumer et d'arrêter les appels d'outils lorsqu'il approche du nombre maximal de tours.

**Emplacement** : `internal/extension/kimi_workaround/`

**Fichiers** :

| Fichier | Rôle |
|---------|------|
| `plugin.go` | Implémentation du plugin, enregistre toutes les capacités |

**Capacités implémentées** :
- `InputPreprocessor` — Prétraite les messages d'entrée
- `ContentFilter` — Filtre le contenu de la réponse
- `ContentRememberer` — Mémorise les blocs de contenu pour le suivi des tours
- `StreamInterceptor` — Interception des événements de flux et suivi des tours
- `SessionStateProvider` — Fournit l'état des tours entre les requêtes

### Activation

Dans la configuration du modèle, définissez `extensions.kimi_workaround.enabled: true` :

```yaml
extensions:
  kimi_workaround:
    enabled: true
    config:
      max_rounds: 8
      warn_rounds: 5
      warn_message: "Progression : tour %d/%d. Veuillez résumer et terminer dès que possible."
      limit_message: "Limite de tours atteinte (%d). Veuillez conclure avec les informations actuelles."
```

Configuration globale :

```yaml
extensions:
  kimi_workaround:
    enabled: true
    config:
      max_rounds: 10
```

Les modèles peuvent surcharger avec :

```yaml
models:
  my-model:
    extensions:
      kimi_workaround:
        enabled: true
```

## codex (Kit de compatibilité Codex)

Bien qu'il ne s'agisse pas d'un plugin au sens traditionnel, `internal/extension/codex/` est une partie importante du système d'Extension.

**Emplacement** : `internal/extension/codex/`

**Fichiers** :

| Fichier | Rôle |
|---------|------|
| `catalog.go` | Génération du catalogue de modèles DTO, génération de config.toml Codex |
| `default_instructions.go` | Modèle d'instructions par défaut (intègre default_instructions.txt) |

### Responsabilités principales

1. **Catalogue de modèles** : Génère `models_catalog.json` et `config.toml` utilisables par Codex CLI à partir de la configuration
2. **Injection d'instructions par défaut** : Fournit des instructions système par défaut adaptées à Codex pour les modèles

### Intégration CLI

Génération de configuration Codex via la ligne de commande moonbridge :

```bash
# Générer le catalogue de modèles et config.toml
moonbridge -print-codex-config my-model

# Générer dans un répertoire spécifique
moonbridge -print-codex-config my-model -codex-home ~/codex
```

## visual (Extension visuelle)

Quand le modèle principal n'a pas de capacités visuelles multimodales, Moon Bridge peut déléguer l'analyse d'images à un fournisseur visuel dédié. L'extension `visual` fonctionne comme un plugin `ToolInjector`, injectant les outils `visual_brief` et `visual_qa` dans la conversation du modèle principal ; la couche Serveur encapsule le fournisseur amont en `CoreProvider` via `wrapWithVisual()`, interceptant les appels d'outils visuels au niveau Core et les déléguant au fournisseur visuel configuré.

**Emplacement** : `internal/extension/visual/`

**Fichiers** :

| Fichier | Rôle |
|---------|------|
| `plugin.go` | Implémentation du plugin, injecte les outils `visual_brief` / `visual_qa`, expose ConfigForModel |
| `orchestrator.go` | Orchestrateur visuel (ancien mode d'encapsulation du Provider Anthropic) |
| `core_orchestrator.go` | Orchestrateur au niveau Core (actuellement utilisé) |
| `client.go` | Définition de l'interface CoreProvider et implémentation BridgeClient |
| `tools.go` | Définition des outils et génération de schémas |
| `types.go` | Définitions de types |
| `legacy.go` | Code legacy |

**Capacités implémentées** :

```go
var _ plugin.ToolInjector        = (*VisualPlugin)(nil)
var _ plugin.ConfigSpecProvider  = (*VisualPlugin)(nil)
```

### Flux de travail

1. La requête arrive au serveur, l'orchestrateur visuel encapsule le fournisseur amont
2. L'orchestrateur analyse les messages de la requête à la recherche de blocs image Anthropic, les remplace par des placeholders texte `Image #1`, `Image #2`, etc.
3. Le modèle principal traite la requête, peut appeler les outils `visual_brief` / `visual_qa`
4. L'orchestrateur intercepte les appels d'outils :
   - Extrait les `image_refs` et `image_urls` des paramètres d'outil
   - Associe les images correspondantes depuis `availableImages` précédemment sauvegardé
   - Envoie au fournisseur visuel via `VisionClient.Analyze()`
   - Le fournisseur visuel retourne les résultats d'analyse
5. Les résultats d'analyse sont retournés comme `tool_result` au modèle principal
6. Le modèle principal peut utiliser les résultats pour continuer le raisonnement, ou rappeler `visual_qa` pour des questions supplémentaires

### Fournisseur visuel

L'analyse visuelle est exécutée via l'interface `VisionClient`. L'implémentation intégrée `BridgeClient` utilise un fournisseur compatible Anthropic indépendant pour envoyer les requêtes d'analyse d'images. Vous pouvez utiliser n'importe quel fournisseur supportant le multimodal (Kimi, GPT-4o, etc.) comme backend visuel.

```go
type VisionClient interface {
    Analyze(ctx context.Context, req *VisionRequest) (*VisionResponse, error)
}
```

### Configuration

```yaml
extensions:
  visual:
    enabled: true
    config:
      provider: "kimi"           # Clé du fournisseur visuel
      model: "kimi-for-coding"    # Modèle visuel
      max_rounds: 4               # Nombre max de tours de questions visuelles
      max_tokens: 2048            # Max tokens pour la réponse visuelle
```

### Interaction avec le fournisseur

L'orchestrateur visuel travaille au niveau Core — via `wrapWithVisual()` (défini dans `internal/service/server/adapter_dispatch.go`), il encapsule le fournisseur amont en `CoreProvider`, interceptant les requêtes/réponses au format Core. Quand le modèle principal ne supporte pas les images et appelle les outils visuels, l'orchestrateur envoie automatiquement la requête d'image au fournisseur visuel configuré et retourne les résultats d'analyse.

## En développement : db_sqlite (Provider de persistance SQLite)

Extension de backend de base de données pour les processus locaux. Cette capacité provient du travail de persistance de la branche dev, actuellement considérée comme une capacité en développement, pas une interface publique stable.

**Emplacement** : `internal/extension/db/sqlite/`

**Capacités implémentées** :

```go
var _ plugin.DBProvider            = (*Plugin)(nil)
var _ plugin.ConfigSpecProvider    = (*Plugin)(nil)
```

Exemple de configuration :

```yaml
extensions:
  db_sqlite:
    enabled: true
    config:
      path: ./data/moonbridge.db  # Chemin du fichier SQLite
      wal: true                   # Activer WAL
      busy_timeout_ms: 5000       # Timeout de busy
      max_open_conns: 1           # Connexions max
```

Quand `path` est vide ou `enabled: false`, la base de données n'est pas fournie. WAL activé par défaut, busy timeout par défaut 5000 ms, max connexions par défaut 1.

## En développement : db_d1 (Provider de persistance Cloudflare D1)

Extension de backend de base de données pour l'environnement Cloudflare Worker. Cette capacité provient de la branche dev, dépend de l'injection de base de données par le point d'entrée Worker.

**Emplacement** : `internal/extension/db/d1/`

Le provider D1 n'importe pas directement le SDK Cloudflare Workers. C'est le point d'entrée Worker qui appelle `InjectDB()` avant l'initialisation pour injecter `*sql.DB`. Dans un processus local normal, même avec un binding configuré, il reste indisponible faute d'injection de base de données.

Exemple de configuration :

```yaml
extensions:
  db_d1:
    enabled: true
    config:
      binding: "DB"
```

## En développement : metrics (Extension de métriques de requêtes)

Enregistre pour chaque requête le modèle, le modèle amont réel, les tokens, le coût, le statut, les messages d'erreur et la durée, et fournit une interface d'interrogation quand la base de données est disponible. Cette capacité provient du travail de persistance/observabilité de la branche dev, actuellement pas considérée comme une interface publique stable.

**Emplacement** : `internal/extension/metrics/`

**Capacités implémentées** :

```go
var _ plugin.DBConsumer           = (*Plugin)(nil)
var _ plugin.ConfigSpecProvider   = (*Plugin)(nil)
var _ plugin.RouteRegistrar       = (*Plugin)(nil)
```

Exemple de configuration :

```yaml
extensions:
  metrics:
    enabled: true
    config:
      default_limit: 100    # Limite par défaut pour les requêtes
      max_limit: 1000       # Limite maximale
```

Quand metrics est lié avec succès au store de base de données, il enregistre `GET /v1/admin/metrics`. Supporte les paramètres de requête `limit`, `offset`, `model`, `status`, `since`, `until`, `order=asc`.

```yaml
providers:
  my-provider:
    offers:
      - model: my-model
        web_search:
          support: "enabled"  # Surcharge le niveau injected du fournisseur
```
