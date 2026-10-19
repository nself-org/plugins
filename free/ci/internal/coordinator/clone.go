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
	"net/url"
	"regexp"
	"strings"
)

var (
	projectRe  = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	revisionRe = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	pipelineRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	keyRe      = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,128}$`)
)

// ValidProject reports whether p is an owner/repo name that cannot be read as a git option.
func ValidProject(p string) bool {
	return projectRe.MatchString(p) && !strings.HasPrefix(p, "-") && !strings.Contains(p, "/-") && !strings.Contains(p, "..")
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
	}
	return u.String(), nil
}
