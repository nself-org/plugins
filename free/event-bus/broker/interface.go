// Package broker defines the Broker interface and Message type used by all
// nSelf event-bus adapters (NATS JetStream, Redpanda, Kafka).
package broker

import "context"

// Message is a unit of work received from a subscription.
type Message struct {
	Subject string
	Payload []byte
	// Ack acknowledges the message so the broker will not redeliver it.
	// Must be called once the handler has finished processing.
	Ack func() error
	// Nak signals the broker that processing failed; triggers redelivery.
	Nak func() error
}

// Subscription represents an active consumer created by Subscribe.
type Subscription interface {
	// Drain waits for in-flight messages to complete then closes the subscription.
	Drain() error
}

// Broker is the interface every adapter must satisfy.
type Broker interface {
	// Publish delivers payload to the given subject.
	// Returns an error if the broker is unreachable; never panics.
	Publish(ctx context.Context, subject string, payload []byte) error

	// Subscribe registers a durable consumer for the subject pattern.
	// Wildcards follow NATS conventions: * (single token), > (all remaining).
	// JetStream and Kafka-protocol adapters both surface at-least-once delivery.
	Subscribe(subject string, handler func(msg Message) error) (Subscription, error)

	// Type returns a human-readable broker type string ("nats", "redpanda", "kafka").
	Type() string

	// Drain closes all subscriptions gracefully then shuts down the connection.
	Drain() error
}
