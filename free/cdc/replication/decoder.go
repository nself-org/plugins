package replication

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nself-org/nself-cdc/broker"
)

// DebeziumEnvelope is the wire payload written to np_cdc_events and sent to broker.
type DebeziumEnvelope struct {
	Op     string          `json:"op"` // c=insert, u=update, d=delete
	Before json.RawMessage `json:"before"`
	After  json.RawMessage `json:"after"`
	TsMs   int64           `json:"ts_ms"`
	Source struct {
		Table string `json:"table"`
		LSN   string `json:"lsn"`
	} `json:"source"`
}

// Decoder decodes raw pgoutput wire bytes into broker.Events.
type Decoder struct {
	relations map[uint32]*relationMsg
}

// NewDecoder returns a ready Decoder.
func NewDecoder() *Decoder {
	return &Decoder{relations: make(map[uint32]*relationMsg)}
}

type relationMsg struct {
	RelationID uint32
	Namespace  string
	Name       string
	Columns    []columnDef
}

type columnDef struct {
	Name     string
	DataType uint32
	TypeMod  int32
}

// Decode processes a single pgoutput CopyData buffer.
// It may return zero, one, or multiple events per buffer.
func (d *Decoder) Decode(buf []byte) ([]*broker.Event, error) {
	if len(buf) == 0 {
		return nil, nil
	}
	msgType := buf[0]

	switch msgType {
	case 'R': // Relation
		rel, err := decodeRelation(buf[1:])
		if err != nil {
			return nil, fmt.Errorf("relation: %w", err)
		}
		d.relations[rel.RelationID] = rel
		return nil, nil

	case 'I': // Insert
		return d.decodeInsert(buf[1:])

	case 'U': // Update
		return d.decodeUpdate(buf[1:])

	case 'D': // Delete
		return d.decodeDelete(buf[1:])

	case 'B', 'C', 'O', 'T': // Begin, Commit, Origin, Type
		return nil, nil

	default:
		return nil, nil
	}
}

func (d *Decoder) decodeInsert(buf []byte) ([]*broker.Event, error) {
	if len(buf) < 6 {
		return nil, fmt.Errorf("insert: buffer too short")
	}
	relID := binary.BigEndian.Uint32(buf[0:4])
	rel, ok := d.relations[relID]
	if !ok {
		return nil, fmt.Errorf("insert: unknown relation %d", relID)
	}
	// buf[4] == 'N' (new tuple tag)
	after, err := decodeTuple(buf[5:], rel)
	if err != nil {
		return nil, fmt.Errorf("insert after: %w", err)
	}
	env := DebeziumEnvelope{
		Op:    "c",
		After: after,
		TsMs:  time.Now().UnixMilli(),
	}
	env.Source.Table = rel.Name
	payload, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	ev := &broker.Event{
		Table:      rel.Name,
		Operation:  "INSERT",
		Payload:    json.RawMessage(payload),
		OccurredAt: time.Now(),
	}
	return []*broker.Event{ev}, nil
}

func (d *Decoder) decodeUpdate(buf []byte) ([]*broker.Event, error) {
	if len(buf) < 6 {
		return nil, fmt.Errorf("update: buffer too short")
	}
	relID := binary.BigEndian.Uint32(buf[0:4])
	rel, ok := d.relations[relID]
	if !ok {
		return nil, fmt.Errorf("update: unknown relation %d", relID)
	}

	var before, after json.RawMessage
	pos := 4
	for pos < len(buf) {
		tag := buf[pos]
		pos++
		switch tag {
		case 'O', 'K': // old tuple
			var err error
			before, err = decodeTuple(buf[pos:], rel)
			if err != nil {
				return nil, fmt.Errorf("update before: %w", err)
			}
			pos += tupleSize(buf[pos:])
		case 'N': // new tuple
			var err error
			after, err = decodeTuple(buf[pos:], rel)
			if err != nil {
				return nil, fmt.Errorf("update after: %w", err)
			}
			pos += tupleSize(buf[pos:])
		default:
			pos = len(buf)
		}
	}

	env := DebeziumEnvelope{Op: "u", Before: before, After: after, TsMs: time.Now().UnixMilli()}
	env.Source.Table = rel.Name
	payload, _ := json.Marshal(env)
	return []*broker.Event{{Table: rel.Name, Operation: "UPDATE", Payload: json.RawMessage(payload), OccurredAt: time.Now()}}, nil
}

func (d *Decoder) decodeDelete(buf []byte) ([]*broker.Event, error) {
	if len(buf) < 6 {
		return nil, fmt.Errorf("delete: buffer too short")
	}
	relID := binary.BigEndian.Uint32(buf[0:4])
	rel, ok := d.relations[relID]
	if !ok {
		return nil, fmt.Errorf("delete: unknown relation %d", relID)
	}
	// buf[4] tag: 'O' or 'K'
	before, err := decodeTuple(buf[5:], rel)
	if err != nil {
		return nil, fmt.Errorf("delete before: %w", err)
	}
	env := DebeziumEnvelope{Op: "d", Before: before, TsMs: time.Now().UnixMilli()}
	env.Source.Table = rel.Name
	payload, _ := json.Marshal(env)
	return []*broker.Event{{Table: rel.Name, Operation: "DELETE", Payload: json.RawMessage(payload), OccurredAt: time.Now()}}, nil
}

// decodeTuple converts a pgoutput TupleData into a JSON object keyed by column name.
func decodeTuple(buf []byte, rel *relationMsg) (json.RawMessage, error) {
	if len(buf) < 2 {
		return json.RawMessage("null"), nil
	}
	ncols := int(binary.BigEndian.Uint16(buf[0:2]))
	pos := 2
	row := make(map[string]interface{}, ncols)

	for i := 0; i < ncols && pos < len(buf); i++ {
		colType := buf[pos]
		pos++
		switch colType {
		case 'n': // null
			if i < len(rel.Columns) {
				row[rel.Columns[i].Name] = nil
			}
		case 't': // text
			if pos+4 > len(buf) {
				return nil, fmt.Errorf("tuple: text length truncated")
			}
			slen := int(binary.BigEndian.Uint32(buf[pos : pos+4]))
			pos += 4
			if pos+slen > len(buf) {
				return nil, fmt.Errorf("tuple: text data truncated")
			}
			val := string(buf[pos : pos+slen])
			pos += slen
			if i < len(rel.Columns) {
				row[rel.Columns[i].Name] = val
			}
		case 'u': // unchanged toast
			if i < len(rel.Columns) {
				row[rel.Columns[i].Name] = nil
			}
		}
	}

	return json.Marshal(row)
}

func decodeRelation(buf []byte) (*relationMsg, error) {
	if len(buf) < 6 {
		return nil, fmt.Errorf("relation: buffer too short")
	}
	rel := &relationMsg{
		RelationID: binary.BigEndian.Uint32(buf[0:4]),
	}
	pos := 4
	end := indexOf(buf, pos, 0)
	rel.Namespace = string(buf[pos:end])
	pos = end + 1
	end = indexOf(buf, pos, 0)
	rel.Name = string(buf[pos:end])
	pos = end + 1
	pos++ // replica identity
	if pos+2 > len(buf) {
		return rel, nil
	}
	ncols := int(binary.BigEndian.Uint16(buf[pos : pos+2]))
	pos += 2
	for i := 0; i < ncols && pos < len(buf); i++ {
		pos++ // flags
		end = indexOf(buf, pos, 0)
		name := string(buf[pos:end])
		pos = end + 1
		if pos+8 > len(buf) {
			break
		}
		dataType := binary.BigEndian.Uint32(buf[pos : pos+4])
		typeMod := int32(binary.BigEndian.Uint32(buf[pos+4 : pos+8]))
		pos += 8
		rel.Columns = append(rel.Columns, columnDef{Name: name, DataType: dataType, TypeMod: typeMod})
	}
	return rel, nil
}

func indexOf(buf []byte, start int, b byte) int {
	for i := start; i < len(buf); i++ {
		if buf[i] == b {
			return i
		}
	}
	return len(buf)
}

func tupleSize(buf []byte) int {
	if len(buf) < 2 {
		return len(buf)
	}
	ncols := int(binary.BigEndian.Uint16(buf[0:2]))
	pos := 2
	for i := 0; i < ncols && pos < len(buf); i++ {
		if pos >= len(buf) {
			break
		}
		switch buf[pos] {
		case 't':
			pos++
			if pos+4 > len(buf) {
				return len(buf)
			}
			slen := int(binary.BigEndian.Uint32(buf[pos : pos+4]))
			pos += 4 + slen
		default:
			pos++
		}
	}
	return pos
}
