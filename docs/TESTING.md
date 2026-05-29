# Tests

## Exécuter les tests

```bash
# Tous les tests unitaires
go test ./...

# Tests d'un package spécifique
go test ./internal/protocol/anthropic/...

# Avec sortie détaillée
go test -v ./internal/service/server/...
```

## Tests E2E

Les tests E2E se trouvent dans `internal/e2e/` et `internal/service/e2e/`.

```bash
# Tests E2E en mode Mock (sans clé API)
go test ./internal/e2e/ -mock

# Tests E2E pour un fournisseur spécifique
go test ./internal/e2e/ -run TestAnthropic -v
go test ./internal/e2e/ -run TestOpenAI -v

# Tests E2E de la couche service
go test ./internal/service/e2e/ -v
```

## Tests avec le Makefile

```bash
make test       # Tests unitaires
make test-e2e   # Tests E2E
```

## Couverture de code

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Écrire des tests

- Les tests unitaires doivent couvrir le nouveau code
- La conversion de protocole doit inclure des tests E2E
- Exécutez `make test` pour garantir l'absence de régression
- Utilisez le mode Mock pour les tests E2E sans clé API réelle
