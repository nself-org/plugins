package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nself-org/plugin-storage/internal/s3store"
)

func TestPlanPut(t *testing.T) {
	const max = 100
	cases := []struct {
		name                             string
		quota, used, overwrite, declared int64
		wantLimit                        int64
		wantQuotaBinds                   bool
		wantErr                          error
	}{
		{"quota off, small", 0, 0, 0, 10, max, false, nil},
		{"quota off ignores used", 0, 1 << 40, 0, 10, max, false, nil},
		{"quota off, exactly max", 0, 0, 0, max, max, false, nil},
		{"quota off, over max", 0, 0, 0, max + 1, 0, false, errObjectTooLarge},
		{"quota off, unknown length", 0, 0, 0, -1, max, false, nil},
		{"room above max: max binds", 1000, 100, 0, 50, max, false, nil},
		{"room equals max: max binds", 1000, 900, 0, 50, max, false, nil},
		{"room below max: quota binds", 1000, 950, 0, 20, 50, true, nil},
		{"declared fits room exactly", 1000, 950, 0, 50, 50, true, nil},
		{"declared over room", 1000, 950, 0, 51, 0, false, errQuotaExceeded},
		{"overwrite frees room", 1000, 950, 40, 90, 90, true, nil},
		{"overwrite frees room, over by one", 1000, 950, 40, 91, 0, false, errQuotaExceeded},
		{"overwrite frees some room", 1000, 990, 20, 30, 30, true, nil},
		{"overwrite larger than room still over", 1000, 990, 20, 31, 0, false, errQuotaExceeded},
		{"account already over quota", 1000, 1200, 0, 1, 0, false, errQuotaExceeded},
		{"account over quota, empty body", 1000, 1200, 0, 0, 0, true, nil},
		{"account over quota, unknown length", 1000, 1200, 0, -1, 0, true, nil},
		{"unknown length under room", 1000, 950, 0, -1, 50, true, nil},
		{"over max wins over quota", 1000, 0, 0, max + 1, 0, false, errObjectTooLarge},
	}
	for _, c := range cases {
		got, err := planPut(max, c.quota, c.used, c.overwrite, c.declared)
		if !errors.Is(err, c.wantErr) {
			t.Errorf("%s: err %v, want %v", c.name, err, c.wantErr)
			continue
		}
		if err == nil && (got.limit != c.wantLimit || got.quotaBinds != c.wantQuotaBinds) {
			t.Errorf("%s: plan %+v, want limit %d quotaBinds %v", c.name, got, c.wantLimit, c.wantQuotaBinds)
		}
	}
}

// drainStore stands in for s3store.Put: it reads the whole body and wraps a read failure the way s3store does.
type drainStore struct{ fakeStore }

func (drainStore) Put(_ context.Context, _, _ string, body io.Reader, _ string, _ int64) (s3store.PutResult, error) {
	if _, err := io.Copy(io.Discard, body); err != nil {
		return s3store.PutResult{}, fmt.Errorf("s3store: read body: %w", err)
	}
	return s3store.PutResult{}, errors.New("test: body fit the limit")
}

// putServer is a Server whose PUT lookup is stubbed: bucket "bkt" exists, the account stores `used` bytes and the
// key being written has `old` bytes (0 = new key).
func putServer(max, quota, used int64, old *int64) *Server {
	return &Server{
		cfg:   &Config{MaxObjectBytes: max, AccountQuotaBytes: quota},
		store: &drainStore{},
		putLookup: func(context.Context, string, string, string, int64) (putLookupResult, error) {
			return putLookupResult{bucketID: "test-bucket-id", oldSize: old, used: used}, nil
		},
	}
}

func TestPutObject_413(t *testing.T) {
	old30 := int64(30)
	cases := []struct {
		name          string
		s             *Server
		bodyLen       int
		chunked       bool // Content-Length unknown
		wantCode      int
		wantErrSubstr string
	}{
		{"content-length over max", putServer(10, 0, 0, nil), 11, false, 413, "object too large"},
		{"chunked over max", putServer(10, 0, 0, nil), 11, true, 413, "object too large"},
		{"content-length over quota room", putServer(1000, 100, 90, nil), 11, false, 413, "account quota exceeded"},
		{"chunked over quota room", putServer(1000, 100, 90, nil), 11, true, 413, "account quota exceeded"},
		{"overwrite frees room, chunked over it", putServer(1000, 100, 90, &old30), 41, true, 413, "account quota exceeded"},
		{"max binds before quota, chunked", putServer(10, 1000, 0, nil), 11, true, 413, "object too large"},
		{"content-length over default max", &Server{store: &drainStore{}}, 0, false, 413, "object too large"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodPut, "/storage/buckets/bkt/objects/k", strings.NewReader(strings.Repeat("x", c.bodyLen)))
		if c.chunked {
			req.ContentLength = -1
		}
		if c.name == "content-length over default max" {
			req.ContentLength = defaultMaxObjectBytes + 1
		}
		rr := httptest.NewRecorder()
		newRouter(c.s).ServeHTTP(rr, req)
		if rr.Code != c.wantCode || !strings.Contains(rr.Body.String(), c.wantErrSubstr) {
			t.Errorf("%s: %d %s", c.name, rr.Code, rr.Body.String())
		}
	}
}

func TestPutObject_overLimitBodyIsNotAnS3Error(t *testing.T) {
	// A body at exactly the limit is not a 413 (the stub store then reports its own sentinel error as 502).
	req := httptest.NewRequest(http.MethodPut, "/storage/buckets/bkt/objects/k", strings.NewReader(strings.Repeat("x", 10)))
	req.ContentLength = -1
	rr := httptest.NewRecorder()
	newRouter(putServer(10, 0, 0, nil)).ServeHTTP(rr, req)
	if rr.Code == http.StatusRequestEntityTooLarge {
		t.Fatalf("body of exactly max bytes answered 413: %s", rr.Body.String())
	}
}
