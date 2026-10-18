// Package broker provides adapters for Kafka/Redpanda and RabbitMQ.
package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	kafka "github.com/segmentio/kafka-go"
)

// KafkaBroker publishes events to Kafka or Redpanda (same protocol).
type KafkaBroker struct {
	writer *kafka.Writer
	prefix string
}

// NewKafkaBroker builds a kafka.Writer using the provided broker URLs.
func NewKafkaBroker(urls []string, topicPrefix string) *KafkaBroker {
	w := &kafka.Writer{
		Addr:         kafka.TCP(urls...),
		Balancer:     &kafka.LeastBytes{},
		BatchTimeout: 5 * time.Millisecond,
		Async:        false,
	}
	return &KafkaBroker{writer: w, prefix: topicPrefix}
}

// Publish sends a batch of events to their respective topics.
// Topic name: <prefix>.<table>.<operation_lowercase>
func (b *KafkaBroker) Publish(ctx context.Context, events []*Event) error {
	if len(events) == 0 {
		return nil
	}
	start := time.Now()
	msgs := make([]kafka.Message, 0, len(events))
	for _, ev := range events {
		topic := topicName(b.prefix, ev.Table, ev.Operation)
		val, err := json.Marshal(ev.Payload)
		if err != nil {
			slog.Warn("kafka: marshal", "err", err)
			continue
		}
		msgs = append(msgs, kafka.Message{
			Topic: topic,
			Key:   []byte(ev.Table),
			Value: val,
		})
	}

	if err := b.writer.WriteMessages(ctx, msgs...); err != nil {
		return fmt.Errorf("kafka: write %d messages: %w", len(msgs), err)
	}
	slog.Debug("kafka: published batch", "count", len(msgs),
		"elapsed_ms", time.Since(start).Milliseconds())
	return nil
}

// Close shuts down the writer.
func (b *KafkaBroker) Close() { _ = b.writer.Close() }

// Type returns the broker type label.
func (b *KafkaBroker) Type() string { return "kafka" }
