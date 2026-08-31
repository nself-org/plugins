package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQBroker publishes events to a RabbitMQ topic exchange.
// Exchange name: <prefix>.events (topic type).
// Routing key:   <prefix>.<table>.<operation_lowercase>
type RabbitMQBroker struct {
	conn     *amqp.Connection
	ch       *amqp.Channel
	exchange string
	prefix   string
}

// NewRabbitMQBroker dials RabbitMQ and declares the topic exchange.
func NewRabbitMQBroker(urls []string, topicPrefix string) (*RabbitMQBroker, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("rabbitmq: no URLs provided")
	}
	conn, err := amqp.Dial(urls[0])
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: channel: %w", err)
	}
	exchange := topicPrefix + ".events"
	if err := ch.ExchangeDeclare(
		exchange, "topic", true, false, false, false, nil,
	); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: exchange declare: %w", err)
	}
	return &RabbitMQBroker{conn: conn, ch: ch, exchange: exchange, prefix: topicPrefix}, nil
}

// Publish sends a batch of events to the topic exchange.
func (b *RabbitMQBroker) Publish(ctx context.Context, events []*Event) error {
	if len(events) == 0 {
		return nil
	}
	start := time.Now()
	for _, ev := range events {
		routingKey := topicName(b.prefix, ev.Table, ev.Operation)
		body, err := json.Marshal(ev.Payload)
		if err != nil {
			slog.Warn("rabbitmq: marshal", "err", err)
			continue
		}
		if err := b.ch.PublishWithContext(ctx, b.exchange, routingKey, false, false,
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Body:         body,
				Timestamp:    ev.OccurredAt,
			},
		); err != nil {
			return fmt.Errorf("rabbitmq: publish: %w", err)
		}
	}
	slog.Debug("rabbitmq: published batch", "count", len(events),
		"elapsed_ms", time.Since(start).Milliseconds())
	return nil
}

// Close closes the channel and connection.
func (b *RabbitMQBroker) Close() {
	_ = b.ch.Close()
	_ = b.conn.Close()
}

// Type returns the broker type label.
func (b *RabbitMQBroker) Type() string { return "rabbitmq" }
