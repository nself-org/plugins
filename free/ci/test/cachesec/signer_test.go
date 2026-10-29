//go:build cachesec

package cachesec

import (
	"context"
	"encoding/base64"

	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	cisigning "github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// signerAnswer replays what the isolated signer process writes for a refused request (its stdout and
// exit status: the wire contract internal/signing's client tests pin).
type signerAnswer struct {
	out  string
	code int
}

func (a signerAnswer) RunStdio(context.Context, cisigning.StdioCall) ([]byte, int, error) {
	return []byte(a.out), a.code, nil
}

// coordinatorSign asks the coordinator's signer client, over the rig store, to sign a manifest of a
// finished run; the signer answers digest_not_committed. It returns the client's refusal.
func (r *rig) coordinatorSign() error {
	r.running("release", owner())
	must(r.t, r.st.Transition(r.ctx, "release", "running", "finalizing", store.TransitionDetail{Epoch: 2}))
	must(r.t, r.st.Transition(r.ctx, "release", "finalizing", "passed", store.TransitionDetail{Epoch: 3}))
	answer := signerAnswer{out: `{"error":{"code":"E717","reason":"` + cisigning.ReasonDigestNotCommitted + `","message":"artifact app is not a committed record of the release attempt"}}`, code: 2}
	c := &cisigning.Client{Runner: answer, Src: r.st, Audit: r.st.AppendAudit}
	envs, err := c.Sign(r.ctx, "prelease", []cisigning.Item{{PayloadType: manifest.PayloadType, PayloadB64: base64.StdEncoding.EncodeToString([]byte(`{}`))}})
	if len(envs) != 0 {
		r.t.Error("a refused signing request returned envelopes")
	}
	return err
}
