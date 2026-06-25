package internal

import (
	"time"
)

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
