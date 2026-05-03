package internal

import (
	"testing"
)

// TestResourceToObjectType verifies known resource name mappings and the
// default passthrough for unknown types.
func TestResourceToObjectType(t *testing.T) {
	cases := []struct {
		resource string
		want     string
	}{
		{"customers", "customer"},
		{"products", "product"},
		{"prices", "price"},
		{"coupons", "coupon"},
		{"promotion_codes", "promotion_code"},
		{"subscriptions", "subscription"},
		{"invoices", "invoice"},
		{"charges", "charge"},
		{"refunds", "refund"},
		{"disputes", "dispute"},
		{"payment_intents", "payment_intent"},
		{"setup_intents", "setup_intent"},
		{"balance_transactions", "balance_transaction"},
		{"tax_rates", "tax_rate"},
		{"checkout_sessions", "checkout.session"},
		// Unknown resource passes through as-is.
		{"custom_resource", "custom_resource"},
		{"", ""},
	}
	for _, tc := range cases {
		got := resourceToObjectType(tc.resource)
		if got != tc.want {
			t.Errorf("resourceToObjectType(%q) = %q, want %q", tc.resource, got, tc.want)
		}
	}
}

// TestClientTruncate verifies the truncate helper in client.go.
func TestClientTruncate(t *testing.T) {
	cases := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 20, "short"},
		{"exactly", 7, "exactly"},
		{"toolongstring", 7, "toolong..."},
		{"", 5, ""},
	}
	for _, tc := range cases {
		got := truncate(tc.input, tc.maxLen)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
		}
	}
}
