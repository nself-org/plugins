package internal

import (
	"strings"
	"testing"
)

// TestBaseURL verifies that BaseURL produces the expected Shopify Admin API URL.
func TestBaseURL(t *testing.T) {
	cfg := &Config{
		ShopDomain: "mystore.myshopify.com",
		APIVersion: "2024-01",
	}
	want := "https://mystore.myshopify.com/admin/api/2024-01"
	got := cfg.BaseURL()
	if got != want {
		t.Errorf("BaseURL() = %q, want %q", got, want)
	}
}

// TestBaseURLFor verifies that BaseURLFor uses the given shop domain.
func TestBaseURLFor(t *testing.T) {
	cfg := &Config{APIVersion: "2024-01"}
	want := "https://other.myshopify.com/admin/api/2024-01"
	got := cfg.BaseURLFor("other.myshopify.com")
	if got != want {
		t.Errorf("BaseURLFor() = %q, want %q", got, want)
	}
}

// TestBaseURLFor_ContainsDomain verifies that the shop domain appears in the URL.
func TestBaseURLFor_ContainsDomain(t *testing.T) {
	cfg := &Config{APIVersion: "2024-04"}
	domain := "test-shop.myshopify.com"
	url := cfg.BaseURLFor(domain)
	if !strings.Contains(url, domain) {
		t.Errorf("BaseURLFor(%q) = %q, expected domain in URL", domain, url)
	}
}

// TestSplitCSV verifies the CSV helper for multi-account configuration.
func TestSplitCSV(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{"  x , y ", []string{"x", "y"}},
		{"", nil},
		{"solo", []string{"solo"}},
		{"a,,b", []string{"a", "b"}},
	}
	for _, tc := range cases {
		got := splitCSV(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("splitCSV(%q) = %v (len %d), want %v (len %d)",
				tc.input, got, len(got), tc.want, len(tc.want))
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitCSV(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

// TestLoadConfig_DefaultAPIVersion verifies that the API version defaults to
// "2024-01" when SHOPIFY_API_VERSION is not set.
func TestLoadConfig_DefaultAPIVersion(t *testing.T) {
	t.Setenv("SHOPIFY_API_VERSION", "")
	cfg := LoadConfig()
	if cfg.APIVersion != "2024-01" {
		t.Errorf("default APIVersion = %q, want %q", cfg.APIVersion, "2024-01")
	}
}

// TestLoadConfig_CustomAPIVersion verifies that an explicit env var is used.
func TestLoadConfig_CustomAPIVersion(t *testing.T) {
	t.Setenv("SHOPIFY_API_VERSION", "2025-01")
	cfg := LoadConfig()
	if cfg.APIVersion != "2025-01" {
		t.Errorf("APIVersion = %q, want %q", cfg.APIVersion, "2025-01")
	}
}

// TestLoadConfig_DefaultSyncInterval verifies that the default sync interval
// of 3600 is applied when SHOPIFY_SYNC_INTERVAL is unset.
func TestLoadConfig_DefaultSyncInterval(t *testing.T) {
	t.Setenv("SHOPIFY_SYNC_INTERVAL", "")
	cfg := LoadConfig()
	if cfg.SyncInterval != 3600 {
		t.Errorf("default SyncInterval = %d, want 3600", cfg.SyncInterval)
	}
}

// TestLoadConfig_MultiAccount verifies that paired CSV env vars produce
// AccountConfig entries.
func TestLoadConfig_MultiAccount(t *testing.T) {
	t.Setenv("SHOPIFY_ACCESS_TOKENS", "tok1,tok2")
	t.Setenv("SHOPIFY_SHOP_DOMAINS", "store1.myshopify.com,store2.myshopify.com")
	t.Setenv("SHOPIFY_ACCOUNT_LABELS", "first,second")
	t.Setenv("SHOPIFY_WEBHOOK_SECRETS", "s1,s2")

	cfg := LoadConfig()
	if len(cfg.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(cfg.Accounts))
	}
	if cfg.Accounts[0].Label != "first" || cfg.Accounts[1].Label != "second" {
		t.Errorf("labels mismatch: got %q, %q", cfg.Accounts[0].Label, cfg.Accounts[1].Label)
	}
	if cfg.Accounts[0].ShopDomain != "store1.myshopify.com" {
		t.Errorf("account[0].ShopDomain = %q, want %q", cfg.Accounts[0].ShopDomain, "store1.myshopify.com")
	}
}
