//go:build s3coverage

package s3coverage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	mrand "math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nself-org/plugin-storage/internal"
)

// env holds the suite configuration read once in TestMain.
var env struct {
	endpoint, access, secret, region, dbURL string
	baseURL                                 string // endpoint with scheme, no trailing slash
}

var (
	direct *s3.Client    // client used by TestOp_* (records requests in directRec)
	dirRec = &recorder{} // every request of the direct client
	pool   *pgxpool.Pool
)

// reqRecord is one observed HTTP request.
type reqRecord struct{ Method, Host, Path, Query string }

// recorder is an http.RoundTripper wrapper that remembers requests (the transport-level request counter).
type recorder struct {
	mu   sync.Mutex
	reqs []reqRecord
}

// Do implements s3.HTTPClient.
func (r *recorder) Do(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.reqs = append(r.reqs, reqRecord{req.Method, req.URL.Host, req.URL.Path, req.URL.RawQuery})
	r.mu.Unlock()
	return http.DefaultClient.Do(req)
}

// snapshot returns a copy of the requests seen so far.
func (r *recorder) snapshot() []reqRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]reqRecord(nil), r.reqs...)
}

// count returns how many recorded requests satisfy pred.
func (r *recorder) count(pred func(reqRecord) bool) int {
	n := 0
	for _, q := range r.snapshot() {
		if pred(q) {
			n++
		}
	}
	return n
}

// newClient builds a path-style SigV4 client against the endpoint under test.
func newClient(h s3.HTTPClient) *s3.Client {
	return s3.New(s3.Options{
		BaseEndpoint:               aws.String(env.baseURL),
		Region:                     env.region,
		UsePathStyle:               true,
		Credentials:                credentials.NewStaticCredentialsProvider(env.access, env.secret, ""),
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
		HTTPClient:                 h,
	})
}

func mustEnv(name string) string {
	v := os.Getenv(name)
	if v == "" {
		fmt.Fprintf(os.Stderr, "s3coverage: %s is required\n", name)
		os.Exit(1)
	}
	return v
}

func TestMain(m *testing.M) {
	env.endpoint = mustEnv("S3COV_ENDPOINT")
	env.access = mustEnv("S3COV_ACCESS_KEY")
	env.secret = mustEnv("S3COV_SECRET_KEY")
	env.dbURL = mustEnv("S3COV_DB_URL")
	env.region = os.Getenv("S3COV_REGION")
	if env.region == "" {
		env.region = "us-east-1"
	}
	env.baseURL = strings.TrimRight(env.endpoint, "/")
	if !strings.Contains(env.baseURL, "://") {
		env.baseURL = "http://" + env.baseURL
	}
	direct = newClient(dirRec)

	ctx := context.Background()
	deadline := time.Now().Add(90 * time.Second)
	for {
		_, err := direct.ListBuckets(ctx, &s3.ListBucketsInput{})
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(os.Stderr, "s3coverage: S3 endpoint not ready after 90s: %v\n", err)
			os.Exit(1)
		}
		time.Sleep(time.Second)
	}
	var err error
	for {
		if pool, err = pgxpool.New(ctx, env.dbURL); err == nil {
			if err = pool.Ping(ctx); err == nil {
				break
			}
			pool.Close()
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(os.Stderr, "s3coverage: database not ready after 90s: %v\n", err)
			os.Exit(1)
		}
		time.Sleep(time.Second)
	}
	internal.BootMigrations(ctx, pool)
	code := m.Run()
	pool.Close()
	log.Printf("s3coverage: finished")
	os.Exit(code)
}

// randHex returns n random bytes as hex.
func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// newBucket creates a random bucket s3cov-<hex> and registers its cleanup (objects, versions, uploads, bucket).
func newBucket(t *testing.T) string {
	t.Helper()
	name := "s3cov-" + randHex(5)
	if _, err := direct.CreateBucket(context.Background(), &s3.CreateBucketInput{Bucket: aws.String(name)}); err != nil {
		t.Fatalf("CreateBucket %s: %v", name, err)
	}
	t.Cleanup(func() { removeBucket(name) })
	return name
}

// removeBucket empties and deletes a bucket on a best-effort basis (errors are logged, not fatal).
func removeBucket(name string) {
	ctx := context.Background()
	vo, err := direct.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: aws.String(name)})
	if err == nil {
		for _, v := range vo.Versions {
			_, _ = direct.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(name), Key: v.Key, VersionId: v.VersionId})
		}
		for _, v := range vo.DeleteMarkers {
			_, _ = direct.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(name), Key: v.Key, VersionId: v.VersionId})
		}
	}
	lo, err := direct.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(name)})
	if err == nil {
		for _, o := range lo.Contents {
			_, _ = direct.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(name), Key: o.Key})
		}
	}
	mo, err := direct.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{Bucket: aws.String(name)})
	if err == nil {
		for _, u := range mo.Uploads {
			_, _ = direct.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(name), Key: u.Key, UploadId: u.UploadId})
		}
	}
	if _, err := direct.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)}); err != nil {
		log.Printf("s3coverage: cleanup of bucket %s: %v", name, err)
	}
}

// putBytes uploads data with PutObject and fails the test on error.
func putBytes(t *testing.T, bucket, key string, data []byte) {
	t.Helper()
	_, err := direct.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(data),
	})
	if err != nil {
		t.Fatalf("PutObject %s/%s: %v", bucket, key, err)
	}
}

// getBytes downloads an object fully.
func getBytes(t *testing.T, bucket, key string) []byte {
	t.Helper()
	out, err := direct.GetObject(context.Background(), &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		t.Fatalf("GetObject %s/%s: %v", bucket, key, err)
	}
	defer out.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(out.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return buf.Bytes()
}

// payload returns n deterministic pseudo-random bytes (seeded, so failures reproduce).
func payload(n int, seed int64) []byte {
	b := make([]byte, n)
	_, _ = mrand.New(mrand.NewSource(seed)).Read(b)
	return b
}

// is404 reports whether err is an S3 "missing" answer.
func is404(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "StatusCode: 404") || strings.Contains(s, "NotFound") || strings.Contains(s, "NoSuchKey") ||
		strings.Contains(s, "NoSuchBucket") || strings.Contains(s, "NoSuchUpload") || strings.Contains(s, "NoSuchVersion")
}
