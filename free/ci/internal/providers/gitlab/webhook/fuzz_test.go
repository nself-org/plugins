package webhook

// Purpose: fuzz targets FuzzGitLabWebhook (the whole Verify path on arbitrary bodies) and
// FuzzStandardWebhooksSignature (the signature and timestamp check on arbitrary headers) for
// acceptance line 4.
// Inputs:  the recorded payloads and hostile headers as seeds.
// Outputs: no panic; every success validates and is explainable; only the declared error types come back.
// Constraints: the fuzz runs offline (the API client always fails), so a payload can never reach the
//          network; run with `go test -fuzz FuzzGitLabWebhook -fuzztime 180s` and likewise for the
//          signature target.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// FuzzGitLabWebhook feeds arbitrary event names and bodies through Verify. The source is configured
// with a bare token for whatever project the body names, so authentication succeeds and the parser, the
// identity rules and the normalizers all run. It proves no crash and that any accepted delivery
// validates as trigger facts for a well-formed project and revision.
func FuzzGitLabWebhook(f *testing.F) {
	for event, name := range map[string]string{
		"Push Hook": "push", "Tag Push Hook": "tag_push", "Merge Request Hook": "mr_fork_open", "Job Hook": "job_pending_nself",
	} {
		f.Add(event, fixture(f, name))
	}
	for _, name := range []string{"push_delete", "push_new_branch", "push_to_tag_ref", "tag_push_nosha", "tag_push_delete", "mr_update_oldrev", "mr_update_title", "mr_close", "mr_other_target", "job_pending_untagged", "job_running"} {
		f.Add("Push Hook", fixture(f, name))
		f.Add("Merge Request Hook", fixture(f, name))
		f.Add("Job Hook", fixture(f, name))
	}
	f.Add("Push Hook", []byte(`{}`))
	f.Add("Push Hook", []byte(`{"object_kind":"push","project":{"id":1,"path_with_namespace":"o/r"},"after":"`+strings.Repeat("a", 40)+`","ref":"refs/heads/x"}`))
	f.Add("Job Hook", []byte(`{"object_kind":"build","project_id":1,"repository":{"homepage":"https://h/o/r.git"},"build_status":"pending","build_id":1,"pipeline_id":1,"sha":"`+strings.Repeat("b", 40)+`","tag_list":["nself"]}`))
	f.Fuzz(func(t *testing.T, event string, body []byte) {
		var probe glPayload
		_ = json.Unmarshal(body, &probe)
		name, _ := identity(probe)
		src := &Source{
			Now:    func() time.Time { return fixedNow },
			Getenv: func(string) string { return "synthetic-fuzz-token" },
			Policy: func(context.Context) (Secrets, error) {
				return Secrets{Tokens: map[string]RepoSecret{strings.ToLower(name): {Env: "SYNTH"}}}, nil
			},
			NewAPI: func(context.Context) (*rest.Client, error) { return nil, errors.New("offline") },
		}
		d := delivery{event: event, uuid: "u", token: "synthetic-fuzz-token"}
		facts, err := src.Verify(context.Background(), d.request(body), body)
		if err != nil {
			var dec *Decline
			var pe *providers.Error
			if !errors.As(err, &dec) && !errors.As(err, &pe) {
				t.Fatalf("undeclared error type %T: %v", err, err)
			}
			return
		}
		if verr := facts.Validate(); verr != nil {
			t.Fatalf("accepted facts do not validate: %v", verr)
		}
		if !coordinator.ValidProject(facts.Repo) || !coordinator.ValidRevision(facts.Revision) || facts.Source != Name || facts.Auth != "bearer" || len(facts.DeliveryID) > maxIDLen {
			t.Fatalf("accepted facts name a bad project, revision, source or auth: %+v", facts)
		}
		// the same bytes with no token must never be accepted
		bad := delivery{event: event}
		if _, err = src.Verify(context.Background(), bad.request(body), body); err == nil {
			t.Fatal("a delivery with no credentials was accepted")
		}
	})
}

// FuzzStandardWebhooksSignature feeds arbitrary secrets, ids, timestamps and signature headers to the
// check. It proves no crash, that a success is explained by a listed entry equal to the HMAC of
// id.timestamp.body and a timestamp inside the window, that a correct signature always verifies, and
// that it never verifies another body.
func FuzzStandardWebhooksSignature(f *testing.F) {
	body := fixture(f, "push")
	ts := strconv.FormatInt(fixedNow.Unix(), 10)
	good := stdSignature(signingSecret, "msg-1", ts, body)
	f.Add(signingSecret, "msg-1", ts, good, body)
	f.Add(signingSecret, "msg-1", ts, "v1,AAAA "+good, body)
	f.Add(signingSecret, "msg-1", ts, strings.Repeat("v1,AAAA ", 40)+good, body)
	f.Add(signingSecret, "msg-1", "1", good, body)
	f.Add(signingSecret, "", ts, good, body)
	f.Add("whsec_", "msg-1", ts, good, body)
	f.Add("whsec_!!!!", "msg-1", ts, good, body)
	f.Add("", "msg-1", ts, "", []byte(nil))
	f.Add(signingSecret, "msg-1", "-9223372036854775808", "v1,", body)
	f.Add(signingSecret, "msg-1", "99999999999999999999", good, body)
	f.Fuzz(func(t *testing.T, secret, id, tsv, header string, data []byte) {
		err := verifySigned(secret, id, tsv, header, data, fixedNow)
		if err != nil {
			var pe *providers.Error
			if !errors.As(err, &pe) || pe.Code != "E706" || pe.Status != http.StatusForbidden || (pe.Reason != reasonSignature && pe.Reason != reasonTimestamp) {
				t.Fatalf("undeclared error %T: %v", err, err)
			}
		}
		_, keyOK := decodeKey(secret)
		inputsOK := keyOK && id != "" && len(id) <= maxIDLen && tsv != "" && header != "" && len(header) <= maxSigHeader
		if err == nil {
			if !inputsOK {
				t.Fatal("accepted with unusable inputs")
			}
			listed := listsMAC(header, secret, id, tsv, data)
			sec, perr := strconv.ParseInt(tsv, 10, 64)
			age := fixedNow.Sub(time.Unix(sec, 0))
			if !listed || perr != nil || age > maxSkew || age < -maxSkew {
				t.Fatalf("accepted without a matching entry or inside the window: listed=%v perr=%v age=%v", listed, perr, age)
			}
		}
		if !inputsOK {
			return
		}
		sec, perr := strconv.ParseInt(tsv, 10, 64)
		if perr != nil || strconv.FormatInt(sec, 10) != tsv {
			return // a timestamp that is not canonical decimal is outside what the properties below describe
		}
		age := fixedNow.Sub(time.Unix(sec, 0))
		if age > maxSkew || age < -maxSkew {
			return
		}
		right := stdSignature(secret, id, tsv, data)
		if verifySigned(secret, id, tsv, right, data, fixedNow) != nil {
			t.Fatal("a correct signature inside the window was refused")
		}
		if verifySigned(secret, id, tsv, right, append(append([]byte{}, data...), 'x'), fixedNow) == nil {
			t.Fatal("a signature verified another body")
		}
	})
}

// listsMAC reports whether header lists a v1 entry whose decoded bytes are the HMAC of id.ts.body (the
// check compares decoded bytes, so this oracle decodes too, with its own code path).
func listsMAC(header, secret, id, ts string, body []byte) bool {
	key, _ := decodeKey(secret)
	m := hmac.New(sha256.New, key)
	m.Write([]byte(id + "." + ts + "."))
	m.Write(body)
	want := m.Sum(nil)
	for _, e := range strings.Fields(header) {
		if enc, ok := strings.CutPrefix(e, "v1,"); ok {
			if got, err := base64.StdEncoding.DecodeString(enc); err == nil && bytes.Equal(got, want) {
				return true
			}
		}
	}
	return false
}
