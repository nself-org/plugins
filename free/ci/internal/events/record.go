package events

// Purpose: one ci.events record without its envelope (D-1061): the value the
// run engine stores in event_index and the follow command replays through an
// Emitter, which adds the envelope, the seq and the timestamp.
// Inputs:  a type, the job and attempt ids and the data map of a record.
// Outputs: Ref (the stored body) and ParseRecord (its inverse).
// Constraints: the body is JSON of job, attempt and data only; type is stored
// beside it, never inside.

import (
	"encoding/json"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Record is the content of one event.
type Record struct {
	Type    model.EventType
	Job     string
	Attempt string
	Data    map[string]any
}

type refBody struct {
	Job     string         `json:"job,omitempty"`
	Attempt string         `json:"attempt,omitempty"`
	Data    map[string]any `json:"data"`
}

// Ref encodes the record's body for storage.
func (r Record) Ref() string {
	d := r.Data
	if d == nil {
		d = map[string]any{}
	}
	b, _ := json.Marshal(refBody{Job: r.Job, Attempt: r.Attempt, Data: d}) // maps of JSON values always marshal
	return string(b)
}

// ParseRecord decodes a stored type and body.
func ParseRecord(typ, ref string) (Record, error) {
	t := model.EventType(typ)
	if !t.Valid() {
		return Record{}, fmt.Errorf("unknown event type %q", typ)
	}
	var b refBody
	if err := json.Unmarshal([]byte(ref), &b); err != nil {
		return Record{}, fmt.Errorf("event body: %w", err)
	}
	if b.Data == nil {
		b.Data = map[string]any{}
	}
	return Record{Type: t, Job: b.Job, Attempt: b.Attempt, Data: b.Data}, nil
}
