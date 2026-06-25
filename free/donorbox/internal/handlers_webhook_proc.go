package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

// --- Webhook processing ------------------------------------------------------

func processDonationWebhook(ctx context.Context, db *DB, payload map[string]interface{}) error {
	// Re-marshal and decode as APIDonation
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	var donation APIDonation
	if err := json.Unmarshal(data, &donation); err != nil {
		return fmt.Errorf("decode donation: %w", err)
	}

	record := mapAPIDonation(donation)
	_, err = db.UpsertDonations(ctx, []Donation{record})
	if err != nil {
		return err
	}

	log.Printf("[nself-donorbox] donation created via webhook: id=%d amount=%s", donation.ID, donation.Amount)
	return nil
}

