package webhook

// Purpose: the subset of GitLab webhook payloads the source reads (Push Hook, Tag Push Hook, Merge
// Request Hook, Job Hook), the project identity probe, and the helpers that turn a value into a
// payload-sourced fact.
// Inputs:  the raw JSON body of a delivery.
// Outputs: glPayload (tolerant of unknown fields) and model.Fact values with source "payload".
// Constraints: every payload-derived fact carries source payload; an absent claim is a nil value,
//          never a zero that reads as a fact. Nothing here is trusted for a decision the API can make
//          (permission, protection, fork status are refetched).

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type glProject struct {
	ID         int64  `json:"id"`
	Path       string `json:"path_with_namespace"`
	Visibility *int   `json:"visibility_level"`
}

type glUser struct {
	Username string `json:"username"`
}

type glMR struct {
	IID             int       `json:"iid"`
	Action          string    `json:"action"`
	State           string    `json:"state"`
	OldRev          string    `json:"oldrev"`
	SourceProjectID int64     `json:"source_project_id"`
	TargetProjectID int64     `json:"target_project_id"`
	SourceBranch    string    `json:"source_branch"`
	TargetBranch    string    `json:"target_branch"`
	LastCommit      glCommit  `json:"last_commit"`
	Source          glProject `json:"source"`
}

type glCommit struct {
	ID string `json:"id"`
}

type glPayload struct {
	ObjectKind   string     `json:"object_kind"`
	Ref          string     `json:"ref"`
	Before       string     `json:"before"`
	After        string     `json:"after"`
	CheckoutSHA  string     `json:"checkout_sha"`
	UserUsername string     `json:"user_username"`
	Project      *glProject `json:"project"`
	User         *glUser    `json:"user"`
	MR           *glMR      `json:"object_attributes"`
	// Job Hook
	BuildStatus string   `json:"build_status"`
	BuildID     int64    `json:"build_id"`
	PipelineID  int64    `json:"pipeline_id"`
	ProjectID   int64    `json:"project_id"`
	SHA         string   `json:"sha"`
	TagList     []string `json:"tag_list"`
	Repository  struct {
		Homepage string `json:"homepage"`
	} `json:"repository"`
}

func parsePayload(body []byte) (glPayload, error) {
	var p glPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return glPayload{}, unsupported("payload is not a supported GitLab event body")
	}
	return p, nil
}

// glProbe is all that is read from a body before it is authenticated: scalars only, so a hostile body
// cannot make the unauthenticated decode allocate more than a copy of one string.
type glProbe struct {
	Project *struct {
		ID   int64  `json:"id"`
		Path string `json:"path_with_namespace"`
	} `json:"project"`
	ProjectID  int64 `json:"project_id"`
	Repository struct {
		Homepage string `json:"homepage"`
	} `json:"repository"`
}

// identity is the project a body claims to be from: project.path_with_namespace and project.id, or for
// an older Job Hook (which has no project object) project_id and the path of repository.homepage. Nothing
// it returns is trusted until the delivery authenticates under that project's own secret.
func (p glProbe) identity() (name string, id int64) {
	if p.Project != nil {
		return p.Project.Path, p.Project.ID
	}
	if u, err := url.Parse(p.Repository.Homepage); err == nil && u.Path != "" {
		name = strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	}
	return name, p.ProjectID
}

// probeBody decodes the identity probe; a body that does not parse names no project.
func probeBody(body []byte) glProbe {
	var p glProbe
	_ = json.Unmarshal(body, &p)
	return p
}

// tags are the runner tags a job asks for, when the payload says (the Jobs API is the other source). The
// payload's runner object lists the tags of the runner that took the job, which say nothing about what the
// job asked for, so it is not read.
func (p glPayload) tags() []string { return p.TagList }

// visibilityOf maps GitLab's numeric visibility_level; an absent or unknown value is unknown.
func visibilityOf(v *int) (string, bool) {
	if v == nil {
		return "", false
	}
	switch *v {
	case 0:
		return "private", true
	case 10:
		return "internal", true
	case 20:
		return "public", true
	}
	return "", false
}

// fact wraps v as a fact from src observed at now.
func fact[T any](v T, src string, now time.Time) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: src, ObservedAt: now, Confidence: "known"}
}

// payloadFact is a fact the delivery itself claims.
func payloadFact[T any](v T, now time.Time) model.Fact[T] { return fact(v, "payload", now) }

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// isHex40 reports a full lower-case 40 character commit id.
func isHex40(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func allZero(s string) bool { return s != "" && strings.Trim(s, "0") == "" }

// clip bounds attacker-chosen text that is echoed back in a decline reason.
func clip(s string) string {
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '?'
		}
		return r
	}, s)
}

// probe is the identity part of a decoded payload (the same scalars probeBody reads).
func (p glPayload) probe() glProbe {
	pr := glProbe{ProjectID: p.ProjectID}
	pr.Repository.Homepage = p.Repository.Homepage
	if p.Project != nil {
		pr.Project = &struct {
			ID   int64  `json:"id"`
			Path string `json:"path_with_namespace"`
		}{ID: p.Project.ID, Path: p.Project.Path}
	}
	return pr
}
