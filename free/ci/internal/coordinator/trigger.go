package coordinator

// Purpose: POST /v1/trigger, the manual-trigger API of the V15 coordinator (P7-NODE-10): an
// Idempotency-Key header makes a retry safe, and the signed body carries the key and a timestamp so a
// captured request cannot be replayed under another key or after its window.
// Inputs:  a JSON body {project, revision, pipeline, force, key, ts}, header Idempotency-Key, header
//          X-Hub-Signature-256 (HMAC of the body with the webhook secret, as for a webhook).
// Outputs: 202 (stored), 200 (replay), 400, 403, 409 (key reused for another request, E608), 503.
// Constraints: signed and allowlisted exactly like a webhook (fail closed without a secret unless
//          --insecure); body.key must equal the header; body.ts must be within triggerWindow of the
//          coordinator clock; project, revision and pipeline pass the same rules as every trigger.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/serve"
)

// triggerWindow is how far a signed trigger's timestamp may be from the coordinator clock.
const triggerWindow = 10 * time.Minute

type triggerRequest struct {
	Project, Revision, Pipeline, Key string
	Force                            bool
	TS                               int64
}

// serveTrigger handles POST /v1/trigger.
func (c *Coordinator) serveTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
	if err != nil || len(body) > 1<<20 {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if secret := c.cfg.Secret; secret != "" {
		if err := serve.VerifySignature(secret, r.Header.Get("X-Hub-Signature-256"), body); err != nil {
			http.Error(w, "signature mismatch", http.StatusForbidden)
			return
		}
	} else if !c.cfg.Insecure {
		http.Error(w, "manual triggers need the webhook secret", http.StatusForbidden)
		return
	}
	var req triggerRequest
	key := r.Header.Get("Idempotency-Key")
	if json.Unmarshal(body, &req) != nil || !keyRe.MatchString(key) || req.Key != key {
		http.Error(w, "need an Idempotency-Key of 8 to 128 characters that the signed body repeats as \"key\"", http.StatusBadRequest)
		return
	}
	if d := c.cfg.Clock.Now().Sub(time.Unix(req.TS, 0)); d > triggerWindow || d < -triggerWindow {
		http.Error(w, "stale or missing \"ts\" (unix seconds, within 10 minutes of the coordinator clock)", http.StatusBadRequest)
		return
	}
	req.Revision = strings.ToLower(req.Revision)
	if !ValidProject(req.Project) || !ValidRevision(req.Revision) || (req.Pipeline != "" && !ValidPipeline(req.Pipeline)) {
		http.Error(w, "need project owner/repo, a full hex revision and a plain pipeline name", http.StatusBadRequest)
		return
	}
	if !serve.RepoAllowed(req.Project, c.cfg.AllowedRepos) {
		http.Error(w, fmt.Sprintf("repo %q is not allowlisted", req.Project), http.StatusForbidden)
		return
	}
	facts, _ := json.Marshal(map[string]string{"trigger": "api"})
	d := Delivery{Source: "manual", Project: req.Project, Revision: req.Revision, TriggerClass: "change", Pipeline: req.Pipeline, Facts: facts}
	kd, rd := sha256.Sum256([]byte("idem:"+key)), sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%t", d.Project, d.Revision, d.Pipeline, req.Force)))
	out, err := c.enqueue(r.Context(), d, false, enqueueOpts{force: req.Force, actor: "api", idem: hex.EncodeToString(kd[:]), reqDigest: hex.EncodeToString(rd[:])})
	answer(w, d, out, err)
}
