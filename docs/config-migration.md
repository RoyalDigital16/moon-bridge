# Migration de configuration

Ce document décrit les migrations entre différentes versions de la configuration Moon Bridge.

## Utilitaires de migration

Des scripts de migration sont disponibles dans `scripts/` :

```bash
# Migration de la configuration existante
python scripts/migrate_config.py --input config.old.yml --output config.yml

# Migration vers la version 5 du format
python scripts/migrate_config_v5.py --input config.yml
```

## Changements par version

### v5
- Restructuration de la section `extensions`
- Nouveau format pour les configurations de plugins

### v4
- Ajout du support multi-protocole
- Introduction de la section `providers` avec champ `protocol`

### v3
- Migration de `developer.proxy` vers `proxy`

## Notes

- Faites toujours une sauvegarde de votre configuration avant la migration
- Validez la configuration migrée avec `-dump-config-schema`
- Consultez [CONFIGURATION.md](CONFIGURATION.md) pour la documentation complète du format actuel
