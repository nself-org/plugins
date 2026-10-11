package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/nself-org/plugin-storage/internal/s3store"
)

// fakeStore is an ObjectStore that fails EnsureBucket on demand; every other method panics if reached.
type fakeStore struct {
	ObjectStore
	ensureErr error
	ensured   []string
}

func (f *fakeStore) EnsureBucket(_ context.Context, b string) error {
	f.ensured = append(f.ensured, b)
	return f.ensureErr
}
func (f *fakeStore) Put(context.Context, string, string, io.Reader, string, int64) (s3store.PutResult, error) {
	return s3store.PutResult{}, errors.New("unexpected Put")
}

func newRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	s.Routes(r)
	return r
}

func do(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestObjectKey_tenantIsolation(t *testing.T) {
	if got := objectKey("a", "b/c"); got != "a/b/c" {
		t.Errorf("objectKey = %q", got)
	}
	// tenant "a/b" with key "c" must not equal tenant "a" with key "b/c"
	if objectKey("a/b", "c") == objectKey("a", "b/c") {
		t.Fatal("tenant a/b collides with tenant a")
	}
	if got := objectKey("a/b", "c"); got != "a%2Fb/c" {
		t.Errorf("objectKey(a/b) = %q", got)
	}
	if !strings.HasPrefix(objectKey("a", "x"), "a/") || strings.HasPrefix(objectKey("a/b", "x"), "a/") {
		t.Error("prefix rule broken")
	}
}

func TestValidBucketName(t *testing.T) {
	good := []string{"abc", "my-bucket", "a.b.c", "s3cov-0123abcd", strings.Repeat("a", 63)}
	bad := []string{"", "ab", strings.Repeat("a", 64), "-abc", "abc-", ".abc", "abc.", "a..b", "ABC", "a_b", "192.168.1.1", "a b", "ab/c"}
	for _, n := range good {
		if !validBucketName(n) {
			t.Errorf("%q should be valid", n)
		}
	}
	for _, n := range bad {
		if validBucketName(n) {
			t.Errorf("%q should be invalid", n)
		}
	}
}

func TestValidateKey_limits(t *testing.T) {
	if validateKey("") != ErrInvalidKey {
		t.Error("empty key must be rejected")
	}
	if validateKey(strings.Repeat("k", 1024)) != nil {
		t.Error("1024-byte key is valid")
	}
	if validateKey(strings.Repeat("k", 1025)) != ErrInvalidKey {
		t.Error("1025-byte key must be rejected")
	}
}

func TestCreateBucket_validationAndStoreStates(t *testing.T) {
	cases := []struct {
		name  string
		store ObjectStore
		body  string
		code  int
		want  string
	}{
		{"bad json", nil, `{`, 400, "invalid request body"},
		{"bad name", &fakeStore{}, `{"name":"Bad_Name"}`, 400, "invalid bucket name"},
		{"no store", nil, `{"name":"good-name"}`, 503, "object storage not configured"},
		{"s3 failure", &fakeStore{ensureErr: errors.New("down")}, `{"name":"good-name"}`, 502, "object storage error"},
	}
	for _, c := range cases {
		s := &Server{store: c.store}
		rr := do(newRouter(s), http.MethodPost, "/storage/buckets", c.body)
		if rr.Code != c.code || !strings.Contains(rr.Body.String(), c.want) {
			t.Errorf("%s: got %d %q", c.name, rr.Code, rr.Body.String())
		}
	}
}

func TestObjectRoutes_503WithoutStore(t *testing.T) {
	h := newRouter(&Server{})
	if rr := do(h, http.MethodPut, "/storage/buckets/bkt/objects/k", "x"); rr.Code != 503 {
		t.Errorf("PUT: %d", rr.Code)
	}
	if rr := do(h, http.MethodGet, "/storage/buckets/bkt/objects/k", ""); rr.Code != 503 {
		t.Errorf("GET: %d", rr.Code)
	}
}

func TestObjectRoutes_invalidKey(t *testing.T) {
	h := newRouter(&Server{})
	for _, m := range []string{http.MethodPut, http.MethodGet, http.MethodDelete} {
		rr := do(h, m, "/storage/buckets/bkt/objects/"+strings.Repeat("k", 1025), "")
		if rr.Code != 400 {
			t.Errorf("%s long key: %d", m, rr.Code)
		}
	}
}

func TestValidAccountID(t *testing.T) {
	for _, id := range []string{"primary", "tenant-a", "a/b", "ümlaut", "x.y", "..a", "a.."} {
		if !validAccountID(id) {
			t.Errorf("%q should be valid", id)
		}
	}
	for _, id := range []string{"", " ", "\t", ".", "..", "a\nb", "a\x00b", "a\x7fb", "\x1f"} {
		if validAccountID(id) {
			t.Errorf("%q should be invalid", id)
		}
	}
}

func TestRoutes_rejectInvalidAccountHeader(t *testing.T) {
	h := newRouter(&Server{store: &fakeStore{}})
	for _, id := range []string{"..", ".", "  ", "a\x01b"} {
		for _, target := range []struct{ method, path string }{
			{http.MethodPost, "/storage/buckets"},
			{http.MethodPut, "/storage/buckets/bkt/objects/k"},
			{http.MethodGet, "/storage/buckets/bkt/objects/k"},
			{http.MethodDelete, "/storage/buckets/bkt/objects/k"},
		} {
			req := httptest.NewRequest(target.method, target.path, strings.NewReader(`{"name":"good-name"}`))
			req.Header["X-Hasura-Source-Account-Id"] = []string{id}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "invalid source account id") {
				t.Errorf("%s %s with id %q: %d %s", target.method, target.path, id, rr.Code, rr.Body.String())
			}
		}
	}
}

func TestObjectKey_dotAccountsAreRejectedNotEscaped(t *testing.T) {
	// "." and ".." survive url.PathEscape unchanged, which is why validAccountID must refuse them.
	if objectKey("..", "k") != "../k" {
		t.Fatal("test premise changed: PathEscape now escapes dots")
	}
	if validAccountID("..") {
		t.Fatal("..  must be refused as an account id")
	}
}

func TestValidateKey_dotKeys(t *testing.T) {
	for _, k := range []string{".", "a/.", "dir/sub/."} {
		if validateKey(k) != ErrInvalidKey {
			t.Errorf("key %q must be rejected", k)
		}
	}
	for _, k := range []string{".hidden", "a/.hidden", "a.b", "a/b."} {
		if validateKey(k) != nil {
			t.Errorf("key %q must be accepted", k)
		}
	}
}

func TestSetObjectHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	setObjectHeaders(rr, "text/html", 12, `"abc"`)
	h := rr.Header()
	if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Content-Security-Policy") != "sandbox" ||
		h.Get("Content-Type") != "text/html" || h.Get("Content-Length") != "12" || h.Get("ETag") != `"abc"` {
		t.Fatalf("headers = %v", h)
	}
	rr = httptest.NewRecorder()
	setObjectHeaders(rr, "text/plain", -1, "")
	if rr.Header().Get("Content-Length") != "" || rr.Header().Get("ETag") != "" {
		t.Fatalf("unknown size / etag must be omitted: %v", rr.Header())
	}
}

func TestWritePutError(t *testing.T) {
	cases := []struct {
		err   error
		code  int
		body  string
		retry bool
	}{
		{fmt.Errorf("wait: %w", s3store.ErrBusy), 503, "upload capacity reached, retry", true},
		{s3store.ErrBodyTruncated, 400, "request body incomplete", false},
		{fmt.Errorf("x: %w", s3store.ErrLengthMismatch), 400, "request body incomplete", false},
		{os.ErrDeadlineExceeded, 408, "request body stalled", false},
		{errors.New("s3 down"), 502, "object storage error", false},
	}
	for _, c := range cases {
		rr := httptest.NewRecorder()
		writePutError(rr, c.err)
		if rr.Code != c.code || !strings.Contains(rr.Body.String(), c.body) || (rr.Header().Get("Retry-After") != "") != c.retry {
			t.Errorf("%v: %d %q retry-after %q", c.err, rr.Code, rr.Body.String(), rr.Header().Get("Retry-After"))
		}
	}
}

func TestValidContentType(t *testing.T) {
	for _, ct := range []string{"text/plain", "application/json; charset=utf-8", "a\tb"} {
		if !validContentType(ct) {
			t.Errorf("%q should be valid", ct)
		}
	}
	for _, ct := range []string{"text/\u00e9", "a\x00b", "a\nb", "a\x7fb", "caf\xe9"} {
		if validContentType(ct) {
			t.Errorf("%q should be invalid", ct)
		}
	}
}

func TestPutObject_invalidContentTypeIs400BeforeS3(t *testing.T) {
	h := newRouter(&Server{store: &fakeStore{}})
	req := httptest.NewRequest(http.MethodPut, "/storage/buckets/bkt/objects/k", strings.NewReader("x"))
	req.Header["Content-Type"] = []string{"text/caf\u00e9"}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 400 || !strings.Contains(rr.Body.String(), "invalid content type") {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
}

func TestValidateObject_physicalKeyLength(t *testing.T) {
	if validateObject("primary", strings.Repeat("k", 1000)) != nil {
		t.Error("short account + 1000-byte key fits")
	}
	if validateObject(strings.Repeat("a", 100), strings.Repeat("k", 1000)) != ErrInvalidKey {
		t.Error("physical key over 1024 bytes must be rejected")
	}
	// escaping grows the id: 10 slashes become 10 x "%2F"
	if validateObject(strings.Repeat("/", 400), "k") != ErrInvalidKey {
		t.Error("escaped account id must count at its escaped length")
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/storage/buckets/bkt/objects/"+strings.Repeat("k", 900), nil)
	req.Header.Set("X-Hasura-Source-Account-Id", strings.Repeat("t", 200))
	newRouter(&Server{store: &fakeStore{}}).ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Errorf("long account + key over HTTP: %d", rr.Code)
	}
}

type fakeDeadline struct{ calls int }

func (f *fakeDeadline) set(time.Time) error { f.calls++; return nil }

func TestIdleReader_extendsAfterReadsWithData(t *testing.T) {
	d := &fakeDeadline{}
	ir := &idleReader{r: strings.NewReader("abcdef"), idle: time.Minute, setDeadline: d.set}
	buf := make([]byte, 2)
	for i := 0; i < 3; i++ {
		if _, err := ir.Read(buf); err != nil {
			t.Fatal(err)
		}
	}
	if d.calls != 3 {
		t.Fatalf("deadline extended %d times, want 3", d.calls)
	}
	if _, err := ir.Read(buf); err != io.EOF || d.calls != 3 {
		t.Fatalf("EOF read: err %v calls %d", err, d.calls)
	}
}

func TestNewIdleReader_unsupportedWriterIsIgnored(t *testing.T) {
	ir := newIdleReader(httptest.NewRecorder(), strings.NewReader("x"), time.Second)
	b, _ := io.ReadAll(ir)
	if string(b) != "x" {
		t.Fatal("body lost")
	}
}

type failingBody struct{ sent bool }

func (f *failingBody) Read(p []byte) (int, error) {
	if !f.sent {
		f.sent = true
		return copy(p, "partial"), nil
	}
	return 0, errors.New("backend stream broke")
}

func TestStreamBody_abortsResponseOnCopyError(t *testing.T) {
	defer func() {
		if r := recover(); r != http.ErrAbortHandler {
			t.Fatalf("recovered %v, want http.ErrAbortHandler", r)
		}
	}()
	streamBody(io.Discard, &failingBody{}, "test")
	t.Fatal("streamBody returned after a copy error")
}

func TestStreamBody_okCopies(t *testing.T) {
	var sb strings.Builder
	streamBody(&sb, strings.NewReader("hello"), "test")
	if sb.String() != "hello" {
		t.Fatal(sb.String())
	}
}
