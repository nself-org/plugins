package protect

// Purpose: the pre-push hook of `policy protect --hook` (P7-CI-07, X2): a push
// to the default branch is refused unless `nself ci evidence verify <sha>`
// passes for the pushed sha, that is the evidence is current, bound to that
// commit and passing. Missing, stale, unbound or failing evidence all refuse.
// Inputs:  the path of the hook file and the default branch name.
// Outputs: the hook file (mode 0755), or E642 with the chaining line when a
// hook already exists.
// Constraints: the check runs with NSELF_V15=1 (a pre-v1.6 cli passes a user-set value
// through; without it a v1.4 plugin has no `evidence verify` and every push would be
// refused); nothing is ever overwritten; an identical managed hook is a
// no-op (re-running the installer is safe); the hook fails closed (no
// binary on PATH refuses the push; `git push --no-verify` is the owner's
// override). Git hooks are shell, so the hook is sh, not Go.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// HookMarker is on the second line of every hook this package writes.
const HookMarker = "# nself-ci-managed-hook v1"

var branchName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,100}$`)

// HookScript returns the hook body for a default branch.
func HookScript(branch string) (string, error) {
	if !branchName.MatchString(branch) {
		return "", fmt.Errorf("protect: %q is not a usable branch name for the hook", branch)
	}
	return strings.ReplaceAll(hookTemplate, "@BRANCH@", branch), nil
}

// ChainLine is what an existing hook adds to call the check itself.
func ChainLine(branch string) string {
	return fmt.Sprintf(`# in your pre-push hook, for each "<local ref> <local sha> <remote ref> <remote sha>" line read from stdin where <remote ref> is refs/heads/%s:
NSELF_V15=1 nself ci evidence verify "<local sha>" --json >/dev/null || exit 1`, branch)
}

// InstallHook writes the hook at path unless one is there. changed is false
// when the identical managed hook was already installed.
func InstallHook(path, branch string) (changed bool, err error) {
	body, err := HookScript(branch)
	if err != nil {
		return false, err
	}
	existing, rerr := os.ReadFile(path)
	switch {
	case rerr == nil:
		if string(existing) == body {
			return false, nil
		}
		return false, fail(CodeHookExists, "%s exists and was not written by this command; nothing was changed.\n%s", path, ChainLine(branch))
	case !errors.Is(rerr, os.ErrNotExist):
		if _, lerr := os.Lstat(path); lerr == nil {
			return false, fail(CodeHookExists, "%s exists and cannot be read (%v); nothing was changed.\n%s", path, rerr, ChainLine(branch))
		}
		return false, fmt.Errorf("protect: cannot inspect %s: %w", path, rerr)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("protect: cannot create the hooks directory: %w", err)
	}
	// O_EXCL: a hook that appeared since the read above is never overwritten.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, fail(CodeHookExists, "%s appeared while installing; nothing was changed.\n%s", path, ChainLine(branch))
		}
		return false, fmt.Errorf("protect: cannot write the hook: %w", err)
	}
	if _, err := f.WriteString(body); err != nil {
		_ = f.Close()
		return false, err
	}
	return true, f.Close()
}

const hookTemplate = `#!/bin/sh
` + HookMarker + `
# Refuses a push to the default branch unless the evidence for the pushed
# commit is current, bound to that commit and passing. Installed by
# ` + "`nself ci policy protect --hook`" + `; bypass once with ` + "`git push --no-verify`" + `.
branch="@BRANCH@"
while read -r local_ref local_sha remote_ref remote_sha; do
  [ "$remote_ref" = "refs/heads/$branch" ] || continue
  case "$local_sha" in *[!0]*) ;; *) continue ;; esac # deleting the branch: nothing to verify
  if command -v nself >/dev/null 2>&1; then
    out=$(NSELF_V15=1 nself ci evidence verify "$local_sha" --json 2>&1 </dev/null)
  elif command -v nself-ci >/dev/null 2>&1; then
    out=$(NSELF_V15=1 nself-ci evidence verify "$local_sha" --json 2>&1 </dev/null)
  else
    echo "nself-ci: refusing the push: neither nself nor nself-ci is on PATH, so the evidence cannot be checked" >&2
    exit 1
  fi
  rc=$?
  if [ "$rc" -ne 0 ]; then
    echo "nself-ci: refusing the push to $branch: no current, bound, passing evidence for $local_sha (exit $rc)" >&2
    echo "  run: nself ci   (on a clean tree at that commit), then push again" >&2
    printf '%s\n' "$out" | tail -n 5 >&2
    exit 1
  fi
done
exit 0
`
