package signing

// Purpose: the wire format between the coordinator and the signer process
// (Epic CACHE D12): one request, one response, nothing else.
// Inputs:  the request bytes (stdin of `artifacts signer --stdio`).
// Outputs: Request, Response and the DSSE envelopes the signer returns.
// Constraints: a request is at most MaxRequestBytes and strict JSON (no duplicate
// or case-variant keys, nothing after the document); it names a run and at
// most MaxItems payloads of the two allowed types; unknown fields are refused
// so a request cannot smuggle an extra instruction; the response carries
// envelopes (or one error object) and never key material.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"

	"github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
)

// MaxRequestBytes bounds a request (and the payload bytes inside it).
const MaxRequestBytes = 4 << 20

// MaxItems bounds the payloads of one request.
const MaxItems = 8

// PayloadStatement is the DSSE payloadType of an in-toto statement.
const PayloadStatement = "application/vnd.in-toto+json"

// Item is one payload to sign.
type Item struct {
	PayloadType string `json:"payload_type"`
	PayloadB64  string `json:"payload_b64"`
}

// Request is what the coordinator sends.
type Request struct {
	RunID string `json:"run_id"`
	Items []Item `json:"items"`
}

// Response is what the signer prints: envelopes in request order, or Error.
type Response struct {
	Envelopes []signing.Envelope `json:"envelopes,omitempty"`
	Error     *ResponseError     `json:"error,omitempty"`
}

// ResponseError is the signer's refusal as the coordinator reads it back.
type ResponseError struct {
	Code    string `json:"code"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// allowedType reports whether the signer signs this payload type at all.
func allowedType(t string) bool { return t == manifest.PayloadType || t == PayloadStatement }

// DecodeRequest reads one request from r, refusing anything over the cap or
// outside the narrow shape. Payloads are returned decoded, in item order.
func DecodeRequest(r io.Reader) (Request, [][]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, MaxRequestBytes+1))
	if err != nil {
		return Request{}, nil, Failf(ReasonPayloadRejected, "the request could not be read")
	}
	if len(raw) > MaxRequestBytes {
		return Request{}, nil, Failf(ReasonPayloadRejected, "the request is larger than %d bytes", MaxRequestBytes)
	}
	if err := strictJSON(raw, "request"); err != nil {
		return Request{}, nil, err
	}
	var req Request
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return Request{}, nil, Failf(ReasonPayloadRejected, "the request is not one {run_id, items} document")
	}
	if req.RunID == "" || len(req.Items) == 0 || len(req.Items) > MaxItems {
		return Request{}, nil, Failf(ReasonPayloadRejected, "the request needs a run id and 1 to %d items", MaxItems)
	}
	payloads := make([][]byte, len(req.Items))
	for i, it := range req.Items {
		if !allowedType(it.PayloadType) {
			return Request{}, nil, Failf(ReasonPayloadRejected, "item %d: payload type %q is not signed here", i, echo(it.PayloadType))
		}
		p, err := base64.StdEncoding.Strict().DecodeString(it.PayloadB64)
		if err != nil || len(p) == 0 {
			return Request{}, nil, Failf(ReasonPayloadRejected, "item %d: the payload is not standard base64 or is empty", i)
		}
		payloads[i] = p
	}
	return req, payloads, nil
}
