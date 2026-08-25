# nself-dr

Disaster recovery, moved out of the CLI core under CLI-R11.

## Install

```bash
nself install dr
```

## Commands

```bash
nself dr promote
nself dr fence
nself dr drill
nself dr install-units
```

`nself dr promote` against a production project asks you to type the project
name before it proceeds. That prompt reads from stdin, which the plugin inherits
from nself, so it still reaches you.

## Configuration

Reads `PROJECT_NAME`, `BASE_DOMAIN`, `BACKUP_DIR`, `DR_STANDBY_HOST` and
`REDIS_ENABLED` from the environment. nself resolves the `.env` cascade and
passes them in.

## License

MIT.
