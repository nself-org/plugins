# Migrating to the new CI engine

<!-- docmap: cap:docs.ci-core -->

This page is for you if you already use the earlier `nself ci` gate, the `.ci.yaml` manifests that `nself ci run` discovers, or the wrapper-script workflow that older versions of this wiki described. It lists what changes, how to move your configuration, and how to adopt the gate in another repository. It replaces the old adoption guide.

The new engine is on when `NSELF_V15=1` is set. The plugin binary does not follow the CLI's own switch yet, so the workflow template sets the variable itself. Until the older path is removed, both run side by side, and you can try the new one in a shell without touching a team workflow.

## What changes

| Before | Now |
|---|---|
| Configuration lived in `.ci.yaml` files per plugin, discovered by `nself ci run` | One `.nself/ci.yaml` at the repository root, validated against a schema. `nself ci run` becomes `nself ci --trigger manual`. |
| A fixed gate per stack, read from the repository root | Checks per detected stack and directory, planned as a job graph with a reason for every job. See [[CI configuration and policy|guides/ci/Configuration-and-Policy]]. |
| A pass or a fail, printed | A pass, fail, error or cancelled result with evidence, history, logs and a stored record. See [[Evidence and machine output|guides/ci/Evidence-and-Machine-Output]]. |
| The commit status was posted by every run | A status is posted only for a full, bound, default-pipeline run from an owner or internal source that was not cancelled. `--job`, `--pipeline`, a dirty tree, a tree that changed during the run and a `--config` outside the repository never post. |
| A missing `gitleaks` was treated as a pass | A missing required tool is `E614`, so the secret scan can no longer pass by being absent. Install `gitleaks` or the run stops. |
| `--sha` chose the commit | Not supported. Use `nself ci evidence verify <revision>`. |
| `nself ci eval` was part of the gate | `eval` is a separate command of the `nself-eval-gate` plugin, `nself eval`. A project that wants it in CI runs it as a job. |
| `nself ci build --artifact android` built in place with Gradle | The arguments are unchanged. Under the new engine it runs a release pipeline and publishes a release directory you can verify. The earlier path is removed when the earlier engine is. |
| Output was text | `--json` gives a stable envelope for every command, and `--stream` gives events. |

Two rules for upgrades and downgrades: the evidence documents gained fields, and an older build rejects a newer document, so downgrading means regenerating evidence. The local database refuses to open under a binary older than it needs, with a clear error.

## Move your configuration

`nself ci config migrate` reads the legacy `.ci.yaml` files and prints the `.nself/ci.yaml` they would become. Nothing is written until you ask:

```bash
mkdir legacy && cd legacy && git init -q
git config user.email docs@example.com && git config user.name docs
cat > .ci.yaml <<'EOF'
plugin: example
stages:
  - name: unit
    cmd: ["go", "test", "./..."]
    timeout: 120
  - name: integration
    cmd: ["go", "test", "-tags", "integration", "./..."]
    env_required:
      - DATABASE_URL
    skip_if_env_missing: true
EOF
nself ci config migrate
```

Run it, read the diff and the notes it prints, then write the file with `nself ci config migrate --write`. It never writes through a symlink and never overwrites a different file. When you are satisfied, delete the legacy files; a leftover `.ci.yaml` is reported as `E621`.

**Migration follows what your file declares.** The earlier runner did not parse YAML. It read `timeout` and `skip_if_env_missing` only at one indentation, read list items only at another, filed all-caps list items under `env_required` and skipped a manifest with no top-level `plugin:` key. A manifest written as valid YAML, with `cmd: [..]` on one line, could therefore declare commands that never ran. `migrate` prints a note for each stage and field where the old reading and the declared meaning differ, so the first run under the new engine may run commands the old one silently skipped. That is the intended change.

What carries over:

| Legacy | Becomes |
|---|---|
| `cmd` | `run` |
| `timeout` | `timeout`, in seconds converted to a duration; a stage without one gets the old default of 300 seconds, written explicitly |
| `env_required` with `skip_if_env_missing` | `required: false` and `when.env_present` |
| stage order | nothing: jobs are independent |

The last row matters. The old runner ran stages in order in one directory and gave no information about which stage consumed another's output, so `migrate` invents no dependencies and prints a note. Add `deps:` wherever one stage reads files another writes, or merge the commands into one job. Null or nested list items, anchors, aliases, merge keys, a second YAML document and unknown fields are refused with `E622`.

## Adopt the gate in another repository

This is the pattern first proven on this repository, rewritten for the current engine. It replaces the earlier wrapper-script recipe, whose script no longer exists.

1. **Describe the gate.** If your repository is a single stack with standard scripts, you need no file. Otherwise write `.nself/ci.yaml` with the jobs you want. Declaring jobs turns detection off for the repository, so a gate is exactly the list you declare. A repository with many modules should say so: detection would otherwise plan a job per module.
2. **Run it locally first.** `nself ci plan` shows which checks will run, and `NSELF_V15=1 nself ci` runs them. Read the plan. Do not assume.
3. **Add the workflow.** Copy `free/ci/templates/nself-ci.yml` from the plugins repository to `.github/workflows/nself-ci.yml`, fill in the pinned release, and add your toolchain setup. The template is described under [Configuration and policy](./Configuration-and-Policy.md#the-workflow-template).
4. **Require it, as an addition.** Run `nself ci policy protect --repo <owner>/<name> --dry-run`, review the body, then apply it. It saves the previous protection first and can restore it. Add `nself-ci` next to your existing required checks, not instead of them, until you have seen it catch what they catch.
5. **Guard the main branch.** `nself ci policy protect --hook` installs a pre-push hook for the default branch, and `--audit <days>` lists commits that never had a green `nself-ci`.

### Read this before making it your only gate

A gate can pass while checking nothing. That is the failure to look for, and it is why step 2 matters.

- **A stack the detector does not see.** The earlier gate decided whether a Node repository had work by looking for scripts in the root `package.json`. A repository whose real scripts sat under another name, or in workspace members without a workspace file, ran only the secret scan and printed a pass. The new planner walks directories and workspaces and gives every skipped job a reason, but a custom script name still needs a job of your own. `nself ci plan` is the place to see it.
- **Per-package checks.** Repository-wide checks do not replace checks that belong to a package or a plugin, such as schema validation, conformance or registry consistency. Keep those workflows.
- **Work the engine does not do.** `nself ci` is not an emulator for hosted workflow files. End-to-end tests, accessibility audits, code scanning, licence audits, coverage ratchets, bundle size checks and deploys need their own jobs. Public repositories can keep them on free hosted runners.
- **Forks.** A token on a fork pull request is read-only, so a commit status cannot be posted from it. The workflow template produces a check run instead, which fork and bot pull requests still get.

### Rollout checklist

- [ ] `.nself/ci.yaml` written, or the detected plan reviewed and accepted.
- [ ] `NSELF_V15=1 nself ci plan` read, and each stack you care about appears.
- [ ] `nself ci` passes locally with `gitleaks` installed.
- [ ] The workflow runs once on a pull request and the `nself-ci` check appears.
- [ ] `policy protect --dry-run` reviewed by the person who owns branch protection, then applied as an additional required check.
- [ ] The earlier workflow or wrapper script removed, and no page of your documentation links to it.
