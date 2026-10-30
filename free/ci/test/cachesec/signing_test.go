//go:build cachesec

package cachesec

// Purpose: the release-signing and control-plane cases of the matrix (threat rows TM-17, TM-18, TM-24):
//          a manifest signed by a plugins or agent key, a signer request for a digest no record holds, a TEAM
//          member fetching release inputs and a job reaching the release path.
// Inputs:  the rig; the case id.
// Outputs: the refusal each real component gives, or nil when it admitted the action.
// Constraints: manifest-*-key drives release/verify with an operator trust file that holds one ci-release
//          key and a manifest signed by a key of another purpose; the role cases issue real tokens and ask
//          roles.Authorize, with a release-role control that must pass so the refusal is not blanket;
//          sign-uncommitted runs the owning package's own named refusal test, because the signer's store
//          fixture is that package's test code.

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/release/verify"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

func seed(n byte) ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed([]byte(strings.Repeat(string(rune('a'+n)), 32)))
}

// manifestKey verifies a release whose manifest envelope is signed by a key that is not a ci-release key.
func (r *rig) manifestKey(id string) error {
	trusted := seed(1)
	line := signing.KeyID(signing.PurposeCIRelease, trusted.Public().(ed25519.PublicKey)) + " " +
		base64.StdEncoding.EncodeToString(trusted.Public().(ed25519.PublicKey)) + "\n"
	dir := r.t.TempDir()
	keys := filepath.Join(dir, "ci-release.keys")
	must(r.t, os.WriteFile(keys, []byte(line), 0o600))
	purpose := signing.PurposePlugins
	if id == "manifest-agent-key" {
		purpose = signing.PurposeAgent
	}
	env, err := signing.SignEnvelope(signing.NewEd25519Signer(purpose, seed(2)), manifest.PayloadType, []byte(`{}`))
	must(r.t, err)
	b, err := json.Marshal(env)
	must(r.t, err)
	rel := filepath.Join(dir, "release")
	must(r.t, os.Mkdir(rel, 0o700))
	must(r.t, os.WriteFile(filepath.Join(rel, verify.ManifestFile), b, 0o600))
	res := verify.Release(r.ctx, rel, verify.Options{OperatorKeys: keys, OperatorRevoked: filepath.Join(dir, "none"), Audit: r.st.AppendAudit})
	if res.Verified {
		return nil
	}
	return &refusal{Code: res.Code, Reason: res.Reason, Msg: res.Message}
}

// roleRefusal asks the role service for a release-level action as a member-role token.
func (r *rig) roleRefusal(id string) error {
	svc := roles.New(r.st, &audit.Log{St: r.st})
	svc.Self = "alice"
	op := svc.Operator("cli:alice")
	token := func(principal string, role roles.Role) roles.Actor {
		tok, _, err := svc.IssueToken(r.ctx, op, roles.Request{Principal: principal, Role: role, Projects: []string{"project"}})
		must(r.t, err)
		a, err := svc.ActorFromToken(r.ctx, tok)
		must(r.t, err)
		return a
	}
	ask := func(a roles.Actor) error {
		if id == "job-signer" {
			return roles.AuthorizeAction(r.ctx, a, roles.ActReleaseStart, "project")
		}
		return roles.Authorize(r.ctx, a, roles.Release, "project")
	}
	if err := ask(token("carol", roles.Release)); err != nil {
		r.t.Fatalf("a release-role control was refused: %v", err)
	}
	principal := "bob"
	if id == "job-signer" {
		principal = "job-attempt-1"
	}
	return ask(token(principal, roles.Member))
}

// signUncommitted runs the signer's own refusal case for a digest that is not a committed record, then
// hands that refusal, as the signer process answers it, to the coordinator's signer client.
func (r *rig) signUncommitted() (error, string) {
	const test = "internal/signing TestSignerRefuses/artifact_digest_is_not_a_record"
	cmd := exec.Command("go", "test", "-mod=vendor", "-count=1", "-run", `^TestSignerRefuses$/^artifact_digest_is_not_a_record$`, "./internal/signing/")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("%s did not pass: %v\n%s", test, err, out)
	}
	return r.coordinatorSign(), "signer check delegated to " + test + "; the coordinator client received the signer's E717 answer and audited it"
}
