//go:build s3coverage

package s3coverage

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// objectURL is the plain path-style URL of an object (for unsigned and preflight requests).
func objectURL(bucket, key string) string { return env.baseURL + "/" + bucket + "/" + key }

// httpDo runs a plain net/http request (no SDK, no signature) and returns status, headers and body.
func httpDo(t *testing.T, method, target string, body io.Reader, hdr map[string]string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, b
}

func TestOp_PresignedGet(t *testing.T) {
	b := newBucket(t)
	data := payload(10_000, 6)
	putBytes(t, b, "p/get.bin", data)
	ps := s3.NewPresignClient(direct)
	req, err := ps.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(b), Key: aws.String("p/get.bin")}, s3.WithPresignExpires(time.Minute))
	if err != nil {
		t.Fatalf("PresignGetObject: %v", err)
	}
	if !strings.Contains(req.URL, "X-Amz-Signature=") {
		t.Fatalf("not a SigV4 query URL: %s", req.URL)
	}
	code, _, body := httpDo(t, http.MethodGet, req.URL, nil, nil)
	if code != 200 || !bytes.Equal(body, data) {
		t.Fatalf("presigned GET: status %d, %d bytes (want 200, %d)", code, len(body), len(data))
	}
	// A tampered signature must be refused.
	bad := strings.Replace(req.URL, "X-Amz-Signature=", "X-Amz-Signature=00", 1)
	if code, _, _ := httpDo(t, http.MethodGet, bad, nil, nil); code == 200 {
		t.Errorf("tampered presigned URL was accepted")
	}
}

func TestOp_PresignedPut(t *testing.T) {
	b := newBucket(t)
	data := payload(20_000, 7)
	ps := s3.NewPresignClient(direct)
	req, err := ps.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(b), Key: aws.String("p/put.bin")}, s3.WithPresignExpires(time.Minute))
	if err != nil {
		t.Fatalf("PresignPutObject: %v", err)
	}
	code, _, body := httpDo(t, http.MethodPut, req.URL, bytes.NewReader(data), nil)
	if code != 200 {
		t.Fatalf("presigned PUT: status %d: %s", code, body)
	}
	if got := getBytes(t, b, "p/put.bin"); !bytes.Equal(got, data) {
		t.Fatalf("presigned PUT stored %d bytes, want %d identical", len(got), len(data))
	}
}

func TestOp_PathStyle(t *testing.T) {
	b := newBucket(t)
	putBytes(t, b, "ps.txt", []byte("path-style"))
	want, _ := url.Parse(env.baseURL)
	seen := 0
	for _, r := range dirRec.snapshot() {
		if strings.HasPrefix(r.Host, b+".") {
			t.Fatalf("virtual-hosted request seen: %s %s%s", r.Method, r.Host, r.Path)
		}
		if strings.HasPrefix(r.Path, "/"+b+"/") || r.Path == "/"+b {
			if r.Host != want.Host {
				t.Errorf("bucket request went to host %q, want %q", r.Host, want.Host)
			}
			seen++
		}
	}
	if seen < 2 {
		t.Fatalf("recorded only %d path-style requests for bucket %s", seen, b)
	}
}

func TestOp_PutBucketPolicy(t *testing.T) {
	b := newBucket(t)
	putBytes(t, b, "public.txt", []byte("hello public"))
	if code, _, _ := httpDo(t, http.MethodGet, objectURL(b, "public.txt"), nil, nil); code == 200 {
		t.Fatalf("object readable without a policy (unsigned GET returned 200)")
	}
	policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Sid":"PublicRead","Effect":"Allow","Principal":"*","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/*"]}]}`, b)
	if _, err := direct.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{Bucket: aws.String(b), Policy: aws.String(policy)}); err != nil {
		t.Fatalf("PutBucketPolicy: %v", err)
	}
	gp, err := direct.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: aws.String(b)})
	if err != nil {
		t.Fatalf("GetBucketPolicy: %v", err)
	}
	if !strings.Contains(aws.ToString(gp.Policy), "s3:GetObject") {
		t.Errorf("GetBucketPolicy lost the statement: %s", aws.ToString(gp.Policy))
	}
	code, _, body := httpDo(t, http.MethodGet, objectURL(b, "public.txt"), nil, nil)
	if code != 200 || string(body) != "hello public" {
		t.Fatalf("unsigned GET after public-read policy: status %d body %q", code, body)
	}
}

func TestOp_PutBucketCors(t *testing.T) {
	b := newBucket(t)
	origin := "https://app.example.test"
	_, err := direct.PutBucketCors(ctx, &s3.PutBucketCorsInput{Bucket: aws.String(b), CORSConfiguration: &s3types.CORSConfiguration{
		CORSRules: []s3types.CORSRule{{
			AllowedOrigins: []string{origin}, AllowedMethods: []string{"GET", "PUT", "POST"},
			AllowedHeaders: []string{"*"}, ExposeHeaders: []string{"ETag"}, MaxAgeSeconds: aws.Int32(3600),
		}},
	}})
	if err != nil {
		t.Fatalf("PutBucketCors: %v", err)
	}
	gc, err := direct.GetBucketCors(ctx, &s3.GetBucketCorsInput{Bucket: aws.String(b)})
	if err != nil || len(gc.CORSRules) != 1 {
		t.Fatalf("GetBucketCors: %v (%d rules)", err, len(gc.CORSRules))
	}
	code, hdr, _ := httpDo(t, http.MethodOptions, objectURL(b, "up.bin"), nil, map[string]string{
		"Origin": origin, "Access-Control-Request-Method": "PUT", "Access-Control-Request-Headers": "content-type",
	})
	if code != 200 && code != 204 {
		t.Fatalf("preflight status %d", code)
	}
	if got := hdr.Get("Access-Control-Allow-Origin"); got != origin && got != "*" {
		t.Fatalf("preflight Access-Control-Allow-Origin = %q, want %q", got, origin)
	}
}

func TestOp_ListObjectVersions(t *testing.T) {
	// Unversioned bucket: the object is listed.
	plain := newBucket(t)
	putBytes(t, plain, "v.txt", []byte("one"))
	out, err := direct.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: aws.String(plain)})
	if err != nil {
		t.Fatalf("ListObjectVersions (unversioned): %v", err)
	}
	found := false
	for _, v := range out.Versions {
		found = found || aws.ToString(v.Key) == "v.txt"
	}
	if !found {
		t.Fatalf("unversioned bucket: v.txt missing from ListObjectVersions")
	}

	// Versioned bucket: two puts give two versions; deleting one by VersionId removes exactly that one.
	vb := newBucket(t)
	if _, err := direct.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{Bucket: aws.String(vb),
		VersioningConfiguration: &s3types.VersioningConfiguration{Status: s3types.BucketVersioningStatusEnabled}}); err != nil {
		t.Fatalf("PutBucketVersioning: %v", err)
	}
	putBytes(t, vb, "v.txt", []byte("first"))
	putBytes(t, vb, "v.txt", []byte("second"))
	out, err = direct.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: aws.String(vb), Prefix: aws.String("v.txt")})
	if err != nil {
		t.Fatalf("ListObjectVersions (versioned): %v", err)
	}
	if len(out.Versions) < 2 {
		t.Fatalf("versioned bucket lists %d versions after two puts, want >= 2", len(out.Versions))
	}
	victim := out.Versions[len(out.Versions)-1]
	if _, err := direct.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(vb), Key: victim.Key, VersionId: victim.VersionId}); err != nil {
		t.Fatalf("DeleteObject by VersionId: %v", err)
	}
	after, err := direct.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: aws.String(vb), Prefix: aws.String("v.txt")})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Versions) != len(out.Versions)-1 {
		t.Fatalf("versions after deleting one: %d, want %d", len(after.Versions), len(out.Versions)-1)
	}
	for _, v := range after.Versions {
		if aws.ToString(v.VersionId) == aws.ToString(victim.VersionId) {
			t.Fatalf("deleted version %s still listed", aws.ToString(victim.VersionId))
		}
	}
}
