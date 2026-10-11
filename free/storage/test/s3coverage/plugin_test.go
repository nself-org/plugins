//go:build s3coverage

package s3coverage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"

	"github.com/nself-org/plugin-storage/internal"
	"github.com/nself-org/plugin-storage/internal/s3store"
)

// pluginHarness is the real internal.Server on the suite database and S3 endpoint, behind httptest.
type pluginHarness struct {
	url string
	rec *recorder // counts every request the plugin's own S3 client sends
}

func newPlugin(t *testing.T) *pluginHarness {
	t.Helper()
	rec := &recorder{}
	st, err := s3store.New(s3store.Config{
		Endpoint: env.baseURL, Region: env.region, AccessKey: env.access, SecretKey: env.secret, HTTPClient: rec,
	})
	if err != nil {
		t.Fatalf("s3store.New: %v", err)
	}
	r := chi.NewRouter()
	internal.NewServer(pool, &internal.Config{Port: "0"}, st).Routes(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return &pluginHarness{url: ts.URL, rec: rec}
}

// call performs a plugin API request as the given tenant.
func (p *pluginHarness) call(t *testing.T, tenant, method, path string, body io.Reader, ctype string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, p.url+path, body)
	if err != nil {
		t.Fatal(err)
	}
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
	chunk := payload(8<<20, 11)
	src := &hashReader{h: sha256.New()}
	src.r = io.MultiReader(bytes.NewReader(chunk), bytes.NewReader(chunk), bytes.NewReader(chunk), bytes.NewReader(chunk),
		bytes.NewReader(chunk), bytes.NewReader(chunk), bytes.NewReader(chunk), bytes.NewReader(chunk))
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
	resp, err := http.Get(p.url + "/storage/buckets/" + bucket + "/objects/big.bin")
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
	// Deleting tenant-a's bucket removes only tenant-a's objects and never the physical bucket.
	if code, _, _ := p.call(t, "tenant-a", http.MethodDelete, "/storage/buckets/"+bucket, nil, ""); code != http.StatusNoContent {
		t.Fatalf("DELETE bucket: %d", code)
	}
	if _, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String("tenant-a/same.txt")}); !is404(err) {
		t.Errorf("tenant-a object survived its bucket delete: %v", err)
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
