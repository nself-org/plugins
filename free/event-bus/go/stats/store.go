// Package stats tracks per-subject publish/subscribe metrics in memory and
// periodically flushes them to np_event_bus_stats in Postgres.
package stats

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// SubjectRow mirrors np_event_bus_stats.
type SubjectRow struct {
	Subject       string
	MsgCount      int64
	ConsumerCount int
}

// ConsumerRow describes an active subscription.
type ConsumerRow struct {
	Subject string `json:"subject"`
	Durable string `json:"durable"`
	Pending int64  `json:"pending_msgs"`
}

// Store is the in-memory stats registry.
type Store interface {
	RecordPublish(subject string)
	RegisterConsumer(subject, durable string)
	DeregisterConsumer(subject, durable string)
	Subjects() []SubjectRow
	Consumers() []ConsumerRow
	Purge(ctx context.Context, subject string) error
}

// MemStore is a thread-safe in-memory implementation.
type MemStore struct {
	mu        sync.RWMutex
	subjects  map[string]*subjectState
	consumers map[string]ConsumerRow
}

type subjectState struct {
	msgCount      atomic.Int64
	consumerCount atomic.Int32
}

// New creates an empty MemStore.
func New() *MemStore {
	return &MemStore{
		subjects:  make(map[string]*subjectState),
		consumers: make(map[string]ConsumerRow),
	}
}

func (s *MemStore) getOrCreate(subject string) *subjectState {
	s.mu.RLock()
	st, ok := s.subjects[subject]
	s.mu.RUnlock()
	if ok {
		return st
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok = s.subjects[subject]; ok {
		return st
	}
	st = &subjectState{}
	s.subjects[subject] = st
	return st
}

// RecordPublish increments the message count for subject.
func (s *MemStore) RecordPublish(subject string) {
	s.getOrCreate(subject).msgCount.Add(1)
}

// RegisterConsumer adds a consumer entry and increments the consumer count.
func (s *MemStore) RegisterConsumer(subject, durable string) {
	st := s.getOrCreate(subject)
	st.consumerCount.Add(1)
	s.mu.Lock()
	s.consumers[durable] = ConsumerRow{Subject: subject, Durable: durable}
	s.mu.Unlock()
}

// DeregisterConsumer removes a consumer and decrements the consumer count.
func (s *MemStore) DeregisterConsumer(subject, durable string) {
	s.mu.Lock()
	delete(s.consumers, durable)
	s.mu.Unlock()
	if st := s.getOrCreate(subject); st.consumerCount.Load() > 0 {
		st.consumerCount.Add(-1)
	}
}

// Subjects returns a snapshot of all tracked subjects.
func (s *MemStore) Subjects() []SubjectRow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := make([]SubjectRow, 0, len(s.subjects))
	for subj, st := range s.subjects {
		rows = append(rows, SubjectRow{
			Subject:       subj,
			MsgCount:      st.msgCount.Load(),
			ConsumerCount: int(st.consumerCount.Load()),
		})
	}
	return rows
}

// Consumers returns a snapshot of active subscriptions.
func (s *MemStore) Consumers() []ConsumerRow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := make([]ConsumerRow, 0, len(s.consumers))
	for _, c := range s.consumers {
		rows = append(rows, c)
	}
	return rows
}

// Purge resets the stats for a subject (simulates broker-side purge for stats layer).
func (s *MemStore) Purge(_ context.Context, subject string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.subjects[subject]; ok {
		st.msgCount.Store(0)
	}
	return nil
}

// FlushTicker returns a channel that fires at the given interval; callers use
// it to trigger periodic Postgres flushes.
func FlushTicker(d time.Duration) <-chan time.Time {
	return time.NewTicker(d).C
}
