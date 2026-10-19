# Drills

A drill is a script that proves something about the repository or its release
by doing it for real: install from a clean directory, upgrade across a
version, run an importer against a fixture. Drills are not a separate harness.
They are jobs of the `drills` pipeline in `.nself/ci.yaml`, run by `nself ci`
like any other job, on a schedule or on demand.

## Add a drill

1. Create a directory here: `scripts/drill/<name>/`. The name starts with a
   letter or digit and uses only letters, digits, `.`, `_` and `-`.
2. Put an executable file called `run` in it (`chmod +x`, committed with the
   execute bit). Any language works; a shebang line picks it.
3. Commit. The next run has a job called `drill-<name>`. You do not edit
   `.nself/ci.yaml`.

A directory without a `run` file, a `run` that is not executable, a symlink and
loose files are ignored. A directory whose name breaks the rule above fails the
pipeline with E640 rather than being skipped without a word.

## What a drill reports

- **The exit code is the result.** 0 passes, anything else fails.
- **Optional nested evidence.** A drill that has several steps can print one
  `ci.evidence/v1` document on stdout, on its own line, as compact JSON. It is
  validated against the schema. Its failing checks are listed in the job's
  check (`findings` and `excerpt`).
  - A drill that exits 0 but prints a document whose result is `fail` ends
    **fail**. A result of `error` ends **error**.
  - A document that is present but invalid ends **error** (E640), whatever the
    exit code.
  - A document spread over several lines is refused the same way (E640): print compact JSON on one line.
  - Print the document last; the last matching line wins, and log noise on
    stderr or earlier lines is fine.
- The working directory is the repository root. Do not print secrets: output
  passes the redactor, but a drill is a poor place to rely on that.

## Run and inspect

```
nself ci --pipeline drills          # all drills, in name order
nself ci --pipeline drills --json   # one check per drill in the evidence
nself ci evidence --pipeline drills # overdue report (exit 10 when overdue)
```

A drill job is overdue when it has no passing, commit-bound run on this
machine newer than the pipeline's `max_age` (36 hours here). The report reads
the 1000 most recent runs of the machine's shared store, so on a very busy
machine an old pass can fall out of view and read as overdue (it fails closed).
The report needs a machine that keeps its run store; a throwaway CI runner has
none, so for the GitHub schedule a failed or missing workflow run is the alert. The scheduled
GitHub workflow is the template at `free/ci/templates/scheduled-pipeline.yml`.

## Change a drill's limits

A nested `error` result counts as an infrastructure failure, so the job's default
infra retry (one) may run the drill a second time. A discovered job has the defaults of any job (10 minutes). To give one drill a
longer timeout or an environment variable, define a job named `drill-<name>`
with `kind: drill` in `.nself/ci.yaml`. A job defined there replaces the
discovered one; the directory still decides that the job exists.
