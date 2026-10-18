package broker

import (
	"context"
	"sync"
)

// MockBroker is an in-memory broker for unit tests.
type MockBroker struct {
	mu       sync.Mutex
	Published []*Event
	FailNext  bool // if true, Publish returns an error once
}

// Publish records events or returns an error when FailNext is set.
func (m *MockBroker) Publish(_ context.Context, events []*Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailNext {
		m.FailNext = false
		return &brokerDownError{}
	}
	m.Published = append(m.Published, events...)
	return nil
}

// Reset clears published events.
func (m *MockBroker) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Published = m.Published[:0]
}

// Count returns how many events have been published.
func (m *MockBroker) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Published)
}

// Close is a no-op.
func (m *MockBroker) Close() {}

// Type returns "mock".
func (m *MockBroker) Type() string { return "mock" }

type brokerDownError struct{}

func (e *brokerDownError) Error() string { return "mock broker: simulated failure" }
