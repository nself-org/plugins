//go:build s3coverage

package s3coverage

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/nself-org/plugin-storage/internal"
)

// newPluginWithQuota is the real plugin server with STORAGE_ACCOUNT_QUOTA_BYTES set to quota.
func newPluginWithQuota(t *testing.T, quota int64) *pluginHarness {
	t.Helper()
	rec := &recorder{}
	r := chi.NewRouter()
	cfg := &internal.Config{Port: "0", InternalSecret: testInternalSecret, AccountQuotaBytes: quota}
	internal.NewServer(pool, cfg, newStore(t, rec)).Routes(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return &pluginHarness{url: ts.URL, rec: rec}
}

// TestPlugin_AccountQuota runs the real quota SQL (lookupPut) against the suite database.
func TestPlugin_AccountQuota(t *testing.T) {
	p := newPluginWithQuota(t, 100)
	// Random account ids keep rows from other tests out of the stored total.
	a, b := "quota-a-"+randHex(4), "quota-b-"+randHex(4)
	bucketA, bucketB := p.createBucket(t, a), p.createBucket(t, b)
	put := func(tenant, bucket, key string, n int) int {
		code, _, _ := p.call(t, tenant, http.MethodPut, "/storage/buckets/"+bucket+"/objects/"+key,
			strings.NewReader(strings.Repeat("x", n)), "application/octet-stream")
		return code
	}

	if code := put(a, bucketA, "one.bin", 60); code != http.StatusOK {
		t.Fatalf("first 60 B PUT under a 100 B quota: %d", code)
	}
	code, _, body := p.call(t, a, http.MethodPut, "/storage/buckets/"+bucketA+"/objects/two.bin",
		strings.NewReader(strings.Repeat("x", 60)), "application/octet-stream")
	if code != http.StatusRequestEntityTooLarge || !strings.Contains(string(body), "account quota exceeded") {
		t.Fatalf("second 60 B key: %d %s, want 413 account quota exceeded", code, body)
	}
	// An overwrite is charged only for the growth: 60 B replacing 60 B fits.
	if code := put(a, bucketA, "one.bin", 60); code != http.StatusOK {
		t.Fatalf("60 B overwrite of the 60 B key: %d, want 200", code)
	}
	// The quota is per account: another account still has its whole budget.
	if code := put(b, bucketB, "one.bin", 60); code != http.StatusOK {
		t.Fatalf("other account's 60 B PUT: %d, want 200", code)
	}
}
