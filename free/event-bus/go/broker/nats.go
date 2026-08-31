package broker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	natsStreamName   = "NSELF"
	natsConsumerBase = "nself-consumer"
)

// NATSBroker is a JetStream-backed Broker adapter.
type NATSBroker struct {
	nc        *nats.Conn
	js        jetstream.JetStream
	stream    jetstream.Stream
	retentionMS int64
	maxBytes    int64
}

// NewNATSBroker connects to an external NATS server and creates the NSELF stream.
func NewNATSBroker(url string, retentionMS, maxBytes int64) (*NATSBroker, error) {
	nc, err := nats.Connect(url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(60),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connect %q: %w", url, err)
	}
	return newNATSBroker(nc, retentionMS, maxBytes)
}

func newNATSBroker(nc *nats.Conn, retentionMS, maxBytes int64) (*NATSBroker, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream context: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       natsStreamName,
		Subjects:   []string{"nself.>"},
		MaxAge:     time.Duration(retentionMS) * time.Millisecond,
		MaxBytes:   maxBytes,
		Storage:    jetstream.FileStorage,
		Retention:  jetstream.LimitsPolicy,
		Replicas:   1,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create/update stream %q: %w", natsStreamName, err)
	}

	return &NATSBroker{nc: nc, js: js, stream: stream, retentionMS: retentionMS, maxBytes: maxBytes}, nil
}

// Publish delivers payload to subject via JetStream.
func (b *NATSBroker) Publish(ctx context.Context, subject string, payload []byte) error {
	if err := validateSubject(subject); err != nil {
		return err
	}
	_, err := b.js.Publish(ctx, subject, payload)
	if err != nil {
		return fmt.Errorf("nats publish %q: %w", subject, err)
	}
	return nil
}

// Subscribe creates a durable push-consumer for the subject pattern.
func (b *NATSBroker) Subscribe(subject string, handler func(msg Message) error) (Subscription, error) {
	if err := validateSubject(subject); err != nil {
		return nil, err
	}
	// Derive a stable durable name from the subject.
	durableName := natsConsumerBase + "-" + sanitizeDurable(subject)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	consumer, err := b.stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       durableName,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverAllPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("create consumer for %q: %w", subject, err)
	}

	cc, err := consumer.Consume(func(msg jetstream.Msg) {
		m := Message{
			Subject: msg.Subject(),
			Payload: msg.Data(),
			Ack:     func() error { return msg.Ack() },
			Nak:     func() error { return msg.Nak() },
		}
		if herr := handler(m); herr != nil {
			_ = msg.Nak()
		}
	})
	if err != nil {
		return nil, fmt.Errorf("start consume for %q: %w", subject, err)
	}

	return &natsSubscription{cc: cc}, nil
}

func (b *NATSBroker) Type() string { return "nats" }

func (b *NATSBroker) Drain() error {
	return b.nc.Drain()
}

// natsSubscription wraps a JetStream ConsumeContext.
type natsSubscription struct {
	cc jetstream.ConsumeContext
}

func (s *natsSubscription) Drain() error {
	s.cc.Stop()
	return nil
}

// validateSubject rejects subjects that do not start with "nself.".
func validateSubject(subject string) error {
	if !strings.HasPrefix(subject, "nself.") {
		return fmt.Errorf("invalid subject %q: must begin with \"nself.\"", subject)
	}
	return nil
}

// sanitizeDurable converts a subject into a legal JetStream durable name.
func sanitizeDurable(subject string) string {
	return strings.NewReplacer(".", "-", "*", "star", ">", "gt").Replace(subject)
}
