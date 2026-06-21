package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

func (d *Dispatcher) StartProcessing() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	log.Printf("[nself-webhooks] delivery processor started (concurrent=%d, maxAttempts=%d)", d.maxConcurrency, d.maxAttempts)

	for {
		select {
		case <-d.stopCh:
			log.Printf("[nself-webhooks] delivery processor stopped")
			return
		case <-ticker.C:
			d.processPending()
		}
	}
}

// StopProcessing signals the background processor to stop.
func (d *Dispatcher) StopProcessing() {
	close(d.stopCh)
}

func (d *Dispatcher) processPending() {
	// Determine how many slots are free in the semaphore so we don't over-fetch.
	available := d.maxConcurrency - len(d.sem)
	if available <= 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deliveries, err := GetPendingDeliveries(ctx, d.pool, available)
	if err != nil {
		log.Printf("[nself-webhooks] error fetching pending deliveries: %v", err)
		return
	}
	if len(deliveries) == 0 {
		return
	}

	log.Printf("[nself-webhooks] processing %d pending deliveries", len(deliveries))

	var wg sync.WaitGroup
	for i := range deliveries {
		// Acquire semaphore slot BEFORE launching the goroutine.
		// This guarantees at most maxConcurrency goroutines hold DB connections.
		d.sem <- struct{}{}

		// Warn at 80% capacity (len*10 >= cap*8) to enable proactive scaling.
		if len(d.sem)*10 >= d.maxConcurrency*8 {
			log.Printf("[nself-webhooks] WARN webhook dispatcher semaphore at 80%% capacity (used=%d cap=%d)", len(d.sem), d.maxConcurrency)
		}

		wg.Add(1)
		go func(del Delivery) {
			defer wg.Done()
			defer func() { <-d.sem }() // release slot on completion
			d.processDelivery(del)
		}(deliveries[i])
	}
	wg.Wait()
}

func (d *Dispatcher) processDelivery(del Delivery) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(d.requestTimeoutMs+5000)*time.Millisecond)
	defer cancel()

	endpoint, err := GetEndpoint(ctx, d.pool, del.EndpointID)
	if err != nil {
		errMsg := "endpoint not found"
		_ = UpdateDeliveryStatus(ctx, d.pool, del.ID, "failed", nil, nil, nil, &errMsg, nil)
		return
	}

	if !endpoint.Enabled {
		errMsg := "endpoint is disabled"
		_ = UpdateDeliveryStatus(ctx, d.pool, del.ID, "failed", nil, nil, nil, &errMsg, nil)
		return
	}

	// Mark as delivering (increment attempt).
	_ = UpdateDeliveryStatus(ctx, d.pool, del.ID, "delivering", nil, nil, nil, nil, nil)

	// Build HTTP request.
	payloadBytes := []byte(del.Payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		errMsg := "failed to build request: " + err.Error()
		d.handleFailure(ctx, del, endpoint, nil, &errMsg)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "nself-webhooks/1.0")
	req.Header.Set("X-Webhook-Signature", del.Signature)
	req.Header.Set("X-Webhook-Event-Type", del.EventType)
	req.Header.Set("X-Webhook-Delivery-Id", del.ID)
	req.Header.Set("X-Webhook-Attempt", strconv.Itoa(del.AttemptCount+1))

	// Apply custom headers from endpoint.
	var customHeaders map[string]string
	if err := json.Unmarshal([]byte(endpoint.Headers), &customHeaders); err == nil {
		for k, v := range customHeaders {
			req.Header.Set(k, v)
		}
	}

	start := time.Now()
	resp, err := d.client.Do(req)
	elapsed := int(time.Since(start).Milliseconds())

	if err != nil {
		errMsg := err.Error()
		d.handleFailure(ctx, del, endpoint, &elapsed, &errMsg)
		return
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	respBody := string(bodyBytes)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Success.
		status := resp.StatusCode
		_ = UpdateDeliveryStatus(ctx, d.pool, del.ID, "delivered", &status, &respBody, &elapsed, nil, nil)
		_ = RecordEndpointSuccess(ctx, d.pool, endpoint.ID)
		return
	}

	// HTTP error.
	errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(respBody, 200))
	d.handleFailure(ctx, del, endpoint, &elapsed, &errMsg)
}

func (d *Dispatcher) handleFailure(ctx context.Context, del Delivery, endpoint *Endpoint, responseTimeMs *int, errMsg *string) {
	newAttemptCount := del.AttemptCount + 1
	shouldRetry := newAttemptCount < del.MaxAttempts

	if shouldRetry {
		delayIdx := newAttemptCount - 1
		if delayIdx >= len(d.retryDelays) {
			delayIdx = len(d.retryDelays) - 1
		}
		nextRetry := time.Now().Add(d.retryDelays[delayIdx])
		_ = UpdateDeliveryStatus(ctx, d.pool, del.ID, "pending", nil, nil, responseTimeMs, errMsg, &nextRetry)
	} else {
		// Max attempts exhausted: dead letter.
		_ = MarkDeliveryDeadLetter(ctx, d.pool, del.ID, responseTimeMs, errMsg)
		log.Printf("[nself-webhooks] delivery %s moved to dead letter queue after %d attempts", del.ID, newAttemptCount)
		// Fire DLQ alert event (T04). No PII — payload is hashed, not embedded.
		emitDLQEvent(del, newAttemptCount, errMsg)
	}

	_ = RecordEndpointFailure(ctx, d.pool, endpoint.ID, d.autoDisableThresh)
}

