package pipeline

// Purpose: validate one attempt and build the create request (Epic D11).
// Inputs:  providers.Attempt.
// Outputs: a request (allowlisted values, gzip+base64 spec, digest) and the body of the create call in
//          either transport.
// Constraints: anything outside the allowlists is E708 before a request is made; the encoded spec is at
//          most MaxSpecChars (the documented limit of a pipeline variable value); variables carry one
//          NSELF_CI_SPEC value, inputs carry the same spec as spec_0..spec_9 chunks of ChunkChars
//          because a string inside a pipeline input must be under 1 KB (GitLab inputs documentation,
//          read 2026-10-19); every other value has the same name in both transports.

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

// MaxSpecChars is the largest encoded spec a dispatch carries: GitLab documents a maximum of 10,000
// characters for a CI/CD variable value (https://docs.gitlab.com/ci/variables/, read 2026-10-19).
const MaxSpecChars = 10000

// The two ways the values reach the pipeline.
const (
	ModeInputs    = "inputs"
	ModeVariables = "variables"
)

// RemediationVariables is the E702 reason when a project refuses pipeline variables.
const RemediationVariables = "pipeline.create_refused: the project refuses pipeline variables (set its variables role to developer or use GitLab 17.10+ pipeline inputs), or workflow:rules filter the pipeline"

var (
	revisionRE = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	fetchRefRE = regexp.MustCompile(`^refs/merge-requests/[0-9]+/head$`)
	attemptRE  = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,256}$`)
	refRE      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,99}$`)
)

// request is one validated dispatch.
type request struct {
	project                                              api.Project
	attempt, revision, fetchRef, tag, compat, timeoutMin string
	spec, digest                                         string
	n                                                    int
	trusted                                              bool
}

func trustedClass(t string) bool { return t == "owner" || t == "internal" || t == "collaborator" }

func timeoutOption(ms int64) (string, bool) {
	if ms <= 0 {
		return "60", true
	}
	minutes := (ms + 59999) / 60000
	for _, o := range gitlabtmpl.Timeouts {
		if int64(o) >= minutes {
			return strconv.Itoa(o), true
		}
	}
	return "", false
}

func encodeSpec(spec []byte) (string, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return "", api.Refuse("spec.encode")
	}
	if _, err = zw.Write(spec); err != nil || zw.Close() != nil {
		return "", api.Refuse("spec.encode")
	}
	enc := base64.StdEncoding.EncodeToString(buf.Bytes())
	if len(enc) > MaxSpecChars {
		return "", api.Refuse("spec.too_large")
	}
	return enc, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func plan(a providers.Attempt) (request, error) {
	r := request{attempt: a.ID, revision: strings.ToLower(a.Revision), fetchRef: a.FetchRef, tag: a.Label, compat: a.CompatMode, n: a.N}
	project, ok := api.ParseProject(a.Repo)
	switch {
	case !ok:
		return r, api.Refuse("repo")
	case !attemptRE.MatchString(a.ID):
		return r, api.Refuse("attempt")
	case a.N < 1:
		return r, api.Refuse("attempt.number")
	case !revisionRE.MatchString(r.revision):
		return r, api.Refuse("revision")
	case a.FetchRef != "" && !fetchRefRE.MatchString(a.FetchRef):
		return r, api.Refuse("fetch_ref")
	}
	r.project = project
	tags, err := gitlabtmpl.TagNames()
	if err != nil {
		return r, api.Misconfig("runner_tags")
	}
	if r.tag == "" {
		r.tag = tags[0]
	}
	if !contains(tags, r.tag) {
		return r, api.Refuse("label.not_allowed")
	}
	if r.compat == "" {
		r.compat = "v1.5"
	}
	if !contains(gitlabtmpl.CompatModes, r.compat) {
		return r, api.Refuse("compat_mode")
	}
	if r.timeoutMin, ok = timeoutOption(a.TimeoutMs); !ok {
		return r, api.Refuse("timeout.over_360")
	}
	sum := sha256.Sum256(a.Spec)
	r.digest = "sha256:" + hex.EncodeToString(sum[:])
	if a.SpecDigest != "" && a.SpecDigest != r.digest {
		return r, api.Refuse("spec.digest")
	}
	if r.spec, err = encodeSpec(a.Spec); err != nil {
		return r, err
	}
	r.trusted = trustedClass(a.Trust)
	return r, nil
}

type variable struct {
	Key          string `json:"key"`
	Value        string `json:"value"`
	VariableType string `json:"variable_type"`
}

type createBody struct {
	Ref       string            `json:"ref"`
	Inputs    map[string]string `json:"inputs,omitempty"`
	Variables []variable        `json:"variables,omitempty"`
}

// values lists (variable name, input name, value) in the order the template declares them.
func (r request) values(nonce string) [][3]string {
	vs := [][3]string{
		{gitlabtmpl.VarSpec, "spec", r.spec}, {gitlabtmpl.VarDigest, "spec_digest", r.digest}, {gitlabtmpl.VarRevision, "revision", r.revision},
		{gitlabtmpl.VarAttempt, "attempt", r.attempt}, {gitlabtmpl.VarNonce, "nonce", nonce}, {gitlabtmpl.VarCompat, "compat", r.compat},
		{gitlabtmpl.VarRunnerTag, "runner_tag", r.tag}, {gitlabtmpl.VarTimeout, "timeout_minutes", r.timeoutMin},
	}
	if r.fetchRef != "" {
		vs = append(vs, [3]string{gitlabtmpl.VarFetchRef, "fetch_ref", r.fetchRef})
	}
	return vs
}

func (r request) body(ref, nonce, mode string) createBody {
	b := createBody{Ref: ref}
	if mode == ModeInputs {
		b.Inputs = map[string]string{}
		for i := 0; i*gitlabtmpl.ChunkChars < len(r.spec); i++ {
			b.Inputs["spec_"+strconv.Itoa(i)] = r.spec[i*gitlabtmpl.ChunkChars : min(len(r.spec), (i+1)*gitlabtmpl.ChunkChars)]
		}
	}
	for _, v := range r.values(nonce) {
		switch {
		case mode == ModeInputs && v[1] == "spec":
			// carried by the spec_N chunks above
		case mode == ModeInputs:
			b.Inputs[v[1]] = v[2]
		default:
			b.Variables = append(b.Variables, variable{Key: v[0], Value: v[2], VariableType: "env_var"})
		}
	}
	return b
}
