package broker

import (
	"context"
	"fmt"
	"strings"

	"github.com/segmentio/kafka-go"
)

// RedpandaBroker wraps kafka-go for Redpanda (Kafka-protocol compatible).
// Redpanda does not support JetStream durable consumers; at-least-once delivery
// is achieved via a committed consumer-group offset.
type RedpandaBroker struct {
	brokers  []string
	writers  map[string]*kafka.Writer
	readers  []*kafka.Reader
}

// NewRedpandaBroker creates a Redpanda adapter.
func NewRedpandaBroker(brokers []string) (*RedpandaBroker, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("redpanda: at least one broker address required")
	}
	return &RedpandaBroker{
		brokers: brokers,
		writers: make(map[string]*kafka.Writer),
	}, nil
}

func (b *RedpandaBroker) writer(topic string) *kafka.Writer {
	if w, ok := b.writers[topic]; ok {
		return w
	}
	w := &kafka.Writer{
		Addr:     kafka.TCP(b.brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	b.writers[topic] = w
	return w
}

// Publish sends payload to the Kafka topic derived from the subject.
func (b *RedpandaBroker) Publish(ctx context.Context, subject string, payload []byte) error {
	if err := validateSubject(subject); err != nil {
		return err
	}
	topic := subjectToTopic(subject)
	w := b.writer(topic)
	return w.WriteMessages(ctx, kafka.Message{Value: payload})
}

// Subscribe creates a Kafka consumer-group reader for the topic pattern.
func (b *RedpandaBroker) Subscribe(subject string, handler func(msg Message) error) (Subscription, error) {
	if err := validateSubject(subject); err != nil {
		return nil, err
	}
	topic := subjectToTopic(subject)
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  b.brokers,
		Topic:    topic,
		GroupID:  "nself-event-bus",
		MinBytes: 1,
		MaxBytes: 1e6,
	})
	b.readers = append(b.readers, r)

	go func() {
		for {
			km, err := r.FetchMessage(context.Background())
			if err != nil {
				return
			}
			m := Message{
				Subject: subject,
				Payload: km.Value,
				Ack:     func() error { return r.CommitMessages(context.Background(), km) },
				Nak:     func() error { return nil }, // Kafka redelivers on group restart
			}
			_ = handler(m)
		}
	}()

	return &kafkaSubscription{r: r}, nil
}

func (b *RedpandaBroker) Type() string { return "redpanda" }

func (b *RedpandaBroker) Drain() error {
	var last error
	for _, w := range b.writers {
		if err := w.Close(); err != nil {
			last = err
		}
	}
	for _, r := range b.readers {
		if err := r.Close(); err != nil {
			last = err
		}
	}
	return last
}

// subjectToTopic converts "nself.cdc.np_users.insert" → "nself-cdc-np_users-insert".
func subjectToTopic(subject string) string {
	return strings.ReplaceAll(subject, ".", "-")
}
