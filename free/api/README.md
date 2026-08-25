# nself-api

Plugin API surface inspection, moved out of the CLI core under CLI-R11.

## Install

```bash
nself install api
```

## Commands

```bash
nself api probe
nself api deprecations
nself api changelog
```

## Configuration

Reads `NSELF_PLUGIN_DIR` from the environment when set; nself resolves the
`.env` cascade and passes it in.

This plugin carries its own copy of the deprecation registry, because it reads
that registry and the CLI embeds it.

## License

MIT.
