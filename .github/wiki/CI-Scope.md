# CI scope: what runs on a pull request

A pull request runs only the legs it can break. Push to `main`, `workflow_dispatch`, `schedule` and `release` always run everything, so a skipped leg on a pull request is caught on `main` at the latest.

The scope comes from `.github/scripts/changed-scope.sh`. It reads the files the pull request changes (the first-parent diff of the merge commit that `pull_request` checks out, so the checkout needs `fetch-depth: 2`) and names the `free/<plugin>` directories involved. It fails open: any doubt (not a pull request, no diff, 2500+ files, an odd directory name, a file directly under `free/`, a change to the script itself) means "run everything". `.github/scripts/changed-scope_test.sh` holds its table tests; the `Scope` job in `validate.yml` runs them.

`.github/scripts/go-closure-changed.sh` answers a narrower question for the `free/ci` workflows: did the pull request change anything the tested Go packages are built from? It uses `go list -deps`, so a new import widens the scope by itself.

Every plugin module is self-contained (its `replace` targets live inside its own directory), so one plugin's change cannot break another plugin's build.

## What runs when

| Workflow / leg | Runs on a pull request when | Everything else |
|---|---|---|
| `Go Build & Test` / `Build & test all Go plugins` (**required**) | every pull request; builds, vets and tests only the touched plugins (all of them when this workflow changes) | push, dispatch: all plugins |
| `Coverage Gate` | `free/**` changed; measures the touched plugins; the webhooks SSRF floor runs when `free/webhooks` changed | push to `main`: all plugins |
| `Validate Plugins` / `Scope` | every pull request (about 10 seconds) | |
| `Validate Plugins` / schema validation, registry validator | `free/*/plugin.json`, `free/*/README.md`, `registry.json`, `releases.json` or `shared/validate-registry.sh` changed | push, dispatch |
| `Validate Plugins` / `Plugin Conformance` | `free/`, `scripts/`, `tests/`, `shared/`, `images.d/`, `registry.json` or `releases.json` changed | push, dispatch |
| `Validate Plugins` / `Plugin build gate` | the touched plugins build in one job; the four shards over all plugins run when `scripts/conformance/`, `scripts/deterministic-tar.sh`, `scripts/cli-tools.version` or `images.d/` changed, or more than 12 plugins changed | push, dispatch: four shards |
| `Registry Consistency Check` | a plugin manifest, top-level config or `go.mod`, `registry.json`, `releases.json`, `catalog.json`, `counts.json`, `shared/**` or the projection scripts changed | push, dispatch |
| `License Gate` | a `go.mod`, `go.sum`, `vendor/` or `third_party/` file changed | push, release, dispatch |
| `Tarball Checksum Gate` | `free/*/plugin.json`, `registry.json`, the exceptions file or the tar/checksum scripts changed | dispatch |
| `ci-detect-langs` (Linux and macOS) | the pull request changed something `internal/detect`, `internal/gates` or `internal/plan` of `free/ci` are built from, or `free/ci/cmd`, `free/ci/testdata`, `go.mod`, `go.sum`, `vendor/` | dispatch: both |
| `ci-isolation-linux` | the same test for the isolation, hygiene and local-provider packages | dispatch |
| `SIEGE` syntax check | only its own workflow file (the scripts it lints live outside this repository) | push, dispatch |
| `nself-ci` (advisory) | any change except markdown, wiki, issue templates and `LICENSE` | push, dispatch |
| `Gitleaks`, `Hygiene`, `Generated File Gate`, `Build Plugins` | unchanged here | |

Already path-scoped before this change: `ci-toolchain-image`, `Images Probe`, `Plugin Counts`, `Infra Validate`, `k8s-kind`, `Forgejo Smoke`, `Traefik Parity`, `Registry Validation`.

Every workflow with a pull request trigger cancels a superseded run (`concurrency` with `cancel-in-progress`), except `dependabot-auto-merge` and `generated-file-gate`; `validate.yml` and `ci-toolchain-image.yml` keep a push run alive on purpose.

## Rules for editing a workflow

- The required check `Build & test all Go plugins` must report on every pull request. Scope it with steps, never with a workflow-level `paths:` filter.
- A job that depends on `scope` and is skipped needs no runner and counts as passed.
- Use `scope` outputs through `env:`, never inside a `run:` script body (a directory name is attacker-chosen on a fork).
