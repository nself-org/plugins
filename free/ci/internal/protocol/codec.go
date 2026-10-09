package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
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
	poison error
}

func NewDecoder(r io.Reader, policy CarrierPolicy) *Decoder {
	return &Decoder{r: bufio.NewReaderSize(r, 4096), policy: policy}
}

// NewResumedDecoder starts one direction after a validated resume marker.
func NewResumedDecoder(r io.Reader, policy CarrierPolicy, lastSeq, claimedLastSeq uint64) (*Decoder, error) {
	if err := checkResumeSeq(lastSeq, claimedLastSeq); err != nil {
		return nil, err
	}
	d := NewDecoder(r, policy)
	d.seq = lastSeq
	return d, nil
}

func checkResumeSeq(lastSeq, claimedLastSeq uint64) error {
	if lastSeq != claimedLastSeq || lastSeq == ^uint64(0) {
		return invalid("resume_seq")
	}
	return nil
}

func (d *Decoder) Decode() (Message, error) {
	if d.poison != nil {
		return Message{}, d.poison
	}
	m, err := d.decode()
	if err != nil && !errors.Is(err, io.EOF) {
		d.poison = err
	}
	return m, err
}

func (d *Decoder) decode() (Message, error) {
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
	if err := requiredFields(raw, reflect.TypeOf(body).Elem()); err != nil {
		return nil, invalid("required_body")
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

type Encoder struct {
	w   io.Writer
	seq uint64
}

func NewEncoder(w io.Writer) *Encoder { return &Encoder{w: w} }

// NewResumedEncoder continues one direction after a validated resume marker.
func NewResumedEncoder(w io.Writer, lastSeq, claimedLastSeq uint64) (*Encoder, error) {
	if err := checkResumeSeq(lastSeq, claimedLastSeq); err != nil {
		return nil, err
	}
	e := NewEncoder(w)
	e.seq = lastSeq
	return e, nil
}
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
