package provision

// Purpose: the V15 pinned GitHub Actions runner (P7-NODE-06). Today's provision asks the GitHub API for
// releases/latest and unpacks whatever it gets; V15 downloads the pinned version, checks its sha256
// against the pin before anything is extracted and refuses on a mismatch (E669).
// Inputs:  a RunnerPin built from toolchain/pins/github-actions-runner.yaml (the composition root maps
// the generated table into it).
// Outputs: the shell block that replaces the latest-release download inside the runner install script.
// Constraints: the block holds only the pin's own URL and sha256 (validated and shell-quoted) and a
// fixed message; it never reads the registration token; an unknown CPU architecture is refused (E669
// runner_arch) instead of falling back to x64.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
)

// RunnerSource is one pinned tarball.
type RunnerSource struct {
	URL    string
	SHA256 string
}

// RunnerPin is the pinned runner: a version and a source per CPU architecture (amd64, arm64).
type RunnerPin struct {
	Version string
	Sources map[string]RunnerSource
}

// Markers the remote script prints on a refusal; installRunnerInstance maps them to E669 reasons.
const (
	markChecksum = "E669: runner tarball sha256 does not match its pin; nothing was extracted"
	markArch     = "E669: no pinned runner for this CPU architecture"
)

var (
	pinURLRe = regexp.MustCompile(`^https?://[A-Za-z0-9._~:/?&=%+@-]+$`)
	pinSHARe = regexp.MustCompile(`^[0-9a-f]{64}$`)
	// archCase maps the node's `uname -m` words to the pin's architecture key.
	archCase = []struct{ pin, uname string }{{"amd64", "x86_64|amd64"}, {"arm64", "aarch64|arm64"}}
)

// Validate checks every source of the pin.
func (p RunnerPin) Validate() error {
	if len(p.Sources) == 0 || p.Version == "" {
		return bootstrap.NewError(bootstrap.ReasonUsage, "the runner pin needs a version and at least one source")
	}
	for arch, s := range p.Sources {
		if !pinURLRe.MatchString(s.URL) || !pinSHARe.MatchString(s.SHA256) {
			return bootstrap.NewError(bootstrap.ReasonUsage, "the runner pin for %s has an invalid url or sha256", arch)
		}
	}
	return nil
}

// downloadBlock is the script text that fetches, verifies and unpacks the pinned tarball in the
// current directory (the caller has already checked config.sh is absent).
func (p RunnerPin) downloadBlock() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	archs := make([]string, 0, len(p.Sources))
	for a := range p.Sources {
		archs = append(archs, a)
	}
	sort.Strings(archs)
	var b strings.Builder
	b.WriteString("  case \"$(uname -m)\" in\n")
	for _, a := range archCase {
		s, ok := p.Sources[a.pin]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "    %s) URL=%s; SHA=%s ;;\n", a.uname, shellQuote(s.URL), shellQuote(s.SHA256))
	}
	fmt.Fprintf(&b, "    *) echo %s >&2; exit 69 ;;\n  esac\n", shellQuote(markArch))
	b.WriteString("  curl -fsSL -o actions-runner.tar.gz \"$URL\"\n")
	b.WriteString("  echo \"$SHA  actions-runner.tar.gz\" | sha256sum -c - >/dev/null 2>&1 || { rm -f actions-runner.tar.gz; echo " +
		shellQuote(markChecksum) + " >&2; exit 69; }\n")
	b.WriteString("  tar xzf actions-runner.tar.gz\n")
	return b.String(), nil
}

// mapRunnerFailure turns the script's refusal markers into E669 errors; other errors pass through.
func mapRunnerFailure(out string, err error) error {
	switch {
	case err == nil:
		return nil
	case strings.Contains(out, markChecksum):
		return bootstrap.NewError(bootstrap.ReasonChecksum, "the runner tarball does not match its pin; nothing was extracted")
	case strings.Contains(out, markArch):
		return bootstrap.NewError(bootstrap.ReasonRunnerArch, "no pinned runner exists for the node's CPU architecture")
	}
	return err
}
