package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const MaxFrame = 1 << 20

// Error carries the public protocol code without including secret frame data.
type Error struct {
	Code   string
	Reason string
}

func (e *Error) Error() string    { return e.Code + ": " + e.Reason }
func invalid(reason string) error { return &Error{CodeInvalid, reason} }

// Message is one frame. Body is a typed body after decoding.
type Message struct {
	V       int     `json:"v"`
	Type    string  `json:"type"`
	Seq     uint64  `json:"seq"`
	Ack     *uint64 `json:"ack,omitempty"`
	Epoch   *uint64 `json:"epoch,omitempty"`
	LeaseID string  `json:"lease_id,omitempty"`
	Body    any     `json:"body"`
}

type wireMessage struct {
	V       int             `json:"v"`
	Type    string          `json:"type"`
	Seq     uint64          `json:"seq"`
	Ack     *uint64         `json:"ack,omitempty"`
	Epoch   *uint64         `json:"epoch,omitempty"`
	LeaseID string          `json:"lease_id,omitempty"`
	Body    json.RawMessage `json:"body"`
}

// Decoder validates one direction's sequence and the caller's carrier policy.
type Decoder struct {
	r      *bufio.Reader
	seq    uint64
	policy CarrierPolicy
}

func NewDecoder(r io.Reader, policy CarrierPolicy) *Decoder {
	return &Decoder{r: bufio.NewReaderSize(r, 4096), policy: policy}
}

func (d *Decoder) Decode() (Message, error) {
	line := make([]byte, 0, 4096)
	for {
		frag, err := d.r.ReadSlice('\n')
		if len(line)+len(frag) > MaxFrame {
			return Message{}, invalid("frame_too_large")
		}
		if need := len(line) + len(frag); need > cap(line) {
			next := cap(line) * 2
			if next < need {
				next = need
			}
			if next > MaxFrame {
				next = MaxFrame
			}
			grown := make([]byte, len(line), next)
			copy(grown, line)
			line = grown
		}
		line = append(line, frag...)
		if err == nil {
			break
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) && len(line) == 0 {
			return Message{}, io.EOF
		}
		return Message{}, invalid("unterminated_frame")
	}
	if len(line) == 0 || line[len(line)-1] != '\n' {
		return Message{}, invalid("unterminated_frame")
	}
	var w wireMessage
	if err := strict(line, &w); err != nil {
		return Message{}, invalid("decode")
	}
	if w.V != 1 || w.Seq == 0 || w.Seq != d.seq+1 || len(w.Body) == 0 {
		return Message{}, invalid("version_seq_or_body")
	}
	body, err := decodeBody(w.Type, w.Body, d.policy)
	if err != nil {
		return Message{}, err
	}
	if err := validate(w.Type, body, w.Epoch, w.LeaseID); err != nil {
		return Message{}, err
	}
	d.seq = w.Seq
	return Message{w.V, w.Type, w.Seq, w.Ack, w.Epoch, w.LeaseID, body}, nil
}

func strict(raw []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func decodeBody(kind string, raw json.RawMessage, p CarrierPolicy) (any, error) {
	var body any
	switch kind {
	case "hello":
		body = &Hello{}
	case "welcome":
		body = &Welcome{}
	case "reject":
		body = &Reject{}
	case "heartbeat":
		body = &Heartbeat{}
	case "lease":
		body = &Lease{}
	case "secrets":
		body = &Secrets{}
	case "chunk":
		body = &Chunk{}
	case "log":
		body = &Log{}
	case "artifact-ref":
		body = &ArtifactRef{}
	case "result":
		body = &Result{}
	case "cancel":
		body = &Cancel{}
	case "drain":
		body = &Drain{}
	case "upgrade":
		body = &Upgrade{}
	case "ack":
		body = &Ack{}
	case "digest":
		body = &Digest{}
	case "offset":
		body = &Offset{}
	case "resume":
		body = &Resume{}
	case "cache-ref":
		body = &CacheRef{}
	default:
		return nil, invalid("unknown_type")
	}
	if err := strict(raw, body); err != nil {
		return nil, invalid("decode_body")
	}
	if s, ok := body.(*Secrets); ok && len(s.Values) > 0 {
		if p.Carrier != "A" || !p.Personal || !p.OperatorOwned {
			return nil, invalid("secrets_carrier")
		}
		for _, value := range s.Values {
			if !validSecretRef(value.Ref) {
				return nil, invalid("secrets_class")
			}
		}
	}
	return body, nil
}

func validate(kind string, body any, epoch *uint64, leaseID string) error {
	switch v := body.(type) {
	case *Chunk:
		if v.Stream != "checkout" && v.Stream != "artifact" && v.Stream != "cache" {
			return invalid("chunk_stream")
		}
		if (v.Stream == "artifact" || v.Stream == "cache") && v.Digest == "" {
			return invalid("chunk_digest")
		}
	case *Digest:
		if !validStream(v.Stream) || v.SHA256 == "" || v.Size < 0 {
			return invalid("digest")
		}
	case *Offset:
		if !validStream(v.Stream) || v.SHA256 == "" || v.Offset < 0 {
			return invalid("offset")
		}
	case *Lease:
		if v.Epoch == 0 || v.LeaseID == "" || (v.CompatMode != Compat14 && v.CompatMode != Compat15) {
			return invalid("lease")
		}
		if epoch != nil && *epoch != v.Epoch || leaseID != "" && leaseID != v.LeaseID {
			return invalid("fencing_fields")
		}
		if v.Cache != nil && v.Cache.Access != "rw" && v.Cache.Access != "ro" && v.Cache.Access != "none" {
			return invalid("cache_access")
		}
		if v.RunnerCredential != nil && v.RunnerCredential.Kind != "github-jitconfig" && v.RunnerCredential.Kind != "gitlab-runner-token" {
			return invalid("runner_credential")
		}
	case *Result:
		if v.Epoch == 0 || v.LeaseID == "" {
			return invalid("result_epoch")
		}
		if epoch != nil && *epoch != v.Epoch || leaseID != "" && leaseID != v.LeaseID {
			return invalid("fencing_fields")
		}
	case *CacheRef:
		if v.LeaseID == "" || v.Spec == "" || v.Key == "" {
			return invalid("cache_ref")
		}
	}
	if epoch != nil && *epoch == 0 {
		return invalid("epoch")
	}
	if kind == "lease" || kind == "result" {
		if epoch == nil || leaseID == "" {
			return invalid("fencing_fields")
		}
	}
	return nil
}
func validStream(s string) bool { return s == "checkout" || s == "artifact" || s == "cache" }
func validSecretRef(ref string) bool {
	return strings.HasPrefix(ref, "project/") && len(ref) > len("project/") || strings.HasPrefix(ref, "environment/") && len(ref) > len("environment/")
}

// Encoder writes canonical JSON keys using encoding/json's deterministic map order.
type Encoder struct {
	w   io.Writer
	seq uint64
}

func NewEncoder(w io.Writer) *Encoder { return &Encoder{w: w} }
func (e *Encoder) Encode(m Message) error {
	if m.V != 1 || m.Seq != e.seq+1 {
		return invalid("version_seq")
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	var sorted any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&sorted); err != nil {
		return err
	}
	data, err = json.Marshal(sorted)
	if err != nil {
		return err
	}
	if len(data)+1 > MaxFrame {
		return invalid("frame_too_large")
	}
	data = append(data, '\n')
	n, err := e.w.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	e.seq = m.Seq
	return nil
}
