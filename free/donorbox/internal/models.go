package internal

import (
	"encoding/json"
	"time"
)

// --- Database record types ---------------------------------------------------

// Campaign represents a row in np_donorbox_campaigns.
type Campaign struct {
	ID              int        `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	Name            *string    `json:"name"`
	Slug            *string    `json:"slug"`
	Currency        string     `json:"currency"`
	GoalAmount      *float64   `json:"goal_amount"`
	TotalRaised     float64    `json:"total_raised"`
	DonationsCount  int        `json:"donations_count"`
	IsActive        bool       `json:"is_active"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Donor represents a row in np_donorbox_donors.
type Donor struct {
	ID              int        `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	FirstName       *string    `json:"first_name"`
	LastName        *string    `json:"last_name"`
	Email           *string    `json:"email"`
	Phone           *string    `json:"phone"`
	Address         *string    `json:"address"`
	City            *string    `json:"city"`
	State           *string    `json:"state"`
	ZipCode         *string    `json:"zip_code"`
	Country         *string    `json:"country"`
	Employer        *string    `json:"employer"`
	DonationsCount  int        `json:"donations_count"`
	LastDonationAt  *time.Time `json:"last_donation_at"`
	Total           float64    `json:"total"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Donation represents a row in np_donorbox_donations.
type Donation struct {
	ID                 int              `json:"id"`
	SourceAccountID    string           `json:"source_account_id"`
	CampaignID         *int             `json:"campaign_id"`
	CampaignName       *string          `json:"campaign_name"`
	DonorID            *int             `json:"donor_id"`
	DonorEmail         *string          `json:"donor_email"`
	DonorName          *string          `json:"donor_name"`
	Amount             float64          `json:"amount"`
	ConvertedAmount    *float64         `json:"converted_amount"`
	ConvertedNetAmount *float64         `json:"converted_net_amount"`
	AmountRefunded     float64          `json:"amount_refunded"`
	Currency           string           `json:"currency"`
	DonationType       *string          `json:"donation_type"`
	DonationDate       *time.Time       `json:"donation_date"`
	ProcessingFee      *float64         `json:"processing_fee"`
	Status             *string          `json:"status"`
	Recurring          bool             `json:"recurring"`
	Comment            *string          `json:"comment"`
	Designation        *string          `json:"designation"`
	StripeChargeID     *string          `json:"stripe_charge_id"`
	PaypalTxnID        *string          `json:"paypal_transaction_id"`
	Questions          json.RawMessage  `json:"questions"`
	CreatedAt          *time.Time       `json:"created_at"`
	UpdatedAt          *time.Time       `json:"updated_at"`
	SyncedAt           *time.Time       `json:"synced_at"`
}

// Plan represents a row in np_donorbox_plans.
type Plan struct {
	ID               int        `json:"id"`
	SourceAccountID  string     `json:"source_account_id"`
	CampaignID       *int       `json:"campaign_id"`
	CampaignName     *string    `json:"campaign_name"`
	DonorID          *int       `json:"donor_id"`
	DonorEmail       *string    `json:"donor_email"`
	Type             *string    `json:"type"`
	Amount           float64    `json:"amount"`
	Currency         string     `json:"currency"`
	Status           *string    `json:"status"`
	StartedAt        *time.Time `json:"started_at"`
	LastDonationDate *time.Time `json:"last_donation_date"`
	NextDonationDate *time.Time `json:"next_donation_date"`
	CreatedAt        *time.Time `json:"created_at"`
	UpdatedAt        *time.Time `json:"updated_at"`
	SyncedAt         *time.Time `json:"synced_at"`
}

// Event represents a row in np_donorbox_events.
type Event struct {
	ID              int        `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	Name            *string    `json:"name"`
	Slug            *string    `json:"slug"`
	Description     *string    `json:"description"`
	StartDate       *time.Time `json:"start_date"`
	EndDate         *time.Time `json:"end_date"`
	Timezone        *string    `json:"timezone"`
	VenueName       *string    `json:"venue_name"`
	Address         *string    `json:"address"`
	City            *string    `json:"city"`
	State           *string    `json:"state"`
	Country         *string    `json:"country"`
	ZipCode         *string    `json:"zip_code"`
	Currency        string     `json:"currency"`
	TicketsCount    int        `json:"tickets_count"`
	IsActive        bool       `json:"is_active"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Ticket represents a row in np_donorbox_tickets.
type Ticket struct {
	ID              int        `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	EventID         *int       `json:"event_id"`
	EventName       *string    `json:"event_name"`
	DonorID         *int       `json:"donor_id"`
	DonorEmail      *string    `json:"donor_email"`
	TicketType      *string    `json:"ticket_type"`
	Quantity        int        `json:"quantity"`
	Amount          float64    `json:"amount"`
	Currency        string     `json:"currency"`
	Status          *string    `json:"status"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// WebhookEvent represents a row in np_donorbox_webhook_events.
type WebhookEvent struct {
	ID              string          `json:"id"`
	EventType       *string         `json:"event_type"`
	Payload         json.RawMessage `json:"payload"`
	SourceAccountID string          `json:"source_account_id"`
	Processed       bool            `json:"processed"`
	ProcessedAt     *time.Time      `json:"processed_at"`
	Error           *string         `json:"error"`
	CreatedAt       *time.Time      `json:"created_at"`
	SyncedAt        *time.Time      `json:"synced_at"`
}

// --- Donorbox API response types ---------------------------------------------

// APICampaign is the JSON shape returned by the Donorbox API for campaigns.
type APICampaign struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Currency       string `json:"currency"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	GoalAmount     string `json:"goal_amount"`
	TotalRaised    string `json:"total_raised"`
	DonationsCount int    `json:"donations_count"`
	IsActive       bool   `json:"is_active"`
}

// APIDonor is the JSON shape returned by the Donorbox API for donors.
type APIDonor struct {
	ID             int    `json:"id"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	Address        string `json:"address"`
	City           string `json:"city"`
	State          string `json:"state"`
	ZipCode        string `json:"zip_code"`
	Country        string `json:"country"`
	Employer       string `json:"employer"`
	DonationsCount int    `json:"donations_count"`
	LastDonationAt string `json:"last_donation_at"`
	Total          string `json:"total"`
}

// APIDonation is the JSON shape returned by the Donorbox API for donations.
type APIDonation struct {
	ID                int                    `json:"id"`
	Campaign          APIDonationCampaign    `json:"campaign"`
	Donor             APIDonationDonor       `json:"donor"`
	Amount            string                 `json:"amount"`
	ConvertedAmount   string                 `json:"converted_amount"`
	ConvertedNetAmt   string                 `json:"converted_net_amount"`
	Recurring         bool                   `json:"recurring"`
	AmountRefunded    string                 `json:"amount_refunded"`
	Currency          string                 `json:"currency"`
	DonationType      string                 `json:"donation_type"`
	DonationDate      string                 `json:"donation_date"`
	ProcessingFee     string                 `json:"processing_fee"`
	Status            string                 `json:"status"`
	Comment           string                 `json:"comment"`
	Designation       string                 `json:"designation"`
	StripeChargeID    *string                `json:"stripe_charge_id"`
	PaypalTxnID       *string                `json:"paypal_transaction_id"`
	Questions         []map[string]string    `json:"questions"`
	CreatedAt         string                 `json:"created_at"`
	UpdatedAt         string                 `json:"updated_at"`
}

// APIDonationCampaign is the nested campaign object in an API donation.
type APIDonationCampaign struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// APIDonationDonor is the nested donor object in an API donation.
type APIDonationDonor struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// APIPlan is the JSON shape returned by the Donorbox API for recurring plans.
type APIPlan struct {
	ID               int              `json:"id"`
	Campaign         APIPlanCampaign  `json:"campaign"`
	Donor            APIPlanDonor     `json:"donor"`
	Type             string           `json:"type"`
	Amount           string           `json:"amount"`
	Currency         string           `json:"currency"`
	Status           string           `json:"status"`
	StartedAt        string           `json:"started_at"`
	LastDonationDate string           `json:"last_donation_date"`
	NextDonationDate string           `json:"next_donation_date"`
	CreatedAt        string           `json:"created_at"`
	UpdatedAt        string           `json:"updated_at"`
}

// APIPlanCampaign is the nested campaign object in an API plan.
type APIPlanCampaign struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// APIPlanDonor is the nested donor object in an API plan.
type APIPlanDonor struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// APIEvent is the JSON shape returned by the Donorbox API for events.
type APIEvent struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Description  string `json:"description"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	Timezone     string `json:"timezone"`
	VenueName    string `json:"venue_name"`
	Address      string `json:"address"`
	City         string `json:"city"`
	State        string `json:"state"`
	Country      string `json:"country"`
	ZipCode      string `json:"zip_code"`
	Currency     string `json:"currency"`
	TicketsCount int    `json:"tickets_count"`
	IsActive     bool   `json:"is_active"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// APITicket is the JSON shape returned by the Donorbox API for tickets.
type APITicket struct {
	ID         int              `json:"id"`
	Event      APITicketEvent   `json:"event"`
	Donor      APITicketDonor   `json:"donor"`
	TicketType string           `json:"ticket_type"`
	Quantity   int              `json:"quantity"`
	Amount     string           `json:"amount"`
	Currency   string           `json:"currency"`
	Status     string           `json:"status"`
	CreatedAt  string           `json:"created_at"`
	UpdatedAt  string           `json:"updated_at"`
}

// APITicketEvent is the nested event object in an API ticket.
type APITicketEvent struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// APITicketDonor is the nested donor object in an API ticket.
type APITicketDonor struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// --- Sync result types -------------------------------------------------------

// SyncStats holds counts of synced records per entity.
type SyncStats struct {
	Campaigns    int        `json:"campaigns"`
	Donors       int        `json:"donors"`
	Donations    int        `json:"donations"`
	Plans        int        `json:"plans"`
	Events       int        `json:"events"`
	Tickets      int        `json:"tickets"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
}

// SyncResult is the outcome of a sync or reconcile operation.
type SyncResult struct {
	Success  bool      `json:"success"`
	Stats    SyncStats `json:"stats"`
	Errors   []string  `json:"errors"`
	Duration int64     `json:"duration_ms"`
}
