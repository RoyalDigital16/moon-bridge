# API HTTP

## Points d'accès principaux

| Point d'accès | Méthode | Description |
|---------------|---------|-------------|
| `/health` | GET | Vérification de l'état de santé |
| `/v1/responses` | POST | API OpenAI Responses (point d'entrée principal) |
| `/responses` | POST | Idem (sans préfixe `/v1`) |
| `/v1/models` | GET | Liste des modèles disponibles |
| `/models` | GET | Idem |

## API de gestion (nécessite persistance activée)

| Point d'accès | Méthode | Description |
|---------------|---------|-------------|
| `/api/v1/config` | GET/PUT | Obtenir/mettre à jour la configuration runtime |
| `/api/v1/models` | GET | Liste des définitions de modèles |
| `/api/v1/models/{slug}` | GET/PUT/DELETE | Gérer un modèle spécifique |
| `/api/v1/providers` | GET/POST/DELETE | Gérer les fournisseurs |
| `/api/v1/providers/{key}` | GET/PUT/DELETE | Gérer un fournisseur spécifique |
| `/api/v1/providers/{key}/offers/{model}` | PATCH/DELETE | Gérer les offres de modèles |
| `/api/v1/routes` | GET | Liste des routes |
| `/api/v1/routes/{alias}` | GET/PUT/DELETE | Gérer une route spécifique |
| `/api/v1/changes/preview` | GET | Prévisualiser les changements en attente |
| `/api/v1/changes/apply` | POST | Appliquer les changements en attente |
| `/api/v1/changes/discard` | POST | Abandonner les changements en attente |
| `/api/v1/settings` | GET/PUT | Paramètres généraux |
| `/api/v1/extensions` | GET | Liste des extensions disponibles |
| `/api/v1/extensions/{name}` | GET/PUT | Gérer la configuration d'une extension |
| `/api/v1/export` | GET | Exporter la configuration |
| `/api/v1/import` | POST | Importer une configuration |
| `/v1/admin/metrics` | GET | Métriques des requêtes (si l'extension metrics est activée) |

## Authentification

L'authentification se fait via l'en-tête `Authorization: Bearer <token>`. Le token est configuré dans `server.auth_token`. Si le token est vide, l'authentification est désactivée.

## Codes d'erreur

| Code HTTP | Signification |
|-----------|--------------|
| 200 | Succès |
| 400 | Requête invalide |
| 401 | Non authentifié |
| 404 | Ressource non trouvée |
| 409 | Conflit (ex: référence existante) |
| 500 | Erreur interne du serveur |
| 502 | Bad Gateway (échec fournisseur amont) |
| 503 | Service indisponible |
