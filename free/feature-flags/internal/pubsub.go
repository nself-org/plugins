package internal

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

// PubSub broadcasts flag-invalidation events over Redis Pub/Sub so that SDK
// consumers can flush their 60s LRU cache within <5 seconds of a kill/disable.
//
// Channel: feature_flags:invalidate:<key>
//
// If Redis is unreachable, the broadcast silently no-ops. Consumers fall back
// to their 60s cache TTL. No message-ordering guarantees.
type PubSub struct {
	redisAddr string
}

// NewPubSub creates a PubSub broadcaster pointing at the given Redis address.
func NewPubSub(redisAddr string) *PubSub {
	return &PubSub{redisAddr: redisAddr}
}

// Broadcast sends a PUBLISH command to Redis for the given flag key.
// Callers must not block on the result — errors are logged but not returned
// so that flag state mutations succeed even when Redis is unavailable.
func (p *PubSub) Broadcast(ctx context.Context, key string) {
	channel := fmt.Sprintf("feature_flags:invalidate:%s", key)
	go func() {
		if err := p.publish(channel, key); err != nil {
			log.Printf("feature-flags pubsub: broadcast error (key=%s): %v (SDK consumers will rely on 60s TTL)", key, err)
		}
	}()
}

// publish opens a short-lived TCP connection to Redis and sends a PUBLISH command.
// This avoids holding a persistent connection in the plugin process.
func (p *PubSub) publish(channel, message string) error {
	conn, err := net.DialTimeout("tcp", p.redisAddr, 2*time.Second)
	if err != nil {
		return fmt.Errorf("dial redis: %w", err)
	}
	defer conn.Close()

	// RESP inline PUBLISH command
	cmd := fmt.Sprintf(
		"*3\r\n$7\r\nPUBLISH\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n",
		len(channel), channel,
		len(message), message,
	)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return fmt.Errorf("write publish: %w", err)
	}

	// Read the integer reply (number of subscribers). We don't need the value.
	buf := make([]byte, 32)
	if _, err := conn.Read(buf); err != nil {
		return fmt.Errorf("read reply: %w", err)
	}

	reply := strings.TrimSpace(string(buf))
	if strings.HasPrefix(reply, "-") {
		return fmt.Errorf("redis error: %s", reply)
	}
	return nil
}
