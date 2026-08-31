package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Event is the unit of work sent to a broker. Defined here to avoid import cycles.
type Event struct {
	Table      string
	Operation  string
	LSN        string
	Payload    json.RawMessage
	OccurredAt time.Time
}

// Broker is the interface all broker adapters satisfy.
type Broker interface {
	Publish(ctx context.Context, events []*Event) error
	Close()
	Type() string
}

// New constructs the correct broker adapter from the type string.
func New(brokerType string, urls []string) (Broker, error) {
	switch strings.ToLower(brokerType) {
	case "kafka", "redpanda":
		return NewKafkaBroker(urls, "nself"), nil
	case "rabbitmq":
		return NewRabbitMQBroker(urls, "nself")
	default:
		return nil, fmt.Errorf("unknown broker type: %q (must be kafka|redpanda|rabbitmq)", brokerType)
	}
}

// topicName produces the standard topic/routing-key string.
// Format: <prefix>.<table>.<operation_lowercase>
// Example: nself.np_users.insert
func topicName(prefix, table, operation string) string {
	return prefix + "." + table + "." + strings.ToLower(operation)
}
