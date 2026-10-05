# nself-plugin-dev

Plugin author tools for nSelf, broken out of the core `nself plugin` command (P7-CANON-18). Free, MIT.

```bash
nself add plugin-dev
nself plugin-dev init my-plugin            # scaffold (go, rust, node, static templates)
nself plugin-dev link ./my-plugin          # register the directory as a local shadow
nself plugin-dev dev my-plugin             # link + hot reload (air, or fswatch)
nself plugin-dev debug my-plugin           # build and attach dlv (ports 2345-2399)
nself plugin-dev test my-plugin            # unit tests + smoke install/uninstall
nself plugin-dev unlink my-plugin
```

`new` is the deprecated alias of `init` and prints core's deprecation notice. `scaffold` is accepted as an alias of `init`.

## Same behaviour as core

Every subcommand reproduces the core `nself plugin <command>` of the same name: flags and defaults, messages, exit codes
and file effects. The `--help` text is captured from a built core binary (`scripts/capture-core-surface.sh`), not retyped.
`scripts/parity-with-core.sh` builds core `nself` from cli `origin/main` and compares stdout, stderr, exit code, the
commands run (recorded by stub `go`, `docker`, `dlv`, `air`) and the file effects of each case.

File effects (the only paths the commands write):

| Command | Writes |
|---------|--------|
| `init`, `new` | the scaffolded tree under `./<name>` or `--out` |
| `link`, `unlink`, `dev` (auto-link) | `~/.nself/plugin-links.json` (0600) |
| `dev` | `$TMPDIR/nself-dev-watch.sh` when no SDK devkit script exists; `.air.toml` in the plugin directory (from that script) |
| `debug` | `$TMPDIR/nself-debug-<name>` (the debug build) |
| `test` | whatever `go test`, `docker run`, `nself plugin install|remove` do |

## Known differences from core

- The usage path in `--help` reads `nself plugin-dev <command>`; the rest is byte-identical.
- `dev --debug` re-executes this binary as `debug <name>` (core re-executes itself as `plugin debug <name>`).
- `test` runs the smoke phases through `nself` found on `PATH` (core uses its own executable, which a plugin cannot).
- `link --list` prints rows sorted by name (core prints Go map order).
- Global flags: `--no-monorepo` and `--no-deprecation-warnings` are accepted and ignored. `--json` is refused by the
  `nself` root for commands whose manifest says `json: none`; run on its own, this binary reports an unknown flag.
- Temporary duplicate: `internal/scaffold`, `internal/clui` and `internal/links` copy cli code (ports do not import cli
  internals). They are deleted with the core copies at P7-SHIP-09.

## Develop

```bash
cd free/plugin-dev
CGO_ENABLED=0 go test -count=1 ./...
NSELF=<checkout holding cli/> TMPDIR=<scratch> bash scripts/parity-with-core.sh
```

The binary has no third-party dependencies.
