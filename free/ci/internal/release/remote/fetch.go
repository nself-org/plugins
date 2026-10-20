package remote

// Purpose: the operator's half of a TEAM release (Epic CACHE D15): fetch the release inputs of a
// run from the coordinator, download every listed blob, re-hash each one while it is written to a
// private blob store, and hand back a Snapshot plus that store for the signer's checks.
// Inputs:  a Fetcher (coordclient.Client: pinned root, user certificate or scoped token), a run id
// and a private staging directory.
// Outputs: *Snapshot and the staged blob store; an *Error for a refusal, a checksum mismatch or a
// malformed answer.
// Constraints: the coordinator is untrusted for content: a blob whose digest or size differs from its
// record is E719 checksum and nothing is signed; every field that becomes a path or an id is validated
// here before it can reach the publication; a blob is read at most size+1 bytes; only the digests the
// inputs list are requested; the staging directory is the caller's to remove; nothing is sent to the
// coordinator but GET requests (no key, no signature request, no secret).

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/release/verify"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/signing"
)

// Fetcher is the authenticated GET of the coordinator client.
type Fetcher interface {
	GetRaw(ctx context.Context, path string) (*http.Response, error)
}

// Fetch downloads and re-hashes the release inputs of run into stage.
func Fetch(ctx context.Context, f Fetcher, run, stage string) (*Snapshot, *cas.Store, error) {
	if !ValidRunID(run) {
		return nil, nil, fail(signing.CodeSigning, signing.ReasonPayloadRejected, "run id is not a plain name")
	}
	in, err := fetchInputs(ctx, f, run)
	if err != nil {
		return nil, nil, err
	}
	blobs, err := cas.New(stage)
	if err != nil {
		return nil, nil, fail(signing.CodeSigning, signing.ReasonSignerFailed, "the staging blob store could not be opened")
	}
	for _, r := range uniqueBlobs(in.Records) {
		if err = fetchBlob(ctx, f, blobs, in, r); err != nil {
			return nil, nil, err
		}
	}
	return NewSnapshot(in), blobs, nil
}

func fetchInputs(ctx context.Context, f Fetcher, run string) (Inputs, error) {
	resp, err := f.GetRaw(ctx, InputsPath(run))
	if err != nil {
		return Inputs{}, err
	}
	defer clientDrain(resp)
	if resp.StatusCode != http.StatusOK {
		return Inputs{}, refusal(resp)
	}
	var in Inputs
	dec := json.NewDecoder(io.LimitReader(resp.Body, MaxInputsBytes+1))
	if dec.Decode(&in) != nil || dec.InputOffset() > MaxInputsBytes {
		return Inputs{}, fail(clientapi.CodeUserAPI, clientapi.ReasonTransport, "the coordinator's release inputs are not a valid document")
	}
	if err = validate(in, run); err != nil {
		return Inputs{}, err
	}
	return in, nil
}

func fetchBlob(ctx context.Context, f Fetcher, blobs *cas.Store, in Inputs, rec records.Record) error {
	resp, err := f.GetRaw(ctx, BlobPath(in.Run, rec.SHA256))
	if err != nil {
		return err
	}
	defer clientDrain(resp)
	if resp.StatusCode != http.StatusOK {
		return refusal(resp)
	}
	digest, n, err := blobs.Put(ctx, cas.ArtifactRealm(in.Pipeline.Project), io.LimitReader(resp.Body, rec.Size+1))
	if err != nil {
		return fail(signing.CodeSigning, signing.ReasonSignerFailed, "a release blob could not be staged")
	}
	if digest != rec.SHA256 || n != rec.Size {
		return fail(verify.Code, verify.ReasonChecksum, "release blob %s changed on the coordinator: its bytes do not match the recorded digest; nothing was signed", rec.SHA256[:12])
	}
	return nil
}

// uniqueBlobs lists one record per digest, sorted, so a digest is downloaded once.
func uniqueBlobs(recs []records.Record) []records.Record {
	seen := map[string]bool{}
	var out []records.Record
	for _, r := range recs {
		if !seen[r.SHA256] {
			seen[r.SHA256] = true
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SHA256 < out[j].SHA256 })
	return out
}

// validate checks the shape of a document from an untrusted coordinator before anything uses it.
func validate(in Inputs, run string) error {
	bad := func(what string) error {
		return fail(clientapi.CodeUserAPI, clientapi.ReasonTransport, "the coordinator's release inputs are malformed: %s", what)
	}
	p := in.Pipeline
	switch {
	case in.Schema != Schema || in.Run != run || p.ID != run:
		return bad("wrong schema or run")
	case p.Project == "" || len(p.Project) > 1024 || p.Revision == "" || len(in.Evidence) == 0 || len(in.Records) >= MaxRecords || len(in.Jobs) > MaxRecords || len(in.Attempts) > 4*MaxRecords:
		return bad("missing or oversized field")
	}
	for _, id := range append([]string{p.Revision, p.InputDigest}, ids(in)...) {
		if !plain(id) {
			return bad("an id is not plain text")
		}
	}
	for _, r := range in.Records {
		if r.Run != run || r.Project != p.Project || !shaRe.MatchString(r.SHA256) || r.Size < 0 || r.Size > MaxBlobBytes || !safeName(r.Name) || !plain(r.Attempt) {
			return bad("a record is not a record of this run")
		}
	}
	return nil
}

func ids(in Inputs) []string {
	var out []string
	for _, j := range in.Jobs {
		out = append(out, j.ID, j.Name, j.Kind)
	}
	for _, a := range in.Attempts {
		out = append(out, a.ID, a.Job, a.State, a.Coordinator)
	}
	return out
}

// plain reports a short string without control characters.
func plain(s string) bool {
	if len(s) > 1024 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == 0xFFFD {
			return false
		}
	}
	return true
}

// safeName accepts the relative slash paths a release names its files by: no escape from the
// release directory, no absolute or drive path, no backslash.
func safeName(n string) bool {
	if n == "" || len(n) > 255 || !plain(n) || strings.ContainsAny(n, "\\:") || path.IsAbs(n) || path.Clean(n) != n {
		return false
	}
	for _, seg := range strings.Split(n, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// refusal reads the coordinator's error body. Only a known code is believed; the message is cut and
// stripped of control characters, so a hostile coordinator cannot write into the operator's terminal.
func refusal(resp *http.Response) error {
	var b clientapi.Body
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&b) != nil {
		return fail(clientapi.CodeUserAPI, clientapi.ReasonTransport, "the coordinator answered %d", resp.StatusCode)
	}
	switch b.Code {
	case clientapi.CodeUserAPI, roles.CodeNotAuthorized, records.CodeNotFound:
	default:
		return fail(clientapi.CodeUserAPI, clientapi.ReasonTransport, "the coordinator answered %d", resp.StatusCode)
	}
	reason := b.Reason
	if !plain(reason) || len(reason) > 64 {
		reason = clientapi.ReasonTransport
	}
	msg := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, b.Message)
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return fail(b.Code, reason, "%s", msg)
}

func clientDrain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}
