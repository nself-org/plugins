package clientapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/detach"
)

// EventJournal is optional for backends predating durable coordinator events.
type EventJournal interface {
	RunEvents(context.Context, string, int64) ([]json.RawMessage, error)
}

func (b *StoreBackend) RunEvents(ctx context.Context, run string, from int64) ([]json.RawMessage, error) {
	return b.Store.RunEventsFrom(ctx, run, from)
}

// journalEvents uses stored bytes and sequences; legacy terminal runs with no
// records fall back to synthesis. Every poll retains the stream's authorization check.
func (s *Server) journalEvents(ctx context.Context, w http.ResponseWriter, r *http.Request, it RunItem, from int64) bool {
	journal, ok := s.cfg.Backend.(EventJournal)
	if !ok {
		return false
	}
	records, err := journal.RunEvents(ctx, it.ID, from)
	if err != nil {
		s.fail(w, err)
		return true
	}
	if len(records) == 0 && detach.Terminal(it.Status) {
		return false
	}
	out := startNDJSON(w)
	next := max(from, int64(1))
	ended := time.Time{}
	for {
		for _, raw := range records {
			rec, ok := detach.ParseLine(raw)
			if !ok {
				return true
			}
			if rec.Seq < next && !rec.Final() {
				continue
			}
			if _, err := out.Write(append(append([]byte(nil), rec.Raw...), '\n')); err != nil {
				return true
			}
			if rec.Final() {
				return true
			}
			next = rec.Seq + 1
		}
		if detach.Terminal(it.Status) {
			if ended.IsZero() {
				ended = time.Now()
			}
			if time.Since(ended) >= detach.IdleGrace {
				return true
			}
		}
		select {
		case <-ctx.Done():
			return true
		case <-time.After(s.cfg.Poll):
		}
		fresh, ok, err := s.cfg.Backend.Get(ctx, it.ID)
		if err != nil || !ok || !s.stillAllowed(r, fresh) {
			return true
		}
		it = fresh
		records, err = journal.RunEvents(ctx, it.ID, next)
		if err != nil {
			return true
		}
	}
}
