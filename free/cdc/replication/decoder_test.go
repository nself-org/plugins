package replication_test

import (
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/nself-org/nself-cdc/replication"
)

// buildRelationMsg builds a minimal pgoutput Relation message for testing.
func buildRelationMsg(relID uint32, ns, name string, cols []string) []byte {
	buf := []byte{'R'}
	buf = binary.BigEndian.AppendUint32(buf, relID)
	buf = append(buf, []byte(ns)...)
	buf = append(buf, 0)
	buf = append(buf, []byte(name)...)
	buf = append(buf, 0)
	buf = append(buf, 'd') // replica identity DEFAULT
	buf = binary.BigEndian.AppendUint16(buf, uint16(len(cols)))
	for _, col := range cols {
		buf = append(buf, 0) // flags
		buf = append(buf, []byte(col)...)
		buf = append(buf, 0)
		buf = binary.BigEndian.AppendUint32(buf, 25) // text OID
		buf = binary.BigEndian.AppendUint32(buf, uint32(0xFFFFFFFF))
	}
	return buf
}

// buildInsertMsg builds a minimal pgoutput Insert message.
func buildInsertMsg(relID uint32, values []string) []byte {
	buf := []byte{'I'}
	buf = binary.BigEndian.AppendUint32(buf, relID)
	buf = append(buf, 'N')
	buf = appendTuple(buf, values)
	return buf
}

func appendTuple(buf []byte, values []string) []byte {
	buf = binary.BigEndian.AppendUint16(buf, uint16(len(values)))
	for _, v := range values {
		buf = append(buf, 't')
		buf = binary.BigEndian.AppendUint32(buf, uint32(len(v)))
		buf = append(buf, []byte(v)...)
	}
	return buf
}

func TestPgoutputDecoder_Insert(t *testing.T) {
	dec := replication.NewDecoder()

	// Register relation.
	relMsg := buildRelationMsg(1, "public", "np_users", []string{"id", "email"})
	events, err := dec.Decode(relMsg)
	if err != nil {
		t.Fatalf("decode relation: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("relation message should produce 0 events, got %d", len(events))
	}

	// Insert message.
	insMsg := buildInsertMsg(1, []string{"42", "alice@example.com"})
	events, err = dec.Decode(insMsg)
	if err != nil {
		t.Fatalf("decode insert: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Operation != "INSERT" {
		t.Errorf("expected INSERT, got %q", ev.Operation)
	}

	var env map[string]interface{}
	if err := json.Unmarshal(ev.Payload, &env); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if env["op"] != "c" {
		t.Errorf("expected op=c, got %v", env["op"])
	}
	if env["before"] != nil {
		t.Errorf("expected before=null for insert, got %v", env["before"])
	}
	after, ok := env["after"].(map[string]interface{})
	if !ok {
		t.Fatalf("after field missing or wrong type")
	}
	if after["id"] != "42" {
		t.Errorf("expected id=42, got %v", after["id"])
	}
	if after["email"] != "alice@example.com" {
		t.Errorf("expected email=alice@example.com, got %v", after["email"])
	}
}

func TestPgoutputDecoder_Delete(t *testing.T) {
	dec := replication.NewDecoder()

	// Register relation.
	relMsg := buildRelationMsg(2, "public", "np_orders", []string{"id", "status"})
	_, _ = dec.Decode(relMsg)

	// Delete message: 'D' + relID(4) + 'O' + tuple(old row).
	buf := []byte{'D'}
	buf = binary.BigEndian.AppendUint32(buf, 2)
	buf = append(buf, 'O')
	buf = appendTuple(buf, []string{"99", "cancelled"})

	events, err := dec.Decode(buf)
	if err != nil {
		t.Fatalf("decode delete: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Operation != "DELETE" {
		t.Errorf("expected DELETE, got %q", ev.Operation)
	}

	var env map[string]interface{}
	_ = json.Unmarshal(ev.Payload, &env)
	if env["op"] != "d" {
		t.Errorf("expected op=d, got %v", env["op"])
	}
	if env["after"] != nil {
		t.Errorf("expected after=null for delete")
	}
}
