package internal

import (
	"fmt"
	"net/url"
	"time"
)

type TransactionSearchResponse struct {
	TransactionDetails []TransactionDetail `json:"transaction_details"`
	TotalItems         int                 `json:"total_items"`
	TotalPages         int                 `json:"total_pages"`
	Page               int                 `json:"page"`
}

// TransactionDetail holds individual transaction data from the search API.
type TransactionDetail struct {
	TransactionInfo TransactionInfo `json:"transaction_info"`
	PayerInfo       *PayerInfo      `json:"payer_info"`
}

// TransactionInfo contains the core transaction fields.
type TransactionInfo struct {
	TransactionID             string  `json:"transaction_id"`
	TransactionEventCode      string  `json:"transaction_event_code"`
	TransactionInitiationDate string  `json:"transaction_initiation_date"`
	TransactionUpdatedDate    string  `json:"transaction_updated_date"`
	TransactionAmount         *Money  `json:"transaction_amount"`
	FeeAmount                 *Money  `json:"fee_amount"`
	TransactionStatus         string  `json:"transaction_status"`
	TransactionSubject        *string `json:"transaction_subject"`
	TransactionNote           *string `json:"transaction_note"`
}

// PayerInfo holds payer details from transactions.
type PayerInfo struct {
	AccountID    *string    `json:"account_id"`
	EmailAddress *string    `json:"email_address"`
	PayerName    *PayerName `json:"payer_name"`
}

// PayerName holds given and surname.
type PayerName struct {
	GivenName *string `json:"given_name"`
	Surname   *string `json:"surname"`
}

// Money represents a PayPal monetary value.
type Money struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

// SearchTransactions searches for transactions in the given date range,
// handling PayPal's 31-day maximum window by batching requests.
// Size-cap exception: 54L — single-responsibility operation; splitting would create artificial fragmentation without structural or maintainability gain.
func (c *PayPalClient) SearchTransactions(startDate, endDate string) ([]TransactionDetail, error) {
	start, err := time.Parse(time.RFC3339, startDate)
	if err != nil {
		return nil, fmt.Errorf("parse start date: %w", err)
	}
	end, err := time.Parse(time.RFC3339, endDate)
	if err != nil {
		return nil, fmt.Errorf("parse end date: %w", err)
	}

	var allTransactions []TransactionDetail
	windowStart := start

	for windowStart.Before(end) {
		windowEnd := windowStart.Add(31 * 24 * time.Hour)
		if windowEnd.After(end) {
			windowEnd = end
		}

		page := 1
		totalPages := 1

		for page <= totalPages {
			params := url.Values{}
			params.Set("start_date", windowStart.Format(time.RFC3339))
			params.Set("end_date", windowEnd.Format(time.RFC3339))
			params.Set("fields", "all")
			params.Set("page_size", "500")
			params.Set("page", fmt.Sprintf("%d", page))

			resp, err := c.doRequest("GET", "/v1/reporting/transactions?"+params.Encode(), nil)
			if err != nil {
				return allTransactions, fmt.Errorf("search transactions page %d: %w", page, err)
			}

			var result TransactionSearchResponse
			err = decodeAndClose(resp, &result)
			if err != nil {
				return allTransactions, err
			}

			allTransactions = append(allTransactions, result.TransactionDetails...)

			if result.TotalPages > 0 {
				totalPages = result.TotalPages
			}
			page++
		}

		windowStart = windowEnd
	}

	return allTransactions, nil
}

// --- Orders ------------------------------------------------------------------

// PayPalOrder represents a PayPal order resource.
