package clientapi

// Purpose: the run routes of the user API: submit, list, show and cancel (Epic D14).
// Inputs:  JSON bodies (strict, 64 KiB), the run reference in the path.
// Outputs: JSON; 202 for a stored trigger, 200 otherwise.
// Constraints: the project of a submit is checked against the caller's role scope and the coordinator
//          allowlist before anything is stored; a run the caller may not see is answered as not found;
//          a run no intake row owns is visible to an all-project admin only; submit and cancel write an
//          audit row first and fail closed when it cannot be written.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

const maxBody = 64 << 10

func (s *Server) ping(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "api": "v1", "principal": a.ID()})
}

// readJSON decodes a strict JSON body.
func readJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return Failf(ReasonBadRequest, "the body is unreadable or larger than %d bytes", maxBody)
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if dec.Decode(v) != nil || dec.More() {
		return Failf(ReasonBadRequest, "the body is not the expected JSON object")
	}
	return nil
}

// canSee reports whether the actor may read the run (project scope, or all-project admin for an unowned run).
func canSee(a roles.Actor, it RunItem) bool {
	if it.Project != "" {
		return a.Can(roles.Member, it.Project) == nil
	}
	return a.Can(roles.Admin, "") == nil
}

// authorizeRun is the live (store re-read, audited when above member) check for one run.
func authorizeRun(ctx context.Context, a roles.Actor, it RunItem) error {
	if it.Project != "" {
		return roles.Authorize(ctx, a, roles.Member, it.Project)
	}
	return roles.Authorize(ctx, a, roles.Admin, "")
}

// auditFirst writes the audit row before the action; no row, no action.
func (s *Server) auditFirst(ctx context.Context, a roles.Actor, action, target, detail string) error {
	if s.cfg.Roles == nil || s.cfg.Roles.Log == nil {
		return Failf(ReasonInternal, "no audit log is configured")
	}
	if err := s.cfg.Roles.Log.Append(ctx, audit.Event{Actor: a.Label(), Action: action, Target: target, Detail: detail}); err != nil {
		return Failf(ReasonInternal, "the audit row could not be written")
	}
	return nil
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	var req SubmitRequest
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	req.Revision = strings.ToLower(req.Revision)
	if !coordinator.ValidProject(req.Project) || !coordinator.ValidRevision(req.Revision) || (req.Pipeline != "" && !coordinator.ValidPipeline(req.Pipeline)) {
		s.fail(w, Failf(ReasonBadRequest, "need project owner/repo, a full hex revision and a plain pipeline name"))
		return
	}
	if err := roles.Authorize(r.Context(), a, roles.Member, req.Project); err != nil {
		s.fail(w, err)
		return
	}
	if !s.cfg.Local && (s.cfg.Allow == nil || !s.cfg.Allow(req.Project)) { // the local operator may submit any project
		s.fail(w, Failf(ReasonProjectBlocked, "project %s is not on the coordinator allowlist", req.Project))
		return
	}
	if err := s.auditFirst(r.Context(), a, "run.submit", req.Project, "revision="+req.Revision+" pipeline="+req.Pipeline); err != nil {
		s.fail(w, err)
		return
	}
	res, err := s.cfg.Backend.Submit(r.Context(), req, a.Label())
	if err != nil {
		s.fail(w, err)
		return
	}
	status := http.StatusAccepted
	if res.Outcome != "enqueued" {
		status = http.StatusOK
	}
	writeJSON(w, status, res)
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	all, err := s.cfg.Backend.List(r.Context(), 500)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := []RunItem{}
	for _, it := range all {
		if len(out) < limit && canSee(a, it) {
			it.Jobs = nil // the list shows runs, not jobs
			out = append(out, it)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": out})
}

// find resolves ref for the caller; "latest" and "" mean the newest run the caller may see. A run the
// caller may not see is not found.
func (s *Server) find(r *http.Request, a roles.Actor, ref string) (RunItem, error) {
	if len(ref) > 128 {
		return RunItem{}, Failf(ReasonNotFound, "no such run")
	}
	if ref == "" || ref == "latest" {
		all, err := s.cfg.Backend.List(r.Context(), 500)
		if err != nil {
			return RunItem{}, err
		}
		for _, it := range all {
			if canSee(a, it) {
				ref = it.ID
				break
			}
		}
		if ref == "" || ref == "latest" {
			return RunItem{}, Failf(ReasonNotFound, "no run")
		}
	}
	it, ok, err := s.cfg.Backend.Get(r.Context(), ref)
	if err != nil {
		return RunItem{}, err
	}
	if !ok || !canSee(a, it) {
		return RunItem{}, Failf(ReasonNotFound, "no such run")
	}
	if err = authorizeRun(r.Context(), a, it); err != nil {
		if roles.ReasonOf(err) == "" {
			return RunItem{}, err // the store or the audit log failed: say so, do not call it "not found"
		}
		return RunItem{}, Failf(ReasonNotFound, "no such run")
	}
	return it, nil
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	it, err := s.find(r, a, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	it, err := s.find(r, a, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	if err = s.auditFirst(r.Context(), a, "run.cancel", it.ID, "project="+it.Project); err != nil {
		s.fail(w, err)
		return
	}
	n, err := s.cfg.Backend.Cancel(r.Context(), it.ID, a.Label())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": it.ID, "cancelled": n})
}
