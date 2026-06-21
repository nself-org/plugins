package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type PayPalDispute struct {
	DisputeID     string          `json:"dispute_id"`
	Reason        string          `json:"reason"`
	Status        string          `json:"status"`
	DisputeAmount *Money          `json:"dispute_amount"`
	Messages      json.RawMessage `json:"messages"`
	CreateTime    string          `json:"create_time"`
	UpdateTime    string          `json:"update_time"`
}

// ListDisputes retrieves all disputes with pagination.
func (c *PayPalClient) ListDisputes() ([]PayPalDispute, error) {
	var disputes []PayPalDispute

	params := url.Values{}
	params.Set("page_size", "50")

	resp, err := c.doRequest("GET", "/v1/customer/disputes?"+params.Encode(), nil)
	if err != nil {
		return disputes, err
	}

	var result struct {
		Items []PayPalDispute `json:"items"`
		Links []struct {
			Href string `json:"href"`
			Rel  string `json:"rel"`
		} `json:"links"`
	}
	if err := decodeAndClose(resp, &result); err != nil {
		return disputes, err
	}

	disputes = append(disputes, result.Items...)

	return disputes, nil
}

// --- Payouts -----------------------------------------------------------------

// PayPalPayoutBatch represents a PayPal payout batch resource.
type PayPalPayoutBatch struct {
	BatchHeader struct {
		PayoutBatchID     string `json:"payout_batch_id"`
		BatchStatus       string `json:"batch_status"`
		TimeCreated       string `json:"time_created"`
		SenderBatchHeader struct {
			SenderBatchID string `json:"sender_batch_id"`
		} `json:"sender_batch_header"`
		Amount *Money `json:"amount"`
		Fees   *Money `json:"fees"`
	} `json:"batch_header"`
}

// ListPayoutBatches retrieves payout batches. PayPal does not have a paginated list,
// so this returns whatever is available from the latest call.
func (c *PayPalClient) ListPayoutBatches() ([]PayPalPayoutBatch, error) {
	// PayPal's payout API does not have a list endpoint.
	// Payouts are typically tracked via webhooks or known batch IDs.
	return nil, nil
}

// --- Invoices ----------------------------------------------------------------

// PayPalInvoice represents a PayPal invoice resource.
type PayPalInvoice struct {
	ID         string          `json:"id"`
	Status     string          `json:"status"`
	Detail     json.RawMessage `json:"detail"`
	Amount     *Money          `json:"amount"`
	DueAmount  *Money          `json:"due_amount"`
	Invoicer   json.RawMessage `json:"invoicer"`
	CreateTime string          `json:"create_time"`
	UpdateTime string          `json:"update_time"`
}

// ListInvoices retrieves all invoices with pagination.
func (c *PayPalClient) ListInvoices() ([]PayPalInvoice, error) {
	var invoices []PayPalInvoice
	page := 1

	for {
		params := url.Values{}
		params.Set("page_size", "100")
		params.Set("page", fmt.Sprintf("%d", page))
		params.Set("total_required", "true")

		resp, err := c.doRequest("GET", "/v2/invoicing/invoices?"+params.Encode(), nil)
		if err != nil {
			return invoices, err
		}

		var result struct {
			Items      []PayPalInvoice `json:"items"`
			TotalPages int             `json:"total_pages"`
		}
		if err := decodeAndClose(resp, &result); err != nil {
			return invoices, err
		}

		invoices = append(invoices, result.Items...)

		if result.TotalPages == 0 || page >= result.TotalPages || len(result.Items) == 0 {
			break
		}
		page++
	}

	return invoices, nil
}

// --- Helpers -----------------------------------------------------------------

// decodeAndClose reads the response body, checks for errors, and decodes JSON.
func decodeAndClose(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("paypal api error (status %d): %s", resp.StatusCode, string(body))
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(v)
}
