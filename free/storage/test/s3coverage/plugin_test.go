//go:build s3coverage

package s3coverage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"

	"github.com/nself-org/plugin-storage/internal"
	"github.com/nself-org/plugin-storage/internal/s3store"
)

// testInternalSecret is the fake PLUGIN_INTERNAL_SECRET the harness server expects in X-Plugin-Secret.
const testInternalSecret = "test-internal-secret"

// pluginHarness is the real internal.Server on the suite database and S3 endpoint, behind httptest.
type pluginHarness struct {
	url string
	rec *recorder // counts every request the plugin's own S3 client sends
}

// newStore builds the plugin's own S3 client for the endpoint under test; h (optional) sees every request.
func newStore(t *testing.T, h s3.HTTPClient) *s3store.Store {
	t.Helper()
	st, err := s3store.New(s3store.Config{
		Endpoint: env.baseURL, Region: env.region, AccessKey: env.access, SecretKey: env.secret, HTTPClient: h,
	})
	if err != nil {
		t.Fatalf("s3store.New: %v", err)
	}
	return st
}

func newPlugin(t *testing.T) *pluginHarness {
	t.Helper()
	p, _ := newPluginWrapped(t)
	return p
}

// failStore wraps the real store and fails chosen operations on demand (S3 failure injection).
type failStore struct {
	internal.ObjectStore
	failPut, failDelete, deleteNotFound, failDeleteObjects, failDeletePrefix atomic.Bool
	afterPut                                                                 atomic.Pointer[func()] // runs after a successful S3 write, before the handler writes the row
}

var errInjected = errors.New("injected S3 failure")

func (f *failStore) Put(c context.Context, bucket, key string, body io.Reader, ct string, declared int64) (s3store.PutResult, error) {
	if f.failPut.Load() {
		return s3store.PutResult{}, errInjected
	}
	res, err := f.ObjectStore.Put(c, bucket, key, body, ct, declared)
	if err == nil {
		if hook := f.afterPut.Load(); hook != nil {
			(*hook)()
		}
	}
	return res, err
}

func (f *failStore) Delete(c context.Context, bucket, key string) error {
	if f.failDelete.Load() {
		return errInjected
	}
	if f.deleteNotFound.Load() {
		return fmt.Errorf("%w: injected", s3store.ErrNotFound)
	}
	return f.ObjectStore.Delete(c, bucket, key)
}

func (f *failStore) DeleteObjects(c context.Context, bucket string, keys []string) error {
	if f.failDeleteObjects.Load() {
		return errInjected
	}
	return f.ObjectStore.DeleteObjects(c, bucket, keys)
}

func (f *failStore) DeletePrefix(c context.Context, bucket, prefix string) error {
	if f.failDeletePrefix.Load() {
		return errInjected
	}
	return f.ObjectStore.DeletePrefix(c, bucket, prefix)
}

// newPluginWrapped is newPlugin with a failure-injecting store in front of the real one.
func newPluginWrapped(t *testing.T) (*pluginHarness, *failStore) {
	t.Helper()
	rec := &recorder{}
	fs := &failStore{ObjectStore: newStore(t, rec)}
	r := chi.NewRouter()
	internal.NewServer(pool, &internal.Config{Port: "0", InternalSecret: testInternalSecret}, fs).Routes(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return &pluginHarness{url: ts.URL, rec: rec}, fs
}

// call performs a plugin API request as the given tenant.
func (p *pluginHarness) call(t *testing.T, tenant, method, path string, body io.Reader, ctype string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, p.url+path, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Plugin-Secret", testInternalSecret)
	if tenant != "" {
		req.Header.Set("X-Hasura-Source-Account-Id", tenant)
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, b
}

// createBucket creates a bucket through the plugin and registers S3-side cleanup.
func (p *pluginHarness) createBucket(t *testing.T, tenant string) string {
	t.Helper()
	name := "s3cov-" + randHex(5)
	t.Cleanup(func() { removeBucket(name) })
	code, _, body := p.call(t, tenant, http.MethodPost, "/storage/buckets", strings.NewReader(`{"name":"`+name+`"}`), "application/json")
	if code != http.StatusCreated {
		t.Fatalf("POST /storage/buckets: %d %s", code, body)
	}
	if _, err := direct.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(name)}); err != nil {
		t.Fatalf("plugin bucket missing on the S3 endpoint: %v", err)
	}
	return name
}

// hashReader hashes everything read through it.
type hashReader struct {
	r io.Reader
	h interface {
		Write([]byte) (int, error)
		Sum([]byte) []byte
	}
	n int64
}

func (h *hashReader) Read(p []byte) (int, error) {
	n, err := h.r.Read(p)
	_, _ = h.h.Write(p[:n])
	h.n += int64(n)
	return n, err
}

func TestPlugin_UploadLargeObjectViaS3(t *testing.T) {
	const size = 64 << 20
	p := newPlugin(t)
	bucket := p.createBucket(t, "")

	// Build the 64 MiB body from a seeded stream and hash it while it is sent.
	// Position-dependent bytes: every part differs, so reordered or duplicated parts change the SHA.
	src := &hashReader{h: sha256.New(), r: &patternStream{n: size}}
	code, _, body := p.call(t, "", http.MethodPut, "/storage/buckets/"+bucket+"/objects/big.bin", src, "application/x-big")
	if code != http.StatusOK {
		t.Fatalf("PUT 64 MiB: %d %s", code, body)
	}
	want := hex.EncodeToString(src.h.Sum(nil))
	if src.n != size {
		t.Fatalf("sent %d bytes", src.n)
	}
	var put struct {
		ID, Key, ETag string
		SizeBytes     int64 `json:"size_bytes"`
	}
	_ = json.Unmarshal(body, &put)
	if put.Key != "big.bin" || put.SizeBytes != size || put.ETag == "" || put.ID == "" {
		t.Fatalf("PUT response = %s", body)
	}

	// Multipart: exactly one CreateMultipartUpload, 4 UploadPart (16 MiB each), one Complete, no single PutObject.
	isPut := func(q reqRecord, wantPart bool) bool {
		return q.Method == http.MethodPut && strings.HasSuffix(q.Path, "/big.bin") && strings.Contains(q.Query, "partNumber=") == wantPart
	}
	if n := p.rec.count(func(q reqRecord) bool { return q.Method == http.MethodPost && strings.Contains(q.Query, "uploads") }); n != 1 {
		t.Errorf("CreateMultipartUpload requests = %d, want 1", n)
	}
	if n := p.rec.count(func(q reqRecord) bool { return isPut(q, true) }); n != 4 {
		t.Errorf("UploadPart requests = %d, want 4", n)
	}
	if n := p.rec.count(func(q reqRecord) bool { return isPut(q, false) }); n != 0 {
		t.Errorf("single PutObject requests = %d, want 0", n)
	}
	if n := p.rec.count(func(q reqRecord) bool { return q.Method == http.MethodPost && strings.Contains(q.Query, "uploadId=") }); n != 1 {
		t.Errorf("CompleteMultipartUpload requests = %d, want 1", n)
	}

	// The object is in S3 under the tenant-prefixed key and is not in Postgres.
	head, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String("primary/big.bin")})
	if err != nil || aws.ToInt64(head.ContentLength) != size {
		t.Fatalf("HeadObject primary/big.bin on S3: %v (len %d)", err, aws.ToInt64(head.ContentLength))
	}
	var rowSize int64
	var rowType string
	err = pool.QueryRow(ctx, `SELECT o.size_bytes, o.content_type FROM np_storage_objects o JOIN np_storage_buckets b ON b.id = o.bucket_id
		WHERE b.name = $1 AND o.key = 'big.bin'`, bucket).Scan(&rowSize, &rowType)
	if err != nil || rowSize != size || rowType != "application/x-big" {
		t.Fatalf("metadata row: size %d type %q err %v", rowSize, rowType, err)
	}
	var dbBytes int64
	err = pool.QueryRow(ctx, `SELECT pg_total_relation_size('public.np_storage_buckets') + pg_total_relation_size('public.np_storage_objects')
		+ pg_total_relation_size('public.np_storage_metadata')`).Scan(&dbBytes)
	if err != nil || dbBytes >= 1<<20 {
		t.Fatalf("storage tables hold %d bytes after a 64 MiB upload (err %v); object bytes must not be in Postgres", dbBytes, err)
	}

	// GET returns identical bytes with the right headers.
	req, err := http.NewRequest(http.MethodGet, p.url+"/storage/buckets/"+bucket+"/objects/big.bin", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Plugin-Secret", testInternalSecret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got := &hashReader{r: resp.Body, h: sha256.New()}
	_, _ = io.Copy(io.Discard, got)
	if resp.StatusCode != 200 || got.n != size || hex.EncodeToString(got.h.Sum(nil)) != want {
		t.Fatalf("GET: status %d, %d bytes, sha mismatch=%v", resp.StatusCode, got.n, hex.EncodeToString(got.h.Sum(nil)) != want)
	}
	if resp.Header.Get("Content-Type") != "application/x-big" || resp.Header.Get("Content-Length") != "67108864" {
		t.Errorf("GET headers: type %q length %q", resp.Header.Get("Content-Type"), resp.Header.Get("Content-Length"))
	}

	// DELETE removes it from S3 and Postgres.
	if code, _, _ := p.call(t, "", http.MethodDelete, "/storage/buckets/"+bucket+"/objects/big.bin", nil, ""); code != http.StatusNoContent {
		t.Fatalf("DELETE: %d", code)
	}
	if _, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String("primary/big.bin")}); !is404(err) {
		t.Fatalf("object still on S3 after DELETE: %v", err)
	}
	var left int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM np_storage_objects o JOIN np_storage_buckets b ON b.id = o.bucket_id WHERE b.name = $1`, bucket).Scan(&left)
	if left != 0 {
		t.Fatalf("%d metadata rows left after DELETE", left)
	}
	if code, _, _ := p.call(t, "", http.MethodGet, "/storage/buckets/"+bucket+"/objects/big.bin", nil, ""); code != http.StatusNotFound {
		t.Errorf("GET after DELETE: %d, want 404", code)
	}
}

func TestPlugin_SmallObjectAndMetadata(t *testing.T) {
	p := newPlugin(t)
	bucket := p.createBucket(t, "")
	data := payload(1234, 12)
	code, _, body := p.call(t, "", http.MethodPut, "/storage/buckets/"+bucket+"/objects/small.txt", bytes.NewReader(data), "text/plain")
	if code != 200 {
		t.Fatalf("PUT: %d %s", code, body)
	}
	if n := p.rec.count(func(q reqRecord) bool {
		return strings.Contains(q.Query, "uploads") || strings.Contains(q.Query, "partNumber")
	}); n != 0 {
		t.Errorf("small upload used multipart (%d requests)", n)
	}
	_, hdr, got := p.call(t, "", http.MethodGet, "/storage/buckets/"+bucket+"/objects/small.txt", nil, "")
	if !bytes.Equal(got, data) || hdr.Get("Content-Type") != "text/plain" || hdr.Get("ETag") == "" {
		t.Fatalf("GET bytes equal=%v type %q etag %q", bytes.Equal(got, data), hdr.Get("Content-Type"), hdr.Get("ETag"))
	}
	if hdr.Get("X-Content-Type-Options") != "nosniff" || hdr.Get("Content-Security-Policy") != "sandbox" {
		t.Errorf("download hardening headers: nosniff %q, CSP %q", hdr.Get("X-Content-Type-Options"), hdr.Get("Content-Security-Policy"))
	}
	// ?metadata=true keeps the pre-S3 JSON shape.
	_, _, meta := p.call(t, "", http.MethodGet, "/storage/buckets/"+bucket+"/objects/small.txt?metadata=true", nil, "")
	var row struct {
		Key       string `json:"key"`
		SizeBytes int64  `json:"size_bytes"`
		Etag      string `json:"etag"`
	}
	if err := json.Unmarshal(meta, &row); err != nil || row.Key != "small.txt" || row.SizeBytes != 1234 || row.Etag == "" {
		t.Fatalf("metadata=true response: %s (%v)", meta, err)
	}
	// Overwrite updates size and bytes.
	p.call(t, "", http.MethodPut, "/storage/buckets/"+bucket+"/objects/small.txt", strings.NewReader("short"), "text/plain")
	_, _, got = p.call(t, "", http.MethodGet, "/storage/buckets/"+bucket+"/objects/small.txt", nil, "")
	if string(got) != "short" {
		t.Fatalf("overwrite not visible: %q", got)
	}
}

func TestPlugin_TenantIsolationInSharedBucket(t *testing.T) {
	p := newPlugin(t)
	bucket := p.createBucket(t, "tenant-a")
	if code, _, body := p.call(t, "tenant-b", http.MethodPost, "/storage/buckets", strings.NewReader(`{"name":"`+bucket+`"}`), "application/json"); code != http.StatusCreated {
		t.Fatalf("second tenant creating the same S3 bucket: %d %s", code, body)
	}
	path := "/storage/buckets/" + bucket + "/objects/same.txt"
	p.call(t, "tenant-a", http.MethodPut, path, strings.NewReader("from a"), "text/plain")
	p.call(t, "tenant-b", http.MethodPut, path, strings.NewReader("from b"), "text/plain")
	if _, _, b := p.call(t, "tenant-a", http.MethodGet, path, nil, ""); string(b) != "from a" {
		t.Errorf("tenant-a reads %q", b)
	}
	if _, _, b := p.call(t, "tenant-b", http.MethodGet, path, nil, ""); string(b) != "from b" {
		t.Errorf("tenant-b reads %q", b)
	}
	for _, k := range []string{"tenant-a/same.txt", "tenant-b/same.txt"} {
		if _, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(k)}); err != nil {
			t.Errorf("physical key %s missing: %v", k, err)
		}
	}
	// Objects without a metadata row (a failed upsert, a racing PUT) sit under the tenant prefix: the bucket delete
	// must sweep them too, but not another tenant's objects (including an id that merely starts the same way).
	for _, k := range []string{"tenant-a/untracked.txt", "tenant-a/deep/er/untracked.txt", "tenant-b/untracked.txt", "tenant-ab/keep.txt"} {
		putBytes(t, bucket, k, []byte("planted"))
	}
	// Deleting tenant-a's bucket removes only tenant-a's objects and never the physical bucket.
	if code, _, _ := p.call(t, "tenant-a", http.MethodDelete, "/storage/buckets/"+bucket, nil, ""); code != http.StatusNoContent {
		t.Fatalf("DELETE bucket: %d", code)
	}
	if _, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String("tenant-a/same.txt")}); !is404(err) {
		t.Errorf("tenant-a object survived its bucket delete: %v", err)
	}
	for _, k := range []string{"tenant-a/untracked.txt", "tenant-a/deep/er/untracked.txt"} {
		if _, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(k)}); !is404(err) {
			t.Errorf("untracked tenant-a object %s survived the bucket delete: %v", k, err)
		}
	}
	for _, k := range []string{"tenant-b/untracked.txt", "tenant-ab/keep.txt", "tenant-b/same.txt"} {
		if _, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(k)}); err != nil {
			t.Errorf("another tenant's object %s was removed by tenant-a's bucket delete: %v", k, err)
		}
	}
	if _, _, b := p.call(t, "tenant-b", http.MethodGet, path, nil, ""); string(b) != "from b" {
		t.Errorf("tenant-b lost its object: %q", b)
	}
	if _, err := direct.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Errorf("physical bucket was deleted: %v", err)
	}
	if code, _, _ := p.call(t, "tenant-b", http.MethodDelete, "/storage/buckets/"+bucket, nil, ""); code != http.StatusNoContent {
		t.Errorf("tenant-b DELETE bucket: %d", code)
	}
}

// rowCount returns how many metadata rows exist for bucket/key (key "" counts the bucket rows instead).
func rowCount(t *testing.T, bucket, key string) int {
	t.Helper()
	var n int
	var err error
	if key == "" {
		err = pool.QueryRow(ctx, `SELECT count(*) FROM np_storage_buckets WHERE name = $1`, bucket).Scan(&n)
	} else {
		err = pool.QueryRow(ctx, `SELECT count(*) FROM np_storage_objects o JOIN np_storage_buckets b ON b.id = o.bucket_id
			WHERE b.name = $1 AND o.key = $2`, bucket, key).Scan(&n)
	}
	if err != nil {
		t.Fatalf("rowCount: %v", err)
	}
	return n
}

// The S3 call always comes first: a failed S3 write leaves no row, a failed S3 delete keeps the row.
func TestPlugin_S3FailureKeepsDatabaseConsistent(t *testing.T) {
	p, fs := newPluginWrapped(t)
	bucket := p.createBucket(t, "")
	obj := "/storage/buckets/" + bucket + "/objects/"

	// Put failure -> 502 and no row.
	fs.failPut.Store(true)
	if code, _, _ := p.call(t, "", http.MethodPut, obj+"a.txt", strings.NewReader("data"), "text/plain"); code != http.StatusBadGateway {
		t.Fatalf("PUT with S3 failing: %d, want 502", code)
	}
	if n := rowCount(t, bucket, "a.txt"); n != 0 {
		t.Fatalf("%d row(s) after a failed S3 write", n)
	}
	fs.failPut.Store(false)
	for _, k := range []string{"a.txt", "b.txt"} {
		if code, _, body := p.call(t, "", http.MethodPut, obj+k, strings.NewReader("data"), "text/plain"); code != 200 {
			t.Fatalf("PUT %s: %d %s", k, code, body)
		}
	}

	// Delete failure -> 502, row and S3 object kept.
	fs.failDelete.Store(true)
	if code, _, _ := p.call(t, "", http.MethodDelete, obj+"a.txt", nil, ""); code != http.StatusBadGateway {
		t.Fatalf("DELETE with S3 failing: %d, want 502", code)
	}
	if n := rowCount(t, bucket, "a.txt"); n != 1 {
		t.Fatalf("row count after a failed S3 delete = %d, want 1", n)
	}
	if _, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String("primary/a.txt")}); err != nil {
		t.Fatalf("S3 object vanished although the delete failed: %v", err)
	}
	fs.failDelete.Store(false)

	// S3 says the key is already gone -> counts as deleted: 204 and the row goes.
	fs.deleteNotFound.Store(true)
	if code, _, _ := p.call(t, "", http.MethodDelete, obj+"b.txt", nil, ""); code != http.StatusNoContent {
		t.Fatalf("DELETE with S3 NotFound: %d, want 204", code)
	}
	if n := rowCount(t, bucket, "b.txt"); n != 0 {
		t.Fatalf("row kept after S3 reported the key gone: %d", n)
	}
	fs.deleteNotFound.Store(false)

	// Bucket delete with DeleteObjects failing -> 502, bucket row and object rows kept.
	fs.failDeleteObjects.Store(true)
	if code, _, _ := p.call(t, "", http.MethodDelete, "/storage/buckets/"+bucket, nil, ""); code != http.StatusBadGateway {
		t.Fatalf("DELETE bucket with S3 failing: %d, want 502", code)
	}
	if rowCount(t, bucket, "") != 1 || rowCount(t, bucket, "a.txt") != 1 {
		t.Fatalf("rows lost although DeleteObjects failed (bucket %d, object %d)", rowCount(t, bucket, ""), rowCount(t, bucket, "a.txt"))
	}
	fs.failDeleteObjects.Store(false)

	// The prefix sweep failing also keeps every row.
	fs.failDeletePrefix.Store(true)
	if code, _, _ := p.call(t, "", http.MethodDelete, "/storage/buckets/"+bucket, nil, ""); code != http.StatusBadGateway {
		t.Fatalf("DELETE bucket with the prefix sweep failing: %d, want 502", code)
	}
	if rowCount(t, bucket, "") != 1 || rowCount(t, bucket, "a.txt") != 1 {
		t.Fatal("rows lost although the prefix sweep failed")
	}
	fs.failDeletePrefix.Store(false)
	if code, _, _ := p.call(t, "", http.MethodDelete, "/storage/buckets/"+bucket, nil, ""); code != http.StatusNoContent {
		t.Fatalf("DELETE bucket after recovery: %d", code)
	}
	if rowCount(t, bucket, "") != 0 || rowCount(t, bucket, "a.txt") != 0 {
		t.Fatal("rows left after a successful bucket delete")
	}
}

func TestPlugin_RejectsInvalidAccountID(t *testing.T) {
	p := newPlugin(t)
	for _, id := range []string{"..", "."} {
		if code, _, body := p.call(t, id, http.MethodGet, "/storage/buckets", nil, ""); code != http.StatusBadRequest || !strings.Contains(string(body), "invalid source account id") {
			t.Errorf("account %q: %d %s", id, code, body)
		}
	}
}

// A client that declares 40 MiB, sends 20 MiB and hangs up must leave nothing behind: no object, no row, and no
// multipart upload left open on the S3 side.
func TestPlugin_TruncatedUploadCommitsNothing(t *testing.T) {
	p := newPlugin(t)
	bucket := p.createBucket(t, "")
	conn, err := net.Dial("tcp", strings.TrimPrefix(p.url, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(conn, "PUT /storage/buckets/%s/objects/trunc.bin HTTP/1.1\r\nHost: plugin\r\nX-Plugin-Secret: %s\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n", bucket, testInternalSecret, 40<<20)
	if _, err := io.Copy(conn, &patternStream{n: 20 << 20}); err != nil {
		t.Fatalf("sending the partial body: %v", err)
	}
	_ = conn.Close()

	deadline := time.Now().Add(30 * time.Second)
	for {
		mu, err := direct.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{Bucket: aws.String(bucket)})
		if err == nil && len(mu.Uploads) == 0 && p.rec.count(func(q reqRecord) bool { return strings.Contains(q.Query, "uploads") }) >= 1 {
			break // the plugin started the multipart upload and aborted it
		}
		if time.Now().After(deadline) {
			t.Fatalf("multipart upload still open or never started (err %v)", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if _, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String("primary/trunc.bin")}); !is404(err) {
		t.Fatalf("a truncated upload produced an object: %v", err)
	}
	if n := rowCount(t, bucket, "trunc.bin"); n != 0 {
		t.Fatalf("%d metadata row(s) for a truncated upload", n)
	}
	if n := p.rec.count(func(q reqRecord) bool { return q.Method == http.MethodPost && strings.Contains(q.Query, "uploadId=") }); n != 0 {
		t.Fatalf("CompleteMultipartUpload was sent %d time(s) for a truncated body", n)
	}
}

// If the metadata row cannot be written after the S3 write, a NEW key's bytes are removed again (no orphan), while
// an overwrite keeps its bytes.
func TestPlugin_OrphanCleanupWhenMetadataWriteFails(t *testing.T) {
	p, fs := newPluginWrapped(t)
	dropBucketRow := func(bucket string) *func() {
		f := func() {
			if _, err := pool.Exec(ctx, `DELETE FROM np_storage_buckets WHERE name = $1`, bucket); err != nil {
				t.Errorf("dropping the bucket row: %v", err)
			}
		}
		return &f
	}

	b1 := p.createBucket(t, "")
	fs.afterPut.Store(dropBucketRow(b1))
	if code, _, _ := p.call(t, "", http.MethodPut, "/storage/buckets/"+b1+"/objects/orphan.txt", strings.NewReader("new"), "text/plain"); code != http.StatusInternalServerError {
		t.Fatalf("PUT with the row write failing: %d, want 500", code)
	}
	if _, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(b1), Key: aws.String("primary/orphan.txt")}); !is404(err) {
		t.Fatalf("orphan bytes of a new key were left in S3: %v", err)
	}

	fs.afterPut.Store(nil)
	b2 := p.createBucket(t, "")
	if code, _, _ := p.call(t, "", http.MethodPut, "/storage/buckets/"+b2+"/objects/keep.txt", strings.NewReader("old"), "text/plain"); code != 200 {
		t.Fatalf("first PUT: %d", code)
	}
	fs.afterPut.Store(dropBucketRow(b2))
	if code, _, _ := p.call(t, "", http.MethodPut, "/storage/buckets/"+b2+"/objects/keep.txt", strings.NewReader("overwritten"), "text/plain"); code != http.StatusInternalServerError {
		t.Fatalf("overwrite with the row write failing: %d, want 500", code)
	}
	if got := getBytes(t, b2, "primary/keep.txt"); string(got) != "overwritten" {
		t.Fatalf("an overwrite must keep its bytes, S3 holds %q", got)
	}
}

// "/" inside a key travels as %2F and the key is decoded exactly once.
func TestPlugin_KeyEncoding(t *testing.T) {
	p := newPlugin(t)
	bucket := p.createBucket(t, "")
	base := "/storage/buckets/" + bucket + "/objects/"
	p.call(t, "", http.MethodPut, base+"a%2Fb", strings.NewReader("slash"), "text/plain")
	p.call(t, "", http.MethodPut, base+"a%252Fb", strings.NewReader("percent"), "text/plain")
	if _, _, b := p.call(t, "", http.MethodGet, base+"a%2Fb", nil, ""); string(b) != "slash" {
		t.Errorf("a%%2Fb reads %q", b)
	}
	if _, _, b := p.call(t, "", http.MethodGet, base+"a%252Fb", nil, ""); string(b) != "percent" {
		t.Errorf("a%%252Fb reads %q", b)
	}
	for _, k := range []string{"primary/a/b", "primary/a%2Fb"} {
		if _, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(k)}); err != nil {
			t.Errorf("physical key %q missing: %v", k, err)
		}
	}
	if code, _, _ := p.call(t, "", http.MethodPut, base+"x%0Ay", strings.NewReader("no"), "text/plain"); code != http.StatusBadRequest {
		t.Errorf("key with a newline: %d, want 400", code)
	}
}

// An account that never registered a bucket must not be able to sweep its prefix in it: DELETE is a no-op (204)
// and objects planted under that account's prefix stay.
func TestPlugin_DeleteUnregisteredBucketTouchesNothing(t *testing.T) {
	p := newPlugin(t)
	bucket := p.createBucket(t, "owner")
	putBytes(t, bucket, "ghost/planted.txt", []byte("planted"))
	putBytes(t, bucket, "owner/own.txt", []byte("own"))
	if code, _, body := p.call(t, "ghost", http.MethodDelete, "/storage/buckets/"+bucket, nil, ""); code != http.StatusNoContent {
		t.Fatalf("DELETE of an unregistered bucket: %d %s, want 204", code, body)
	}
	for _, k := range []string{"ghost/planted.txt", "owner/own.txt"} {
		if _, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(k)}); err != nil {
			t.Errorf("object %s was removed by an account that never registered the bucket: %v", k, err)
		}
	}
	if rowCount(t, bucket, "") != 1 {
		t.Error("the owner's bucket row changed")
	}
}

// The Content-Type stored with the bytes in S3 wins over the row, which can be stale after racing PUTs.
func TestPlugin_DownloadContentTypeComesFromS3(t *testing.T) {
	p := newPlugin(t)
	bucket := p.createBucket(t, "")
	if code, _, _ := p.call(t, "", http.MethodPut, "/storage/buckets/"+bucket+"/objects/t.txt", strings.NewReader("hi"), "text/plain"); code != 200 {
		t.Fatalf("PUT: %d", code)
	}
	if _, err := pool.Exec(ctx, `UPDATE np_storage_objects o SET content_type = 'application/x-stale' FROM np_storage_buckets b
		WHERE b.id = o.bucket_id AND b.name = $1 AND o.key = 't.txt'`, bucket); err != nil {
		t.Fatal(err)
	}
	_, hdr, body := p.call(t, "", http.MethodGet, "/storage/buckets/"+bucket+"/objects/t.txt", nil, "")
	if string(body) != "hi" || hdr.Get("Content-Type") != "text/plain" {
		t.Fatalf("GET with a stale row: body %q type %q, want the S3 type text/plain", body, hdr.Get("Content-Type"))
	}
}
