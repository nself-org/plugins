package coordinator

// Purpose: the durable intake of the V15 coordinator (P7-NODE-10, Epic D7): the webhook handler (the v1.4
// path and registered Ingresses), the idempotency-key API for manual triggers and Enqueue for non-HTTP
// sources. Every route stores the trigger before it answers 2xx.
// Inputs:  HTTP requests, Delivery values from Ingresses and Reconcilers.
// Outputs: 202 (stored, or coalesced into a live pipeline), 200 (duplicate delivery id or key replay),
//          400, 403, 409 (key reused for another request, E608), 413, 503 (store unavailable only).
// Constraints: the HMAC check, the allowlist and the event parser are the v1.4 ones (internal/serve); a
//          request an Ingress matched is verified only by it; no body or header is echoed in an answer or
//          a log; the semantic key is (project, revision, trigger class, policy digest).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/serve"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// MaxBody is the one body cap, checked before any Accept or signature work.
const MaxBody = 25 << 20

// bigBody is the size above which a request body takes one of the bodySlots while it is read.
const bigBody = 1 << 20

// bodySlots bounds the large bodies buffered at once (at most 16 x 25 MiB) before authentication.
var bodySlots = make(chan struct{}, 16)

// maxDeliveryID is the longest delivery id the intake stores.
const maxDeliveryID = 256

// Outcome says what an enqueue did: enqueued, duplicate, coalesced or replay, and the row it stands for.
type Outcome = store.IntakeOutcome

type enqueueOpts struct {
	force                  bool
	actor, idem, reqDigest string
}

// Enqueue stores a trigger from a non-HTTP source (a schedule, a Reconciler).
func (c *Coordinator) Enqueue(ctx context.Context, d Delivery) (Outcome, error) {
	return c.enqueue(ctx, d, false, enqueueOpts{})
}

func (c *Coordinator) enqueue(ctx context.Context, d Delivery, webhook bool, o enqueueOpts) (Outcome, error) {
	d.Revision = strings.ToLower(d.Revision)
	if d.Source == "" || !ValidProject(d.Project) || !ValidRevision(d.Revision) || (d.Pipeline != "" && !ValidPipeline(d.Pipeline)) {
		return Outcome{}, newErr("", ReasonInvalidDelivery, "a delivery needs a source, an owner/repo project, a full hex revision and a plain pipeline name")
	}
	if len(d.DeliveryID) > maxDeliveryID {
		return Outcome{}, newErr("", ReasonInvalidDelivery, "delivery id is too long")
	}
	if d.TriggerClass == "" {
		d.TriggerClass = "change"
	}
	keyPipeline := d.Pipeline
	facts := map[string]json.RawMessage{}
	if len(d.Facts) > 0 && json.Unmarshal(d.Facts, &facts) != nil {
		return Outcome{}, newErr("", ReasonInvalidDelivery, "delivery facts must be a JSON object")
	}
	if d.Synthetic != nil {
		raw, err := json.Marshal(d.Synthetic)
		if err != nil {
			return Outcome{}, newErr("", ReasonInvalidDelivery, "synthetic pipeline cannot be stored")
		}
		facts["synthetic"] = raw
		sum := sha256.Sum256(raw)
		keyPipeline += "|synthetic=" + hex.EncodeToString(sum[:8])
	}
	var event string
	_ = json.Unmarshal(facts["trigger"], &event)
	blob, _ := json.Marshal(facts)
	out, err := c.cfg.Store.IntakeEnqueue(ctx, store.IntakeRequest{
		Source: d.Source, Event: event, DeliveryID: d.DeliveryID, Project: d.Project, Revision: d.Revision, TriggerClass: d.TriggerClass,
		PolicyDigest: c.cfg.PolicyDigest, SemanticKey: semanticKey(strings.ToLower(d.Project), d.Revision, d.TriggerClass, c.cfg.PolicyDigest, keyPipeline),
		Pipeline: d.Pipeline, Facts: blob, Webhook: webhook, Force: c.cfg.Force || o.force, Actor: o.actor,
		IdemDigest: o.idem, ReqDigest: o.reqDigest,
	})
	if err != nil {
		var se *store.Error
		if errors.As(err, &se) && se.Code == CodeIdempotency {
			return out, newErr(CodeIdempotency, ReasonKeyReused, "idempotency key already used for a different request")
		}
		return out, newErr(CodeStore, ReasonStoreUnavailable, "intake store unavailable")
	}
	if out.Kind == store.IntakeEnqueued {
		c.nudge()
	}
	return out, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// answer maps an enqueue result to the response.
func answer(w http.ResponseWriter, d Delivery, out Outcome, err error) {
	switch {
	case ReasonOf(err) == ReasonKeyReused:
		http.Error(w, "idempotency key already used for a different request", http.StatusConflict)
	case ReasonOf(err) == ReasonInvalidDelivery:
		http.Error(w, "invalid delivery", http.StatusBadRequest)
	case err != nil:
		http.Error(w, "intake store unavailable; retry later", http.StatusServiceUnavailable)
	default:
		code := http.StatusAccepted
		if out.Kind == store.IntakeDuplicate || out.Kind == store.IntakeReplay {
			code = http.StatusOK
		}
		sha := d.Revision[:min(7, len(d.Revision))]
		writeJSON(w, code, map[string]string{"accepted": "true", "repo": d.Project, "sha": sha, "intake": out.ID, "outcome": out.Kind})
	}
}

// serveWebhook handles /webhook and POST /.
func (c *Coordinator) serveWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var ing Ingress
	for _, i := range registeredIngresses() {
		if i.Match(r) {
			ing = i
			break
		}
	}
	if r.ContentLength > MaxBody {
		http.Error(w, "body too large (limit 25 MiB)", http.StatusRequestEntityTooLarge)
		return
	}
	if r.ContentLength < 0 { // GitHub and every provider send a length; an unknown one is a slow-body attack
		http.Error(w, "Content-Length required", http.StatusLengthRequired)
		return
	}
	if r.ContentLength > bigBody { // large: bound how many sit in memory before authentication
		select {
		case bodySlots <- struct{}{}:
			defer func() { <-bodySlots }()
		default:
			w.Header().Set("Retry-After", "5")
			http.Error(w, "too many large requests in flight; retry later", http.StatusTooManyRequests)
			return
		}
	}
	if ing == nil && c.cfg.Secret == "" && !c.cfg.Insecure { // the v1.4 path cannot authenticate: refuse before reading the body
		c.cfg.Logf("refused an unmatched webhook: no shared secret and not --insecure")
		http.Error(w, "signature required", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxBody+1))
	switch {
	case err != nil:
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	case len(body) > MaxBody:
		http.Error(w, "body too large (limit 25 MiB)", http.StatusRequestEntityTooLarge)
		return
	}
	if ing != nil {
		d, err := ing.Accept(r.Context(), r, body)
		var declined interface{ DeclineReason() string }
		if errors.As(err, &declined) { // verified but not for the intake (a runner demand nobody serves): 200, nothing enqueued
			c.cfg.Logf("ingress %s declined a delivery", ing.Name())
			writeJSON(w, http.StatusOK, map[string]any{"accepted": false, "reason": declined.DeclineReason()})
			return
		}
		if err != nil {
			c.cfg.Logf("ingress %s refused a delivery", ing.Name())
			http.Error(w, "delivery refused", http.StatusForbidden)
			return
		}
		// an Ingress holding per-repo secrets authorises its own repos (it must check the secret belongs to
		// Delivery.Project); any other Ingress is held to --allowed-repos like the v1.4 path
		if h, ok := ing.(SecretHolder); !ok || !h.HasSecrets() {
			if !serve.RepoAllowed(d.Project, c.cfg.AllowedRepos) {
				c.cfg.Logf("ingress %s: repo %q is not in --allowed-repos", ing.Name(), d.Project)
				http.Error(w, "repo is not allowlisted", http.StatusForbidden)
				return
			}
		}
		out, err := c.enqueue(r.Context(), d, true, enqueueOpts{})
		answer(w, d, out, err)
		return
	}
	if secret := c.cfg.Secret; secret != "" {
		if err := serve.VerifySignature(secret, r.Header.Get("X-Hub-Signature-256"), body); err != nil {
			c.cfg.Logf("webhook signature mismatch from %s", r.RemoteAddr)
			http.Error(w, "signature mismatch", http.StatusForbidden)
			return
		}
	}
	ev, err := serve.ParseWebhook(r.Header.Get("X-GitHub-Event"), body)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"accepted": false, "reason": err.Error()})
		return
	}
	if !serve.RepoAllowed(ev.RepoFullName, c.cfg.AllowedRepos) {
		c.cfg.Logf("rejected: repo %q is not in --allowed-repos", ev.RepoFullName)
		http.Error(w, fmt.Sprintf("repo %q is not allowlisted", ev.RepoFullName), http.StatusForbidden)
		return
	}
	facts, _ := json.Marshal(map[string]string{"trigger": ev.EventType, "ref": ev.Ref, "clone_url": ev.CloneURL})
	d := Delivery{Source: "github", DeliveryID: r.Header.Get("X-GitHub-Delivery"), Project: ev.RepoFullName, Revision: ev.SHA, TriggerClass: "change", Facts: facts}
	out, err := c.enqueue(r.Context(), d, true, enqueueOpts{})
	answer(w, d, out, err)
}
