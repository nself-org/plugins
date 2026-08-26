# nself-ai-cli

The `nself ai` command family, moved out of the CLI core under CLI-R11.

Named `ai-cli` rather than `ai` because the paid `ai` service plugin already
owns that name. The command is still `nself ai` — the CLI proxies it here.

## Install

```bash
nself install ai-cli
```

## Commands

```bash
nself ai chat
nself ai local health
nself ai local models
nself ai pool status
nself ai pool rotate
```

## Related

`nself doctor` keeps its AI checks whether or not this plugin is installed, and
suggests the relevant `nself ai` commands once it is.

## License

MIT.
