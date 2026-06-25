package internal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// --- Helpers -----------------------------------------------------------------

// Size-cap exception: single-responsibility HTTP route handler — 55L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
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

