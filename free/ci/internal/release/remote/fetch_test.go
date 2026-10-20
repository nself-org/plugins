package remote

// Purpose: the operator's fetch half against a scripted coordinator: validation of an untrusted
// document, the re-hash of every blob, the sanitizing of a hostile refusal body.
// Constraints: no network, no process; every digest is the real sha256.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/release/verify"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

func asErr(err error, out **Error) bool { return errors.As(err, out) }

func hexSum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// script is a Fetcher that answers by path.
type script struct {
	status map[string]int
	body   map[string][]byte
	calls  []string
}

func (s *script) GetRaw(_ context.Context, p string) (*http.Response, error) {
	s.calls = append(s.calls, p)
	code := http.StatusOK
	if c, ok := s.status[p]; ok {
		code = c
	}
	b, ok := s.body[p]
	if !ok {
		code, b = http.StatusNotFound, []byte(`{"code":"E713","reason":"not_found","message":"no such artifact"}`)
	}
	return &http.Response{StatusCode: code, Body: io.NopCloser(bytes.NewReader(b))}, nil
}

const (
	tRun  = "run-1"
	tProj = "/srv/checkout"
	tRev  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func goodInputs(blobs ...[]byte) Inputs {
	in := Inputs{Schema: Schema, Run: tRun, Pipeline: Pipeline{ID: tRun, Project: tProj, Revision: tRev, InputDigest: "in"}, Evidence: []byte(`{"run_id":"run-1"}`),
		Jobs: []Job{{ID: "j1", Name: "publish", Kind: "release"}}, Attempts: []Attempt{{ID: "j1.a1", Job: "j1", N: 1, State: "passed"}}}
	for i, b := range blobs {
		in.Records = append(in.Records, records.Record{ID: "r", Project: tProj, Run: tRun, Attempt: "j1.a1", Revision: tRev,
			Name: "file" + string(rune('a'+i)), Kind: records.KindBinary, SHA256: hexSum(b), Size: int64(len(b)), State: records.StateCommitted})
	}
	return in
}

func serve(t *testing.T, in Inputs, blobs map[string][]byte) *script {
	t.Helper()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	s := &script{status: map[string]int{}, body: map[string][]byte{InputsPath(tRun): raw}}
	for d, b := range blobs {
		s.body[BlobPath(tRun, d)] = b
	}
	return s
}

func TestFetchRehashesEveryBlob(t *testing.T) {
	a, b := []byte("app-bytes"), []byte("sbom-bytes")
	in := goodInputs(a, b)
	good := map[string][]byte{hexSum(a): a, hexSum(b): b}
	snap, blobs, err := Fetch(context.Background(), serve(t, in, good), tRun, t.TempDir())
	if err != nil || snap == nil || blobs == nil {
		t.Fatalf("good fetch: %v", err)
	}
	if got, _, _ := snap.PipelineByID(context.Background(), tRun); got.Project != tProj {
		t.Fatalf("pipeline %+v", got)
	}

	cases := map[string][]byte{
		"altered":   []byte("app-BYTES"),
		"truncated": a[:3],
		"oversized": append(append([]byte{}, a...), "more"...),
		"empty":     nil,
	}
	for name, bad := range cases {
		s := serve(t, in, map[string][]byte{hexSum(a): bad, hexSum(b): b})
		_, _, err := Fetch(context.Background(), s, tRun, t.TempDir())
		var re *Error
		if err == nil || !asErr(err, &re) || re.ErrCode != verify.Code || re.Reason != verify.ReasonChecksum {
			t.Fatalf("%s: want E719 checksum, got %v", name, err)
		}
	}
}

func TestFetchRequestsOnlyListedDigestsOnce(t *testing.T) {
	a := []byte("same")
	in := goodInputs(a, a) // two records, one digest
	s := serve(t, in, map[string][]byte{hexSum(a): a})
	if _, _, err := Fetch(context.Background(), s, tRun, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if len(s.calls) != 2 || s.calls[0] != InputsPath(tRun) || s.calls[1] != BlobPath(tRun, hexSum(a)) {
		t.Fatalf("calls %v", s.calls)
	}
}

func TestValidateRefusesMalformedDocuments(t *testing.T) {
	ok := goodInputs([]byte("x"))
	mut := func(f func(*Inputs)) Inputs {
		in := goodInputs([]byte("x"))
		f(&in)
		return in
	}
	bad := map[string]Inputs{
		"schema":               mut(func(i *Inputs) { i.Schema = "other/v9" }),
		"run":                  mut(func(i *Inputs) { i.Run = "run-2" }),
		"pipeline id":          mut(func(i *Inputs) { i.Pipeline.ID = "run-2" }),
		"no evidence":          mut(func(i *Inputs) { i.Evidence = nil }),
		"foreign run":          mut(func(i *Inputs) { i.Records[0].Run = "run-2" }),
		"foreign proj":         mut(func(i *Inputs) { i.Records[0].Project = "/other" }),
		"bad digest":           mut(func(i *Inputs) { i.Records[0].SHA256 = strings.Repeat("G", 64) }),
		"short digest":         mut(func(i *Inputs) { i.Records[0].SHA256 = "abc" }),
		"negative size":        mut(func(i *Inputs) { i.Records[0].Size = -1 }),
		"huge size":            mut(func(i *Inputs) { i.Records[0].Size = MaxBlobBytes + 1 }),
		"traversal":            mut(func(i *Inputs) { i.Records[0].Name = "../../etc/passwd" }),
		"absolute":             mut(func(i *Inputs) { i.Records[0].Name = "/etc/passwd" }),
		"backslash":            mut(func(i *Inputs) { i.Records[0].Name = `a\b` }),
		"control id":           mut(func(i *Inputs) { i.Jobs[0].ID = "j1\x1b[31m" }),
		"too many":             mut(func(i *Inputs) { i.Records = make([]records.Record, MaxRecords) }),
		"unknown kind":         mut(func(i *Inputs) { i.Records[0].Kind = "rootkit" }),
		"control in platform":  mut(func(i *Inputs) { i.Records[0].Platform = "linux/amd64\x1b[31m" }),
		"control in image ref": mut(func(i *Inputs) { i.Records[0].Image = &records.ImageInfo{Ref: "reg/app\n:1"} }),
	}
	if err := validate(ok, tRun); err != nil {
		t.Fatalf("good document refused: %v", err)
	}
	for name, in := range bad {
		if err := validate(in, tRun); err == nil {
			t.Errorf("%s: a malformed document was accepted", name)
		}
	}
}

func TestSafeName(t *testing.T) {
	good := []string{"app", "app.cdx.json", "sbom/app.cdx.json", ".nself-ci/sbom/x.json", "a b"}
	bad := []string{"", ".", "..", "a/../b", "a//b", "/a", "a/", "a/./b", "C:x", "a\\b", "a\x00b", strings.Repeat("x", 256)}
	for _, n := range good {
		if !safeName(n) {
			t.Errorf("%q refused", n)
		}
	}
	for _, n := range bad {
		if safeName(n) {
			t.Errorf("%q accepted", n)
		}
	}
}

func TestRefusalIsSanitized(t *testing.T) {
	body := func(code, reason, msg string) []byte {
		b, _ := json.Marshal(map[string]string{"code": code, "reason": reason, "message": msg})
		return b
	}
	cases := []struct {
		name     string
		status   int
		raw      []byte
		wantCode string
	}{
		{"known E687", 403, body(roles.CodeNotAuthorized, "role", "the caller may not do this"), roles.CodeNotAuthorized},
		{"known E713", 404, body(records.CodeNotFound, "not_found", "no such artifact"), records.CodeNotFound},
		{"unknown code", 403, body("E999", "x", "pwned"), "E664"},
		{"not json", 502, []byte("<html>bad gateway</html>"), "E664"},
		{"escape codes", 403, body("E664", "role", "\x1b[2Jevil\r\nline"), "E664"},
		{"long message", 403, body("E664", "role", strings.Repeat("m", 5000)), "E664"},
		{"long multibyte message", 403, body("E664", "role", strings.Repeat("é", 5000)), "E664"},
	}
	for _, c := range cases {
		s := &script{status: map[string]int{InputsPath(tRun): c.status}, body: map[string][]byte{InputsPath(tRun): c.raw}}
		_, _, err := Fetch(context.Background(), s, tRun, t.TempDir())
		var re *Error
		if err == nil || !asErr(err, &re) || re.ErrCode != c.wantCode {
			t.Fatalf("%s: %v", c.name, err)
		}
		if strings.ContainsAny(re.Msg, "\x1b\r\n") || len(re.Msg) > 700 || strings.Contains(re.Msg, "pwned") || !utf8.ValidString(re.Msg) {
			t.Fatalf("%s: message not sanitized: %q", c.name, re.Msg)
		}
	}
}

func TestFetchRefusesBadRunID(t *testing.T) {
	for _, id := range []string{"", "..", "a/b", "a b", "a\x00", strings.Repeat("x", 201)} {
		if _, _, err := Fetch(context.Background(), &script{}, id, t.TempDir()); err == nil {
			t.Fatalf("run id %q accepted", id)
		}
	}
}
