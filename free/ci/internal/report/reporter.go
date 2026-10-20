// Package report posts the one commit status of a run (context "nself-ci").
//
// Purpose: generalise the v1.4 status post (internal/legacy/status.go) behind a
// Reporter interface, keep a durable outbox keyed by (revision, context) with
// a monotonic seq, and say why nothing was posted when nothing can be.
// Inputs:  a Status (revision, state, description) and a Reporter.
// Outputs: a posted status, or a one-line reason.
// Constraints: the context string and states never change (CI D13); every
// process runs through the exec funnel; no secret is ever read here: gh holds
// its own credentials.
package report

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Context is the commit status context of the gate. It is frozen.
const Context = "nself-ci"

// State is a commit status state. The set is frozen.
type State string

const (
	Pending State = "pending"
	Success State = "success"
	Failure State = "failure"
	Error   State = "error"
)

// Valid reports whether s is one of the four GitHub states.
func (s State) Valid() bool { return s == Pending || s == Success || s == Failure || s == Error }

// MaxDescription is the GitHub limit for a status description.
const MaxDescription = 140

// MaxProjectSegments is the deepest project path a status can name (GitLab allows 20 segments).
const MaxProjectSegments = 20

// Status is one commit status to deliver. Owner and Repo make the payload
// self-contained, so a later invocation can drain it from anywhere. Owner is
// one name. Repo is one name on GitHub; for a host with subgroups (GitLab) it
// is the rest of the project path, "sub/project", so Owner + "/" + Repo is the
// full path.
type Status struct {
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	Revision    string `json:"revision"`
	State       State  `json:"state"`
	Description string `json:"description"`
	TargetURL   string `json:"target_url,omitempty"`
}

// Reporter delivers a status to a code host.
type Reporter interface {
	Post(ctx context.Context, s Status) error
}

var (
	ownerRepo = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
	shaRe     = regexp.MustCompile(`^[0-9a-f]{40,64}$`)
)

// Validate refuses a status that could not be sent safely.
func (s Status) Validate() error {
	switch {
	case !ownerRepo.MatchString(s.Owner) || !validRepoPath(s.Repo):
		return errors.New("report: invalid owner or repo")
	case !shaRe.MatchString(s.Revision):
		return errors.New("report: invalid revision")
	case !s.State.Valid():
		return errors.New("report: invalid state")
	}
	return nil
}

// validRepoPath reports whether r is one to MaxProjectSegments-1 valid names joined by "/".
func validRepoPath(r string) bool {
	segs := strings.Split(r, "/")
	if len(segs) >= MaxProjectSegments {
		return false
	}
	for _, seg := range segs {
		if !ownerRepo.MatchString(seg) {
			return false
		}
	}
	return true
}

var lineBreaks = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")

// Truncate cuts a description to MaxDescription runes. Every line break becomes a space.
func Truncate(d string) string {
	d = lineBreaks.Replace(d)
	if utf8.RuneCountInString(d) <= MaxDescription {
		return d
	}
	r := []rune(d)
	return string(r[:MaxDescription-1]) + "…"
}
