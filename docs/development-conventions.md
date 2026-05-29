# Conventions de développement

## Nommage des fichiers

- Les noms de fichiers doivent refléter leur responsabilité
- Utilisez des noms descriptifs comme `candidate_routing_test.go`, pas de numéros de gestion de projet
- Les fichiers de test utilisent le suffixe `_test.go`

## Journalisation

- Utilisez `log/slog` pour la journalisation structurée
- Préférez les attributs structurés aux messages formatés
- Niveaux de log : `Debug`, `Info`, `Warn`, `Error`

## Gestion de la configuration

- La configuration au niveau package est gérée via `internal/config`
- Les valeurs par défaut sont définies dans les structures de configuration
- Les modifications de configuration sont appliquées via rechargement à chaud

## Conversion de protocole

- Utilisez `CoreRequest` / `CoreResponse` de `internal/format` comme représentation intermédiaire
- Tout nouvel adaptateur doit implémenter `ProviderAdapter` et `ProviderStreamAdapter`
- La conversion doit être symétrique (aller et retour)

## Tests

- Les tests unitaires couvrent le nouveau code
- La conversion de protocole doit inclure des tests E2E
- Exécutez `make test` pour garantir l'absence de régression

## Extensions

- Les extensions sont enregistrées via `BuiltinExtensions()` dans `internal/service/app/`
- Chaque extension implémente l'interface `plugin.Plugin`
- Les configurations d'extensions sont déclarées via `ConfigSpecProvider`
