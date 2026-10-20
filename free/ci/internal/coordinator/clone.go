package coordinator

// Purpose: the input rules every trigger passes before it is stored or cloned (P7-NODE-10, security
// review): a full hex revision, an owner/repo project, a plain pipeline name and a clone URL that cannot
// reach anything but the project's own host.
// Inputs:  the project, revision, pipeline and clone URL of a Delivery.
// Outputs: ValidProject, ValidRevision, ValidPipeline and SafeCloneURL (the one URL a checkout may use).
// Constraints: git is called without a `--` separator, so nothing that starts with "-" may reach it; only
//          https with no user info is accepted; github and manual triggers may clone only
//          https://github.com/<project>.git, other sources (an Ingress or a Reconciler the operator
//          registered) may name their own https host.

import (
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

var (
	projectRe     = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	projectPathRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)+$`)
	revisionRe    = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	pipelineRe    = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	keyRe         = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,128}$`)
)

// ValidProject reports whether p is an owner/repo name that cannot be read as a git option.
func ValidProject(p string) bool {
	owner, repo, ok := strings.Cut(p, "/")
	return ok && projectRe.MatchString(p) && !strings.HasPrefix(owner, "-") && !strings.HasPrefix(repo, "-") &&
		owner != "." && repo != "." && !strings.Contains(p, "..")
}

// maxProjectSegments is GitLab's nesting bound for a project path (group, up to 18 subgroups, project).
const maxProjectSegments = 20

// ValidProjectFor is ValidProject for the source that named the project: github, manual and an unnamed
// source stay owner/repo, any other source (GitLab) may name a nested path of 2 to 20 segments. Every
// segment follows the same rules as ValidProject, so a nested name cannot be read as a git option either.
func ValidProjectFor(source, p string) bool {
	if source == "github" || source == "manual" || source == "" {
		return ValidProject(p)
	}
	segs := strings.Split(p, "/")
	if len(segs) < 2 || len(segs) > maxProjectSegments || !projectPathRe.MatchString(p) || strings.Contains(p, "..") {
		return false
	}
	for _, seg := range segs {
		if seg == "." || strings.HasPrefix(seg, "-") {
			return false
		}
	}
	return true
}

// ValidRevision reports whether r is a full lower-case hex commit id (40 or 64 characters).
func ValidRevision(r string) bool { return revisionRe.MatchString(r) }

// ValidPipeline reports whether n is a plain pipeline name.
func ValidPipeline(n string) bool { return pipelineRe.MatchString(n) }

// SafeCloneURL returns the URL a checkout of project may use. raw is the clone URL the source reported;
// "" means the project's GitHub URL, which only github and manual sources may use.
func SafeCloneURL(source, project, raw string) (string, error) {
	github := source == "github" || source == "manual" || source == ""
	if raw == "" {
		if !github {
			return "", newErr("", ReasonInvalidDelivery, "source %q gave no clone URL", source)
		}
		return "https://github.com/" + project + ".git", nil
	}
	u, err := url.Parse(raw)
	switch {
	case err != nil, strings.ContainsAny(raw, " \t\r\n\x00"), strings.HasPrefix(raw, "-"):
		return "", newErr("", ReasonInvalidDelivery, "clone URL is not usable")
	case u.Scheme != "https", u.User != nil, u.Host == "", u.RawQuery != "", u.Fragment != "":
		return "", newErr("", ReasonInvalidDelivery, "clone URL must be a plain https URL")
	}
	if github {
		path := strings.TrimSuffix(strings.ToLower(u.Path), ".git")
		if !strings.EqualFold(u.Host, "github.com") || path != "/"+strings.ToLower(project) {
			return "", newErr("", ReasonInvalidDelivery, "clone URL does not belong to the project")
		}
		return "https://github.com/" + project + ".git", nil // rebuilt, so no encoding trick in the reported path survives
	}
	if internalHost(u.Hostname()) {
		return "", newErr("", ReasonInvalidDelivery, "clone URL names a local or private address")
	}
	return u.String(), nil
}

// internalHost reports a host that is localhost or a loopback, private, link-local or unspecified address
// literal; a name that merely resolves to one is outside what a string check can see.
func internalHost(h string) bool {
	if strings.EqualFold(h, "localhost") || strings.HasSuffix(strings.ToLower(h), ".localhost") {
		return true
	}
	ip, err := netip.ParseAddr(strings.Trim(h, "[]"))
	return err == nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast())
}
