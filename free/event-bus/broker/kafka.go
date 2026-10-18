package broker

import (
	"context"
	"fmt"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
)

// KafkaBroker wraps kafka-go with optional SASL/PLAIN auth.
type KafkaBroker struct {
	RedpandaBroker
	mechanism sasl.Mechanism
}

// NewKafkaBroker creates a Kafka adapter with optional SASL credentials.
func NewKafkaBroker(brokers []string, saslUser, saslPass string) (*KafkaBroker, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("kafka: at least one broker address required")
	}

	base := &RedpandaBroker{
		brokers: brokers,
		writers: make(map[string]*kafka.Writer),
	}
	kb := &KafkaBroker{RedpandaBroker: *base}

	if saslUser != "" {
		kb.mechanism = plain.Mechanism{
			Username: saslUser,
			Password: saslPass,
		}
		// Rebuild writers with SASL transport.
		kb.writers = make(map[string]*kafka.Writer)
	}

	return kb, nil
}

func (b *KafkaBroker) writer(topic string) *kafka.Writer {
	if w, ok := b.writers[topic]; ok {
		return w
	}
	w := &kafka.Writer{
		Addr:     kafka.TCP(b.brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	if b.mechanism != nil {
		w.Transport = &kafka.Transport{
			SASL: b.mechanism,
		}
	}
	b.writers[topic] = w
	return w
}

func (b *KafkaBroker) Publish(ctx context.Context, subject string, payload []byte) error {
	if err := validateSubject(subject); err != nil {
		return err
	}
	topic := subjectToTopic(subject)
	w := b.writer(topic)
	return w.WriteMessages(ctx, kafka.Message{Value: payload})
}

func (b *KafkaBroker) Type() string { return "kafka" }

// kafkaSubscription wraps a *kafka.Reader.
type kafkaSubscription struct {
	r *kafka.Reader
}

func (s *kafkaSubscription) Drain() error {
	return s.r.Close()
}
