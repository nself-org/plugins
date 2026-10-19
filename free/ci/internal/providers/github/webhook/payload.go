package webhook

// Purpose: the subset of GitHub webhook payloads the source reads, and the helpers that turn a value
// into a payload-sourced fact.
// Inputs:  the raw JSON body of push, pull_request, create, repository_dispatch and workflow_job.
// Outputs: ghPayload (tolerant of unknown fields) and model.Fact values with source "payload".
// Constraints: every payload-derived fact carries source payload (Epic acceptance); an absent claim is
//          a nil value, never a zero that reads as a fact. Nothing here is trusted for a decision the
//          API can make (permission, protection, fork status are refetched).

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type ghRepo struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	Private       *bool  `json:"private"`
	Visibility    string `json:"visibility"`
	DefaultBranch string `json:"default_branch"`
}

type ghUser struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

type ghBranch struct {
	Ref  string  `json:"ref"`
	SHA  string  `json:"sha"`
	Repo *ghRepo `json:"repo"` // null once the head repository is deleted
}

type ghPR struct {
	Number            int      `json:"number"`
	AuthorAssociation string   `json:"author_association"`
	Head              ghBranch `json:"head"`
	Base              ghBranch `json:"base"`
}

type ghJob struct {
	ID         int64    `json:"id"`
	RunID      int64    `json:"run_id"`
	HeadSHA    string   `json:"head_sha"`
	HeadBranch string   `json:"head_branch"`
	Labels     []string `json:"labels"`
}

type ghPayload struct {
	Action      string `json:"action"`
	Ref         string `json:"ref"`
	RefType     string `json:"ref_type"`
	Before      string `json:"before"`
	After       string `json:"after"`
	Deleted     bool   `json:"deleted"`
	Repository  ghRepo `json:"repository"`
	Sender      ghUser `json:"sender"`
	PullRequest *ghPR  `json:"pull_request"`
	WorkflowJob *ghJob `json:"workflow_job"`
	HeadCommit  struct {
		ID string `json:"id"`
	} `json:"head_commit"`
	ClientPayload struct {
		Revision string `json:"revision"`
	} `json:"client_payload"`
}

func parsePayload(body []byte) (ghPayload, error) {
	var p ghPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return ghPayload{}, unsupported("payload is not a supported GitHub event body")
	}
	return p, nil
}

// fact wraps v as a fact from src observed at now.
func fact[T any](v T, src string, now time.Time) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: src, ObservedAt: now, Confidence: "known"}
}

// payloadFact is a fact the delivery itself claims.
func payloadFact[T any](v T, now time.Time) model.Fact[T] { return fact(v, "payload", now) }

// visibilityOf returns the repository's visibility, and false when the object does not say (an absent
// claim is unknown, never "public").
func visibilityOf(r ghRepo) (string, bool) {
	switch r.Visibility {
	case "public", "private", "internal":
		return r.Visibility, true
	}
	if r.Private != nil {
		if *r.Private {
			return "private", true
		}
		return "public", true
	}
	return "", false
}

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
