# Déploiement

## Binaire autonome

```bash
# Compiler
go build -o moonbridge ./cmd/moonbridge

# Exécuter
./moonbridge -config config.yml
```

## Docker

```bash
# Construire l'image
docker build -t moonbridge .

# Exécuter
docker run -p 38440:38440 \
  -v $(pwd)/config.yml:/config/config.yml \
  moonbridge
```

### Docker Compose

```bash
docker compose -f docker-compose.example.yml up
```

## Cloudflare Worker

```bash
# Compiler en WASM
GOOS=wasip1 GOARCH=wasm go build -o build/cloudflare.wasm ./cmd/cloudflare

# Déployer
npx wrangler deploy
```

Le Worker nécessite une configuration d'authentification en production. Définissez un token Bearer dans `server.auth_token` ou injectez une configuration via `wrangler secret put MOONBRIDGE_CONFIG`.

## Variables d'environnement

| Variable | Description |
|----------|-------------|
| `XDG_CONFIG_HOME` | Répertoire de configuration (par défaut : `~/.config`) |
| `CODEX_HOME` | Répertoire de configuration Codex |

## Persistance des données

Les données persistantes (SQLite) sont stockées dans le chemin configuré dans `extensions.db_sqlite.config.path`. Par défaut : `./data/moonbridge.db`.

Les fichiers de trace sont stockés dans `data/trace/`.
