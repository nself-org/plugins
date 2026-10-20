package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/usage"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestServeUsageStartupTick(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	for _, account := range []string{"org:acme", "user:alice"} {
		t.Run(account, func(t *testing.T) {
			home := serveFixture(t, quietHost())
			path := filepath.Join(home, "state.db")
			st, err := store.Open(path, store.Options{})
			if err != nil {
				t.Fatal(err)
			}
			for key, value := range map[string]any{usage.KeyBillingImport: true, usage.KeyBillingAccount: account} {
				raw, _ := json.Marshal(value)
				if err := st.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject, Key: key, Value: raw, Actor: "test"}, nil); err != nil {
					t.Fatal(err)
				}
			}
			if err := st.Close(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			r := usage.Default()
			old := r.Build
			defer func() { r.Build = old }()
			var called atomic.Bool
			r.Build = func(_ context.Context, eff *policy.Effective) (*usage.Importer, error) {
				settings, err := usage.SettingsFrom(eff)
				if err == nil && settings.Import && settings.Account == account {
					called.Store(true)
				}
				cancel()
				return nil, errors.New("test importer reached")
			}
			code := serveV15(ctx, []string{"--insecure", "--addr", "127.0.0.1:0", "--workdir", filepath.Join(home, "wd"), "--data-dir", home}, nil)
			if code != 0 || !called.Load() {
				t.Fatalf("serve=%d importer reached=%v", code, called.Load())
			}
			st, err = store.Open(path, store.Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := st.Close(); err != nil {
					t.Error(err)
				}
			}()
			if _, ok, err := st.ProviderMarkGet(context.Background(), usage.AttemptName, account); err != nil || !ok {
				t.Fatalf("attempt mark: present=%v err=%v", ok, err)
			}
			if r.Policy != nil || r.Store != nil {
				t.Fatal("usage wiring survived coordinator shutdown")
			}
		})
	}
}
