package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

var paymentsEnv = []string{
	"PAYMENTS_PORT", "DATABASE_URL", "PAYMENTS_DEFAULT_PROVIDER",
	"STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET", "STRIPE_PRICE_IDS",
	"LEMONSQUEEZY_API_KEY", "LEMONSQUEEZY_STORE_ID", "LEMONSQUEEZY_WEBHOOK_SECRET",
	"PADDLE_API_KEY", "PADDLE_WEBHOOK_SECRET", "PADDLE_PUBLIC_KEY",
	"PAYMENTS_DUNNING_GRACE_DAYS", "PAYMENTS_DUNNING_RETRY_INTERVALS",
	"PLUGIN_INTERNAL_SECRET", "LOG_LEVEL",
}

func clearEnv(t *testing.T) {
	t.Helper()
	saved := make(map[string]string, len(paymentsEnv))
	for _, k := range paymentsEnv {
		saved[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	})
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)
	c := Load()
	if c.Port != 3086 {
		t.Errorf("Port = %d; want 3086", c.Port)
	}
	if c.DefaultProvider != "stripe" {
		t.Errorf("DefaultProvider = %q; want stripe", c.DefaultProvider)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel = %q; want info", c.LogLevel)
	}
	if c.DunningGraceDays != 7 {
		t.Errorf("DunningGraceDays = %d; want 7", c.DunningGraceDays)
	}
	if !reflect.DeepEqual(c.DunningRetryIntervals, []int{1, 3, 7}) {
		t.Errorf("DunningRetryIntervals = %v; want [1 3 7]", c.DunningRetryIntervals)
	}
}

func TestLoad_Overrides(t *testing.T) {
	clearEnv(t)
	os.Setenv("PAYMENTS_PORT", "9000")
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("PAYMENTS_DEFAULT_PROVIDER", "lemonsqueezy")
	os.Setenv("STRIPE_SECRET_KEY", "sk")
	os.Setenv("LEMONSQUEEZY_API_KEY", "lk")
	os.Setenv("PADDLE_API_KEY", "pk")
	os.Setenv("PAYMENTS_DUNNING_GRACE_DAYS", "14")
	os.Setenv("PAYMENTS_DUNNING_RETRY_INTERVALS", "2,5,10")
	os.Setenv("LOG_LEVEL", "debug")
	c := Load()
	if c.Port != 9000 || c.DefaultProvider != "lemonsqueezy" ||
		c.StripeSecretKey != "sk" || c.LemonSqueezyAPIKey != "lk" || c.PaddleAPIKey != "pk" ||
		c.DunningGraceDays != 14 || c.LogLevel != "debug" {
		t.Errorf("overrides: %+v", c)
	}
	if !reflect.DeepEqual(c.DunningRetryIntervals, []int{2, 5, 10}) {
		t.Errorf("intervals = %v", c.DunningRetryIntervals)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		c      Config
		want   string // substring of error or "" for OK
	}{
		{"missing db", Config{DefaultProvider: "stripe", StripeSecretKey: "k"}, "DATABASE_URL"},
		{"missing provider", Config{DatabaseURL: "x"}, "PROVIDER"},
		{"unknown provider", Config{DatabaseURL: "x", DefaultProvider: "btc"}, "must be one of"},
		{"stripe no key", Config{DatabaseURL: "x", DefaultProvider: "stripe"}, "STRIPE_SECRET_KEY"},
		{"stripe ok", Config{DatabaseURL: "x", DefaultProvider: "stripe", StripeSecretKey: "k"}, ""},
		{"ls no key", Config{DatabaseURL: "x", DefaultProvider: "lemonsqueezy"}, "LEMONSQUEEZY_API_KEY"},
		{"ls ok", Config{DatabaseURL: "x", DefaultProvider: "lemonsqueezy", LemonSqueezyAPIKey: "k"}, ""},
		{"paddle no key", Config{DatabaseURL: "x", DefaultProvider: "paddle"}, "PADDLE_API_KEY"},
		{"paddle ok", Config{DatabaseURL: "x", DefaultProvider: "paddle", PaddleAPIKey: "k"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.c.Validate()
			if tc.want == "" {
				if err != nil {
					t.Errorf("expected ok; got %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q; got nil", tc.want)
				}
				if !strings.Contains(err.Error(), tc.want) {
					t.Errorf("err %q missing %q", err.Error(), tc.want)
				}
			}
		})
	}
}

func TestParseIntList(t *testing.T) {
	def := []int{1, 2, 3}
	cases := []struct {
		in   string
		want []int
	}{
		{"", def},
		{"4,5,6", []int{4, 5, 6}},
		{"4, 5 , 6", []int{4, 5, 6}},
		{"abc", def},
		{"4,bad,6", []int{4, 6}},
		{",,", def},
		{"7", []int{7}},
	}
	for _, tc := range cases {
		got := parseIntList(tc.in, def)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseIntList(%q) = %v; want %v", tc.in, got, tc.want)
		}
	}
}

func TestEnvInt_InvalidFalls(t *testing.T) {
	clearEnv(t)
	os.Setenv("PAYMENTS_PORT", "abc")
	c := Load()
	if c.Port != 3086 {
		t.Errorf("invalid int env should fall back; got %d", c.Port)
	}
}

func TestEnvStr_EmptyFalls(t *testing.T) {
	clearEnv(t)
	os.Setenv("LOG_LEVEL", "")
	c := Load()
	if c.LogLevel != "info" {
		t.Errorf("empty env should use default; got %q", c.LogLevel)
	}
}
