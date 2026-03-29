package internal

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

var startedAt = time.Now()

// RegisterRoutes mounts all donorbox endpoints on the given router.
func RegisterRoutes(r chi.Router, db *DB, client *DonorboxClient, webhookSecret string) {
	// Health probes
	r.Get("/ready", handleReady(db))
	r.Get("/live", handleLive())
	r.Get("/status", handleStatus(db))

	// Sync operations (require API client)
	r.Post("/sync", handleSync(db, client))
	r.Post("/reconcile", handleReconcile(db, client))

	// Webhook
	r.Post("/webhooks/donorbox", handleWebhook(db, webhookSecret))

	// API queries
	r.Get("/api/campaigns", handleListCampaigns(db))
	r.Get("/api/donors", handleListDonors(db))
	r.Get("/api/donations", handleListDonations(db))
	r.Get("/api/plans", handleListPlans(db))
	r.Get("/api/stats", handleGetStats(db))
	r.Get("/api/events", handleListWebhookEvents(db))
}

// --- Health probes -----------------------------------------------------------

func handleReady(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		if err := db.Pool().Ping(ctx); err != nil {
			sdk.Respond(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready", "error": err.Error()})
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

func handleLive() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"status": "alive",
			"uptime": time.Since(startedAt).String(),
		})
	}
}

func handleStatus(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		stats, err := db.GetStats(ctx)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"plugin":  "donorbox",
			"version": "1.0.0",
			"uptime":  time.Since(startedAt).String(),
			"stats":   stats,
		})
	}
}

// --- Sync operations ---------------------------------------------------------

func handleSync(db *DB, client *DonorboxClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			sdk.Respond(w, http.StatusServiceUnavailable, map[string]string{
				"error": "Donorbox API credentials not configured. Set DONORBOX_EMAIL and DONORBOX_API_KEY.",
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()

		result := runSync(ctx, db, client)
		status := http.StatusOK
		if !result.Success {
			status = http.StatusPartialContent
		}
		sdk.Respond(w, status, result)
	}
}

func handleReconcile(db *DB, client *DonorboxClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			sdk.Respond(w, http.StatusServiceUnavailable, map[string]string{
				"error": "Donorbox API credentials not configured. Set DONORBOX_EMAIL and DONORBOX_API_KEY.",
			})
			return
		}

		lookbackDays := 7
		if v := r.URL.Query().Get("lookback_days"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				lookbackDays = n
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		result := runReconcile(ctx, db, client, lookbackDays)
		status := http.StatusOK
		if !result.Success {
			status = http.StatusPartialContent
		}
		sdk.Respond(w, status, result)
	}
}

// --- Webhook -----------------------------------------------------------------

func handleWebhook(db *DB, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
			return
		}

		// Verify HMAC-SHA256 signature if secret is configured
		if secret != "" {
			sig := r.Header.Get("X-Donorbox-Signature")
			if sig == "" {
				sig = r.Header.Get("X-Hub-Signature-256")
			}
			if !verifyHMAC(body, sig, secret) {
				sdk.Respond(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
				return
			}
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
			return
		}

		eventType, _ := payload["event_type"].(string)
		if eventType == "" {
			eventType = "donation.created"
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		// Generate unique event ID
		randBytes := make([]byte, 4)
		rand.Read(randBytes)
		eventID := fmt.Sprintf("donorbox_%d_%s", time.Now().UnixMilli(), hex.EncodeToString(randBytes))

		// Store raw event
		if err := db.InsertWebhookEvent(ctx, eventID, eventType, body); err != nil {
			log.Printf("[nself-donorbox] webhook store error: %v", err)
		}

		// Process event
		var processErr *string
		switch eventType {
		case "donation.created":
			if err := processDonationWebhook(ctx, db, payload); err != nil {
				errStr := err.Error()
				processErr = &errStr
				log.Printf("[nself-donorbox] webhook process error: %v", err)
			}
		default:
			log.Printf("[nself-donorbox] unhandled webhook event type: %s", eventType)
		}

		if markErr := db.MarkEventProcessed(ctx, eventID, processErr); markErr != nil {
			log.Printf("[nself-donorbox] mark processed error: %v", markErr)
		}

		sdk.Respond(w, http.StatusOK, map[string]string{"status": "received", "event_id": eventID})
	}
}

// --- API query handlers ------------------------------------------------------

func handleListCampaigns(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := parsePagination(r)

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		campaigns, err := db.QueryCampaigns(ctx, limit, offset)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if campaigns == nil {
			campaigns = []Campaign{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"campaigns": campaigns,
			"limit":     limit,
			"offset":    offset,
		})
	}
}

func handleListDonors(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := parsePagination(r)

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		donors, err := db.QueryDonors(ctx, limit, offset)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if donors == nil {
			donors = []Donor{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"donors": donors,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func handleListDonations(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := parsePagination(r)
		status := r.URL.Query().Get("status")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		donations, err := db.QueryDonations(ctx, status, limit, offset)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if donations == nil {
			donations = []Donation{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"donations": donations,
			"limit":     limit,
			"offset":    offset,
		})
	}
}

func handleListPlans(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := parsePagination(r)
		status := r.URL.Query().Get("status")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		plans, err := db.QueryPlans(ctx, status, limit, offset)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plans == nil {
			plans = []Plan{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"plans":  plans,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func handleGetStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		stats, err := db.GetStats(ctx)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		sdk.Respond(w, http.StatusOK, stats)
	}
}

func handleListWebhookEvents(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		events, err := db.QueryWebhookEvents(ctx, limit)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if events == nil {
			events = []WebhookEvent{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"events": events,
		})
	}
}

// --- Sync logic --------------------------------------------------------------

func runSync(ctx context.Context, db *DB, client *DonorboxClient) *SyncResult {
	started := time.Now()
	stats := SyncStats{}
	var errors []string

	type syncTask struct {
		name string
		fn   func() (int, error)
	}

	tasks := []syncTask{
		{"Campaigns", func() (int, error) { return syncCampaigns(ctx, db, client) }},
		{"Donors", func() (int, error) { return syncDonors(ctx, db, client) }},
		{"Donations", func() (int, error) { return syncDonations(ctx, db, client, "") }},
		{"Plans", func() (int, error) { return syncPlans(ctx, db, client) }},
		{"Events", func() (int, error) { return syncEvents(ctx, db, client) }},
		{"Tickets", func() (int, error) { return syncTickets(ctx, db, client) }},
	}

	for _, t := range tasks {
		log.Printf("[nself-donorbox] syncing %s...", t.name)
		count, err := t.fn()
		if err != nil {
			msg := fmt.Sprintf("%s: %v", t.name, err)
			errors = append(errors, msg)
			log.Printf("[nself-donorbox] sync error: %s", msg)
		} else {
			log.Printf("[nself-donorbox] %s: %d records", t.name, count)
		}

		switch t.name {
		case "Campaigns":
			stats.Campaigns = count
		case "Donors":
			stats.Donors = count
		case "Donations":
			stats.Donations = count
		case "Plans":
			stats.Plans = count
		case "Events":
			stats.Events = count
		case "Tickets":
			stats.Tickets = count
		}
	}

	now := time.Now()
	stats.LastSyncedAt = &now

	return &SyncResult{
		Success:  len(errors) == 0,
		Stats:    stats,
		Errors:   errors,
		Duration: time.Since(started).Milliseconds(),
	}
}

func runReconcile(ctx context.Context, db *DB, client *DonorboxClient, lookbackDays int) *SyncResult {
	started := time.Now()
	stats := SyncStats{}
	var errors []string

	since := time.Now().AddDate(0, 0, -lookbackDays)
	dateFrom := since.Format("2006-01-02")

	log.Printf("[nself-donorbox] reconciling last %d days (from %s)", lookbackDays, dateFrom)

	// Reconcile donations with date filter
	log.Println("[nself-donorbox] reconciling Donations...")
	count, err := syncDonations(ctx, db, client, dateFrom)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Donations: %v", err))
	}
	stats.Donations = count

	// Re-sync donors to pick up totals
	log.Println("[nself-donorbox] reconciling Donors...")
	count, err = syncDonors(ctx, db, client)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Donors: %v", err))
	}
	stats.Donors = count

	now := time.Now()
	stats.LastSyncedAt = &now

	return &SyncResult{
		Success:  len(errors) == 0,
		Stats:    stats,
		Errors:   errors,
		Duration: time.Since(started).Milliseconds(),
	}
}

// --- Individual sync functions -----------------------------------------------

func syncCampaigns(ctx context.Context, db *DB, client *DonorboxClient) (int, error) {
	items, err := client.ListAllCampaigns()
	if err != nil {
		return 0, err
	}

	records := make([]Campaign, 0, len(items))
	for _, c := range items {
		records = append(records, Campaign{
			ID:             c.ID,
			Name:           strPtr(c.Name),
			Slug:           strPtr(c.Slug),
			Currency:       defaultStr(c.Currency, "USD"),
			GoalAmount:     parseFloatPtr(c.GoalAmount),
			TotalRaised:    parseFloat(c.TotalRaised),
			DonationsCount: c.DonationsCount,
			IsActive:       c.IsActive,
			CreatedAt:      parseTimePtr(c.CreatedAt),
			UpdatedAt:      parseTimePtr(c.UpdatedAt),
		})
	}
	return db.UpsertCampaigns(ctx, records)
}

func syncDonors(ctx context.Context, db *DB, client *DonorboxClient) (int, error) {
	items, err := client.ListAllDonors()
	if err != nil {
		return 0, err
	}

	records := make([]Donor, 0, len(items))
	for _, d := range items {
		records = append(records, Donor{
			ID:             d.ID,
			FirstName:      strPtrOrNil(d.FirstName),
			LastName:       strPtrOrNil(d.LastName),
			Email:          strPtrOrNil(d.Email),
			Phone:          strPtrOrNil(d.Phone),
			Address:        strPtrOrNil(d.Address),
			City:           strPtrOrNil(d.City),
			State:          strPtrOrNil(d.State),
			ZipCode:        strPtrOrNil(d.ZipCode),
			Country:        strPtrOrNil(d.Country),
			Employer:       strPtrOrNil(d.Employer),
			DonationsCount: d.DonationsCount,
			LastDonationAt: parseTimePtr(d.LastDonationAt),
			Total:          parseFloat(d.Total),
			CreatedAt:      parseTimePtr(d.CreatedAt),
			UpdatedAt:      parseTimePtr(d.UpdatedAt),
		})
	}
	return db.UpsertDonors(ctx, records)
}

func syncDonations(ctx context.Context, db *DB, client *DonorboxClient, dateFrom string) (int, error) {
	items, err := client.ListAllDonations(dateFrom, "")
	if err != nil {
		return 0, err
	}

	records := make([]Donation, 0, len(items))
	for _, d := range items {
		records = append(records, mapAPIDonation(d))
	}
	return db.UpsertDonations(ctx, records)
}

func syncPlans(ctx context.Context, db *DB, client *DonorboxClient) (int, error) {
	items, err := client.ListAllPlans()
	if err != nil {
		return 0, err
	}

	records := make([]Plan, 0, len(items))
	for _, p := range items {
		records = append(records, Plan{
			ID:               p.ID,
			CampaignID:       intPtrNonZero(p.Campaign.ID),
			CampaignName:     strPtrOrNil(p.Campaign.Name),
			DonorID:          intPtrNonZero(p.Donor.ID),
			DonorEmail:       strPtrOrNil(p.Donor.Email),
			Type:             strPtrOrNil(p.Type),
			Amount:           parseFloat(p.Amount),
			Currency:         defaultStr(p.Currency, "USD"),
			Status:           strPtrOrNil(p.Status),
			StartedAt:        parseTimePtr(p.StartedAt),
			LastDonationDate: parseTimePtr(p.LastDonationDate),
			NextDonationDate: parseTimePtr(p.NextDonationDate),
			CreatedAt:        parseTimePtr(p.CreatedAt),
			UpdatedAt:        parseTimePtr(p.UpdatedAt),
		})
	}
	return db.UpsertPlans(ctx, records)
}

func syncEvents(ctx context.Context, db *DB, client *DonorboxClient) (int, error) {
	items, err := client.ListAllEvents()
	if err != nil {
		return 0, err
	}

	records := make([]Event, 0, len(items))
	for _, e := range items {
		records = append(records, Event{
			ID:           e.ID,
			Name:         strPtrOrNil(e.Name),
			Slug:         strPtrOrNil(e.Slug),
			Description:  strPtrOrNil(e.Description),
			StartDate:    parseTimePtr(e.StartDate),
			EndDate:      parseTimePtr(e.EndDate),
			Timezone:     strPtrOrNil(e.Timezone),
			VenueName:    strPtrOrNil(e.VenueName),
			Address:      strPtrOrNil(e.Address),
			City:         strPtrOrNil(e.City),
			State:        strPtrOrNil(e.State),
			Country:      strPtrOrNil(e.Country),
			ZipCode:      strPtrOrNil(e.ZipCode),
			Currency:     defaultStr(e.Currency, "USD"),
			TicketsCount: e.TicketsCount,
			IsActive:     e.IsActive,
			CreatedAt:    parseTimePtr(e.CreatedAt),
			UpdatedAt:    parseTimePtr(e.UpdatedAt),
		})
	}
	return db.UpsertEvents(ctx, records)
}

func syncTickets(ctx context.Context, db *DB, client *DonorboxClient) (int, error) {
	items, err := client.ListAllTickets()
	if err != nil {
		return 0, err
	}

	records := make([]Ticket, 0, len(items))
	for _, t := range items {
		records = append(records, Ticket{
			ID:         t.ID,
			EventID:    intPtrNonZero(t.Event.ID),
			EventName:  strPtrOrNil(t.Event.Name),
			DonorID:    intPtrNonZero(t.Donor.ID),
			DonorEmail: strPtrOrNil(t.Donor.Email),
			TicketType: strPtrOrNil(t.TicketType),
			Quantity:   t.Quantity,
			Amount:     parseFloat(t.Amount),
			Currency:   defaultStr(t.Currency, "USD"),
			Status:     strPtrOrNil(t.Status),
			CreatedAt:  parseTimePtr(t.CreatedAt),
			UpdatedAt:  parseTimePtr(t.UpdatedAt),
		})
	}
	return db.UpsertTickets(ctx, records)
}

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

// --- Helpers -----------------------------------------------------------------

func mapAPIDonation(d APIDonation) Donation {
	var donorName *string
	if d.Donor.Name != "" {
		donorName = &d.Donor.Name
	} else {
		parts := []string{}
		if d.Donor.FirstName != "" {
			parts = append(parts, d.Donor.FirstName)
		}
		if d.Donor.LastName != "" {
			parts = append(parts, d.Donor.LastName)
		}
		if len(parts) > 0 {
			joined := strings.Join(parts, " ")
			donorName = &joined
		}
	}

	var questions json.RawMessage
	if d.Questions != nil {
		q, err := json.Marshal(d.Questions)
		if err == nil {
			questions = q
		}
	}
	if questions == nil {
		questions = json.RawMessage("[]")
	}

	return Donation{
		ID:                 d.ID,
		CampaignID:         intPtrNonZero(d.Campaign.ID),
		CampaignName:       strPtrOrNil(d.Campaign.Name),
		DonorID:            intPtrNonZero(d.Donor.ID),
		DonorEmail:         strPtrOrNil(d.Donor.Email),
		DonorName:          donorName,
		Amount:             parseFloat(d.Amount),
		ConvertedAmount:    parseFloatPtr(d.ConvertedAmount),
		ConvertedNetAmount: parseFloatPtr(d.ConvertedNetAmt),
		AmountRefunded:     parseFloat(d.AmountRefunded),
		Currency:           defaultStr(d.Currency, "USD"),
		DonationType:       strPtrOrNil(d.DonationType),
		DonationDate:       parseTimePtr(d.DonationDate),
		ProcessingFee:      parseFloatPtr(d.ProcessingFee),
		Status:             strPtrOrNil(d.Status),
		Recurring:          d.Recurring,
		Comment:            strPtrOrNil(d.Comment),
		Designation:        strPtrOrNil(d.Designation),
		StripeChargeID:     d.StripeChargeID,
		PaypalTxnID:        d.PaypalTxnID,
		Questions:          questions,
		CreatedAt:          parseTimePtr(d.CreatedAt),
		UpdatedAt:          parseTimePtr(d.UpdatedAt),
	}
}

func verifyHMAC(payload []byte, signature, secret string) bool {
	if secret == "" || signature == "" {
		return false
	}

	// Strip "sha256=" prefix if present
	signature = strings.TrimPrefix(signature, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}

func parsePagination(r *http.Request) (int, int) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

func strPtr(s string) *string {
	return &s
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtrNonZero(i int) *int {
	if i == 0 {
		return nil
	}
	return &i
}

func parseFloat(s string) float64 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseFloatPtr(s string) *float64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseTimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	// Try RFC3339 first, then other common formats
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
