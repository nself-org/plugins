package signing

// Purpose: the coordinator side of the signer (Epic CACHE D12): start
// `nself-ci artifacts signer` through the exec funnel after the run, hand it
// one request and read the envelopes back (cap:ci.release-signing: Sign,
// PublicKey; P7-CACHE-17 and P7-CACHE-18 call it).
// Inputs:  a Runner, the run's Source and a run id with payloads.
// Outputs: DSSE envelopes in request order, or an E717 refusal.
// Constraints: the signer's answer is checked for shape (one ci-release key id and one
// 64-byte signature per item); its cryptographic check belongs to the verifier
// that holds the trust file (P7-CACHE-09/18); the signer starts only when every attempt of the run is
// terminal (the coordinator refuses itself, the signer checks again); it runs
// outside every job's process group, namespace and container (the funnel
// starts it as its own process); a timeout, a crash, a missing or malformed
// answer or any wrong envelope fails the release closed with E717 and returns
// no partial signature; the client never holds a key.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

// DefaultTimeout bounds one signer call.
const DefaultTimeout = time.Minute

// Client starts the signer.
type Client struct {
	Runner StdioRunner
	// Src is read only for the terminal check.
	Src Source
	// Exe is the nself-ci binary ("" = this process's own).
	Exe string
	// Env is added to the signer's environment (NSELF_CI_HOME, NSELF_CI_REPO).
	Env     []string
	Timeout time.Duration
	// Team is set on a TEAM coordinator, which holds secret references only
	// (Epic X8): it never starts the signer and never resolves the release key.
	Team bool
}

func (c *Client) exe() (string, error) {
	if c.Exe != "" {
		return c.Exe, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", Failf(ReasonSignerFailed, "the signer binary could not be located")
	}
	return exe, nil
}

func (c *Client) call(ctx context.Context, mode string, stdin []byte) ([]byte, error) {
	if c.Runner == nil {
		return nil, Failf(ReasonSignerFailed, "the signer client has no runner")
	}
	if c.team() {
		return nil, errTeam()
	}
	exe, err := c.exe()
	if err != nil {
		return nil, err
	}
	t := c.Timeout
	if t <= 0 {
		t = DefaultTimeout
	}
	env := append([]string{"NSELF_V15=1"}, c.Env...)
	out, code, err := c.Runner.RunStdio(ctx, StdioCall{Argv: []string{exe, "artifacts", "signer", mode}, Env: env, Stdin: stdin, Timeout: t})
	if err != nil {
		return nil, Failf(ReasonSignerFailed, "the signer did not run or timed out")
	}
	if code != 0 {
		return nil, signerRefusal(out, code)
	}
	return out, nil
}

// team reports whether this client runs on a TEAM coordinator: the field, or
// the coordinator's own environment (the signer's child environment is
// allowlisted, so the mode must be read here, in the caller's process).
func (c *Client) team() bool { return c.Team || os.Getenv("NSELF_CI_COORDINATOR_MODE") == "team" }

// errTeam is the refusal on a TEAM coordinator.
func errTeam() error {
	return Failf(ReasonSignerFailed, "a TEAM coordinator never holds the release key: sign on the operator's machine (nself ci artifacts sign <run> --coordinator <url>)")
}

// knownReason reports whether a relayed reason is one of E717's.
func knownReason(r string) bool {
	switch r {
	case ReasonKeyMissing, ReasonKeyInvalid, ReasonKeyIDMismatch, ReasonPayloadRejected, ReasonDigestNotCommitted, ReasonSignerFailed, ReasonCLITooOld, ReasonSecretExists:
		return true
	}
	return false
}

// signerRefusal turns the signer's error object into the same E717 reason.
func signerRefusal(out []byte, code int) error {
	var resp Response
	if i := bytes.IndexByte(out, '{'); i >= 0 && json.Unmarshal(out[i:], &resp) == nil && resp.Error != nil && knownReason(resp.Error.Reason) {
		return Failf(resp.Error.Reason, "%s", clip(resp.Error.Message, 509))
	}
	return Failf(ReasonSignerFailed, "the signer failed (exit %d)", code)
}

// Sign signs the payloads of a finished run, or refuses.
func (c *Client) Sign(ctx context.Context, runID string, items []Item) ([]signing.Envelope, error) {
	if c.Src == nil || c.Runner == nil {
		return nil, Failf(ReasonSignerFailed, "the signer client has no run store or runner")
	}
	if c.team() {
		return nil, errTeam()
	}
	if err := CheckTerminal(ctx, c.Src, runID); err != nil {
		return nil, err
	}
	req, err := json.Marshal(Request{RunID: runID, Items: items})
	if err != nil {
		return nil, Failf(ReasonPayloadRejected, "the request could not be encoded")
	}
	if len(req) > MaxRequestBytes {
		return nil, Failf(ReasonPayloadRejected, "the request is larger than %d bytes", MaxRequestBytes)
	}
	out, err := c.call(ctx, "--stdio", req)
	if err != nil {
		return nil, err
	}
	var resp Response
	if i := bytes.IndexByte(out, '{'); i < 0 || json.Unmarshal(out[i:], &resp) != nil || resp.Error != nil {
		return nil, Failf(ReasonSignerFailed, "the signer answered nothing usable")
	}
	if err := sane(items, resp.Envelopes); err != nil {
		return nil, err
	}
	return resp.Envelopes, nil
}

// sane checks the answer is one envelope per item, over that item's payload,
// with exactly one signature, so a broken signer cannot hand back a mix.
func sane(items []Item, envs []signing.Envelope) error {
	if len(envs) != len(items) {
		return Failf(ReasonSignerFailed, "the signer answered %d envelopes for %d items", len(envs), len(items))
	}
	for i, e := range envs {
		if e.PayloadType != items[i].PayloadType || e.Payload != items[i].PayloadB64 || len(e.Signatures) != 1 {
			return Failf(ReasonSignerFailed, "envelope %d does not match its request item", i)
		}
		sig, err := base64.StdEncoding.Strict().DecodeString(e.Signatures[0].Sig)
		if err != nil || len(sig) != ed25519.SignatureSize || !keyIDRe.MatchString(e.Signatures[0].KeyID) {
			return Failf(ReasonSignerFailed, "envelope %d carries a malformed key id or signature", i)
		}
	}
	return nil
}

// PublicKey asks the signer for the trust line of the release key.
func (c *Client) PublicKey(ctx context.Context) (KeyInfo, error) { return c.keyCall(ctx, "--public") }

// Init asks the signer to create the release key.
func (c *Client) Init(ctx context.Context) (KeyInfo, error) { return c.keyCall(ctx, "--init") }

// Rotate asks the signer to replace the release key.
func (c *Client) Rotate(ctx context.Context) (KeyInfo, error) { return c.keyCall(ctx, "--rotate") }

func (c *Client) keyCall(ctx context.Context, mode string) (KeyInfo, error) {
	out, err := c.call(ctx, mode, nil)
	if err != nil {
		return KeyInfo{}, err
	}
	var info KeyInfo
	if i := bytes.IndexByte(out, '{'); i < 0 || json.Unmarshal(out[i:], &info) != nil || info.KeyID == "" || info.Public == "" {
		return KeyInfo{}, Failf(ReasonSignerFailed, "the signer answered nothing usable")
	}
	return info, nil
}
