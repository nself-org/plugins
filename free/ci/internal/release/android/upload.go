package android

// Purpose: the explicit publish step of `build --artifact android --upload`
// (Epic CACHE D18): attach the built APKs to a GitHub release with
// `gh release upload`.
// Inputs:  a Capture (the exec funnel, report.Capture in production), the
// owner, repo and tag, and the APK paths.
// Outputs: nil, or an error that names the asset and the reason.
// Constraints: never `--clobber`: an asset of the same name on the release is
// an error naming it, found before the upload and again from gh's own answer;
// the owner, repo, tag and file names are checked so none can be read as a gh
// flag; one gh process for the check and one for the upload, both through the
// funnel; the output of a failed gh call is cut to its first line.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Capture runs argv through the exec funnel and returns the merged output and
// the exit code; err is set only when the process could not be run.
type Capture func(ctx context.Context, argv ...string) (out string, exit int, err error)

// ErrAssetExists is returned when the release already holds an asset of that name.
var ErrAssetExists = errors.New("release asset already exists")

var (
	ownerRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)
	repoRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	tagRe   = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._/+-]{0,254}$`)
)

// Upload attaches apks to the release tag of owner/repo.
func Upload(ctx context.Context, run Capture, owner, repo, tag string, apks []string) error {
	switch {
	case !ownerRe.MatchString(owner):
		return fmt.Errorf("upload: owner %q is not a GitHub owner name", owner)
	case !repoRe.MatchString(repo) || repo == "." || repo == "..":
		return fmt.Errorf("upload: repo %q is not a GitHub repository name", repo)
	case !tagRe.MatchString(tag):
		return fmt.Errorf("upload: tag %q is not a release tag", tag)
	case len(apks) == 0:
		return errors.New("upload: no APK to upload")
	}
	target := owner + "/" + repo
	for _, a := range apks {
		if !filepath.IsAbs(a) || strings.HasPrefix(filepath.Base(a), "-") {
			return fmt.Errorf("upload: %q is not an absolute APK path", a)
		}
	}
	out, exit, err := run(ctx, "gh", "release", "view", tag, "--repo", target, "--json", "assets", "--jq", ".assets[].name")
	if err != nil || exit != 0 {
		return fmt.Errorf("upload: release %s of %s cannot be read: %s", tag, target, reason(out, err))
	}
	have := map[string]bool{}
	for _, name := range strings.Fields(out) {
		have[name] = true
	}
	for _, a := range apks {
		if name := filepath.Base(a); have[name] {
			return fmt.Errorf("%w: %s is already on release %s of %s; nothing was uploaded (this command never overwrites)", ErrAssetExists, name, tag, target)
		}
	}
	argv := append([]string{"gh", "release", "upload", tag}, apks...)
	argv = append(argv, "--repo", target)
	out, exit, err = run(ctx, argv...)
	if err != nil || exit != 0 {
		for _, a := range apks {
			if strings.Contains(out, filepath.Base(a)) && strings.Contains(strings.ToLower(out), "already") {
				return fmt.Errorf("%w: %s on release %s of %s: %s", ErrAssetExists, filepath.Base(a), tag, target, reason(out, err))
			}
		}
		return fmt.Errorf("upload: gh release upload failed: %s", reason(out, err))
	}
	return nil
}

// reason is the first non-empty line of gh's output, or the run error.
func reason(out string, err error) string {
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	if err != nil {
		return err.Error()
	}
	return "no output"
}

// ParseRemote extracts owner and repo from a GitHub remote URL
// (https://github.com/o/r[.git], ssh://git@github.com/o/r, git@github.com:o/r).
func ParseRemote(url string) (owner, repo string, ok bool) {
	url = strings.TrimSuffix(strings.TrimSpace(url), ".git")
	if i := strings.Index(url, "://"); i >= 0 {
		url = url[i+3:]
	}
	if i := strings.IndexAny(url, ":/"); i >= 0 {
		url = url[i+1:]
	}
	parts := strings.Split(strings.Trim(url, "/"), "/")
	if len(parts) < 2 {
		return "", "", false
	}
	owner, repo = parts[len(parts)-2], parts[len(parts)-1]
	return owner, repo, ownerRe.MatchString(owner) && repoRe.MatchString(repo)
}
