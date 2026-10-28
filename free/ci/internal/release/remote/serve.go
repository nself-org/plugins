package remote

// Purpose: the two user-API handlers a TEAM coordinator serves so the operator can sign a
// release on their own machine: GET /v1/runs/{id}/release-inputs (facts, evidence, artifact and
// SBOM records) and GET /v1/runs/{id}/release-blobs/{sha256} (one blob, streamed).
// Inputs:  the authenticated caller (clientapi.ActorFrom), the run id and digest from the path.
// Outputs: JSON, or the blob bytes; a refusal body {code, reason, message}.
// Constraints: the caller must hold the release role (E687, audited by roles.Authorize) and
// must hold it on the run's own project (roles.Authorize(actor, release, project)); the project
// and the blob realm come from the stored run and record, never from the caller; a digest that
// is not in this run's release inputs, a run of another project and a run that does not exist
// all answer the same E713; a cache realm is unreachable because only artifact records are
// listed; the coordinator never resolves a secret here and holds no key; nothing but ids, the
// reason key and a fixed sentence reaches an error body.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

// Handlers serves both routes over the Deps the provider returns; ok is false until the
// coordinator has started (the answer is then a plain "not found").
type Handlers struct {
	Deps func() (Deps, bool)
	Logf func(format string, a ...any)
}

// InputsHandler is GET /v1/runs/{id}/release-inputs.
func (h Handlers) InputsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, in, ok := h.authorized(w, r, false)
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(in)
	})
}

// BlobHandler is GET /v1/runs/{id}/release-blobs/{sha256}.
func (h Handlers) BlobHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, in, ok := h.authorized(w, r, true)
		if !ok {
			return
		}
		digest := r.PathValue("sha256")
		rec, found := in.blob(digest)
		if !shaRe.MatchString(digest) || !found {
			refuse(w, errNotFound)
			return
		}
		realm := cas.ArtifactRealm(rec.Project)
		info, err := d.Blobs.Stat(realm, digest)
		if err != nil {
			h.fault(w, "release blob stat", errors.New("blob missing"))
			return
		}
		rc, err := d.Blobs.Open(realm, digest)
		if err != nil {
			h.fault(w, "release blob open", err)
			return
		}
		defer func() { _ = rc.Close() }()
		w.Header().Set("Content-Type", "application/octet-stream")
		if info.Size() == rec.Size { // a resized blob goes out as stored (chunked): the operator's re-hash is what rejects it
			w.Header().Set("Content-Length", strconv.FormatInt(rec.Size, 10))
		}
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return // headers only: the mux answers HEAD on a GET route, and no byte of the blob is read for it
		}
		_, _ = io.Copy(w, io.LimitReader(rc, MaxBlobBytes+1)) // the operator re-hashes: a truncated, grown or altered stream fails there
	})
}

// authorized runs the checks common to both routes and returns the caller's inputs; the role
// check (and its audit row) runs on every request, only the collect may come from Deps.Cache
// when cached is set.
func (h Handlers) authorized(w http.ResponseWriter, r *http.Request, cached bool) (Deps, Inputs, bool) {
	ctx := r.Context()
	d, ok := h.deps()
	run := r.PathValue("id")
	if !ok || !ValidRunID(run) {
		refuse(w, errNotFound)
		return Deps{}, Inputs{}, false
	}
	actor, ok := clientapi.ActorFrom(ctx)
	if !ok {
		refuse(w, fail(roles.CodeNotAuthorized, roles.ReasonNoActor, "no identity"))
		return Deps{}, Inputs{}, false
	}
	if actor.Role().Rank() < roles.Release.Rank() { // the role is checked (and audited) before any run is looked up
		refuse(w, roleError(roles.Authorize(ctx, actor, roles.Release, "")))
		return Deps{}, Inputs{}, false
	}
	project, err := d.resolve(ctx, run)
	if err == nil {
		err = scopeError(roles.Authorize(ctx, actor, roles.Release, project))
	}
	var in Inputs
	if err == nil {
		in, err = d.collectCached(ctx, run, authz.Role(actor.Role()), cached)
	}
	if err != nil {
		h.refuseErr(w, err)
		return Deps{}, Inputs{}, false
	}
	return d, in, true
}

func (h Handlers) deps() (Deps, bool) {
	if h.Deps == nil {
		return Deps{}, false
	}
	d, ok := h.Deps()
	return d, ok && d.Runs != nil && d.Store != nil && d.Blobs != nil
}

// scopeError turns a refusal on the run's project into the same answer as "not found" (a
// release holder of another project must not learn the run exists) and keeps every other
// role refusal as E687.
func scopeError(err error) error {
	switch {
	case err == nil:
		return nil
	case roles.ReasonOf(err) == roles.ReasonProject:
		return errNotFound
	}
	return roleError(err)
}

// roleError maps an E687 error of the role service to the wire error with the same reason key.
func roleError(err error) error {
	if err == nil {
		return fail(roles.CodeNotAuthorized, roles.ReasonRole, "the caller may not do this")
	}
	reason := roles.ReasonOf(err)
	if reason == "" {
		reason = roles.ReasonRole
	}
	return fail(roles.CodeNotAuthorized, reason, "the caller may not do this")
}

func (h Handlers) refuseErr(w http.ResponseWriter, err error) {
	var re *Error
	if errors.As(err, &re) {
		refuse(w, re)
		return
	}
	h.fault(w, "release inputs", err)
}

// fault answers 500 with a fixed body; the text of err stays in the log.
func (h Handlers) fault(w http.ResponseWriter, what string, err error) {
	if h.Logf != nil {
		h.Logf("release route: %s: %v", what, err)
	}
	writeBody(w, http.StatusInternalServerError, clientapi.CodeUserAPI, clientapi.ReasonInternal, "internal error")
}

// refuse writes the wire error; E687 is 403, E713 is 404, anything else 400.
func refuse(w http.ResponseWriter, err error) {
	var re *Error
	if !errors.As(err, &re) {
		writeBody(w, http.StatusInternalServerError, clientapi.CodeUserAPI, clientapi.ReasonInternal, "internal error")
		return
	}
	status := http.StatusBadRequest
	switch re.ErrCode {
	case roles.CodeNotAuthorized:
		status = http.StatusForbidden
	case errNotFound.(*Error).ErrCode:
		status = http.StatusNotFound
	}
	writeBody(w, status, re.ErrCode, re.Reason, re.Msg)
}

func writeBody(w http.ResponseWriter, status int, code, reason, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(clientapi.Body{Code: code, Reason: reason, Message: msg})
}
