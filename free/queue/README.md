# nself-queue

Background job queue inspection, moved out of the CLI core under CLI-R11.

## Install

```bash
nself install queue
```

## Commands

```bash
nself queue status
nself queue retry
nself queue purge
```

## Configuration

Reads the Postgres connection from the environment. nself resolves the `.env`
cascade and passes in `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`,
`POSTGRES_USER` and `POSTGRES_PASSWORD`.

## License

MIT.
