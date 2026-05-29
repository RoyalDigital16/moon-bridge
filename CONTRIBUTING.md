# Guide de contribution

Merci de votre intérêt pour Moon Bridge ! Les contributions via Issues et Pull Requests sont les bienvenues.

## Signaler un problème

- Utilisez GitHub Issues pour soumettre
- Veuillez inclure : environnement d'exécution, configuration (après anonymisation), étapes de reproduction, comportement attendu et réel
- Si le problème implique une erreur API, joignez les journaux de traçage des requêtes (activez `trace.enabled: true`)

## Soumettre du code

### Stratégie de branches

- `main` — version stable
- `dev` — branche de développement, toutes les PR sont fusionnées ici
- `fix/*` — branches de correction
- `feat/*` — branches de fonctionnalités

### Processus de développement

1. Forkez le dépôt et créez une branche de fonctionnalité : `git checkout -b feat/my-feature`
2. Écrivez le code et ajoutez des tests
3. Exécutez tous les tests : `go test ./...`
4. Soumettez une PR vers la branche `dev`

### Conventions de code

- Utilisez `log/slog` pour la journalisation structurée
- Les noms de fichiers reflètent leur responsabilité (ex: `candidate_routing_test.go`), sans numéro de gestion de projet
- La conversion de protocole utilise `format.CoreRequest` / `CoreResponse` comme représentation intermédiaire
- Tout nouvel adaptateur doit implémenter à la fois `ProviderAdapter` et `ProviderStreamAdapter`

### Exigences de test

- Les tests unitaires couvrent le nouveau code
- La conversion de protocole doit inclure des tests E2E (`internal/e2e/`)
- Exécutez `make test` pour garantir l'absence de régression

## Ajouter un nouveau fournisseur

1. Ajoutez une constante de protocole dans `internal/config/config.go` (ex: `ProtocolMyAdapter`)
2. Créez le package `internal/protocol/<adapter>/` implémentant `format.ProviderAdapter` et `format.ProviderStreamAdapter`
3. Enregistrez l'adaptateur dans le `format.Registry` via `internal/service/app/app.go`
4. Ajoutez une branche de distribution du protocole dans `internal/service/server/adapter_dispatch.go`
5. Ajoutez des tests E2E dans `internal/e2e/`

## Licence

Ce projet est sous licence [GPL v3](LICENSE). En soumettant du code, vous acceptez qu'il soit publié sous cette licence.
