// Package api holds the GitLab REST v4 helpers the GitLab providers share (P7-HOST-06, Epic D11): the
// paths and wire types of the routes they call, the project path encoding, error constructors for the
// E70x codes, a paginated listing and the client builder over the provider REST funnel.
package api

// Purpose: name every GitLab route the dispatch and status providers use, with the fields they read.
// Inputs:  none (declarations only).
// Outputs: path templates for rest.Client (parameters in {braces}), wire types, error helpers.
// Constraints: a project is addressed by its path ("group/sub/name"), which the funnel escapes into one
//          path segment; the pipeline trigger endpoint is deliberately absent (Epic D11: no trigger
//          tokens); no type here carries a credential; errors carry reason keys, never a response body.

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// The routes. {id} is the URL-encoded project path, {pipeline} and {job} are numeric ids.
const (
	VersionPath      = "/version"
	UserPath         = "/user"
	ProjectPath      = "/projects/{id}"
	PipelinesPath    = "/projects/{id}/pipelines"
	PipelinePath     = "/projects/{id}/pipelines/{pipeline}"
	PipelineVars     = "/projects/{id}/pipelines/{pipeline}/variables"
	PipelineJobs     = "/projects/{id}/pipelines/{pipeline}/jobs"
	PipelineCancel   = "/projects/{id}/pipelines/{pipeline}/cancel"
	CreatePipeline   = "/projects/{id}/pipeline"
	StatusPath       = "/projects/{id}/statuses/{sha}"
	artifactsZipPath = "/jobs/{job}/artifacts"
	rawFilePath      = "/repository/files/{file}/raw"
)

// MaxPages bounds every listing; PageSize is the per_page GitLab accepts at most.
const (
	MaxPages = 3
	PageSize = 100
)

// Project is a "group/sub/name" project path.
type Project struct{ Path string }

func okSegment(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > 255 || strings.HasPrefix(s, ".") && len(s) == 1 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

// ParseProject accepts two to twenty path segments of letters, digits, '-', '_' and '.'.
func ParseProject(s string) (Project, bool) {
	segs := strings.Split(s, "/")
	if len(segs) < 2 || len(segs) > 20 {
		return Project{}, false
	}
	for _, seg := range segs {
		if !okSegment(seg) {
			return Project{}, false
		}
	}
	return Project{Path: s}, true
}

func (p Project) String() string { return p.Path }

// Params is the {id} parameter of a project route; the funnel escapes the slashes.
func (p Project) Params() map[string]string { return map[string]string{"id": p.Path} }

// With adds more parameters to a copy of Params.
func (p Project) With(kv ...string) map[string]string {
	m := p.Params()
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}

// User is the user a token or a pipeline belongs to.
type User struct {
	Username string `json:"username"`
}

// ProjectInfo is the part of a project the providers read.
type ProjectInfo struct {
	ID            int64  `json:"id"`
	DefaultBranch string `json:"default_branch"`
	Visibility    string `json:"visibility"`
}

// Pipeline is a pipeline as GitLab reports it.
type Pipeline struct {
	ID        int64     `json:"id"`
	Status    string    `json:"status"`
	Source    string    `json:"source"`
	Ref       string    `json:"ref"`
	SHA       string    `json:"sha"`
	Name      string    `json:"name"`
	User      User      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

// Variable is one pipeline variable as the variables route echoes it.
type Variable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ArtifactFile is one entry of a job's artifacts list.
type ArtifactFile struct {
	FileType string `json:"file_type"`
	Size     int64  `json:"size"`
}

// Job is a pipeline job.
type Job struct {
	ID                int64          `json:"id"`
	Name              string         `json:"name"`
	Status            string         `json:"status"`
	ArtifactsExpireAt *time.Time     `json:"artifacts_expire_at"`
	Artifacts         []ArtifactFile `json:"artifacts"`
}

// ArchiveSize returns the size of the job's artifact archive (0 and false when there is none).
func (j Job) ArchiveSize() (int64, bool) {
	for _, a := range j.Artifacts {
		if a.FileType == "archive" {
			return a.Size, true
		}
	}
	return 0, false
}

// ArtifactsURL is the download link of a job's artifact archive under baseURL.
func ArtifactsURL(baseURL string, p Project, job int64) string {
	return strings.TrimRight(baseURL, "/") + "/projects/" + escape(p.Path) + strings.Replace(artifactsZipPath, "{job}", itoa(job), 1)
}

// RawFileURL is the download link of a file's raw bytes at ref under baseURL.
func RawFileURL(baseURL string, p Project, file, ref string) string {
	return strings.TrimRight(baseURL, "/") + "/projects/" + escape(p.Path) + strings.Replace(rawFilePath, "{file}", escape(file), 1) + "?ref=" + escape(ref)
}

// Failure builds a providers.Error with a reason key.
func Failure(class providers.ErrorClass, code, reason string) error {
	return providers.NewError(class, code, reason, 0, time.Time{})
}

// Refuse is E708: the request is refused before any call is made.
func Refuse(reason string) error { return Failure(providers.Config, "E708", reason) }

// Misconfig is E702: the setup cannot work as it is.
func Misconfig(reason string) error { return Failure(providers.Config, "E702", reason) }

// NotEvidence is E705: what came back is not evidence of this attempt.
func NotEvidence(reason string) error { return Failure(providers.Config, "E705", reason) }

// Unavailable is E704 transient: try again.
func Unavailable(reason string) error { return Failure(providers.Transient, "E704", reason) }

// IsStatus reports whether err is a funnel error for an HTTP status.
func IsStatus(err error, status int) bool {
	var pe *providers.Error
	return errors.As(err, &pe) && pe.Status == status
}

// IsTransient reports a retryable failure (transient or outage).
func IsTransient(err error) bool {
	switch providers.ClassOf(err) {
	case providers.Transient, providers.Outage:
		return true
	}
	return false
}

// IsCredentialMissing reports E700, which makes a provider contribute nothing.
func IsCredentialMissing(err error) bool {
	var pe *providers.Error
	return errors.As(err, &pe) && pe.Code == "E700"
}

func escape(s string) string { return url.PathEscape(s) }
