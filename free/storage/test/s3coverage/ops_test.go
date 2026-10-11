//go:build s3coverage

package s3coverage

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var ctx = context.Background()

func TestOp_CreateBucket(t *testing.T) {
	name := newBucket(t)
	if _, err := direct.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(name)}); err != nil {
		t.Fatalf("bucket not visible after CreateBucket: %v", err)
	}
}

func TestOp_HeadBucket(t *testing.T) {
	name := newBucket(t)
	if _, err := direct.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(name)}); err != nil {
		t.Fatalf("HeadBucket existing: %v", err)
	}
	_, err := direct.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String("s3cov-missing-" + randHex(4))})
	if !is404(err) {
		t.Fatalf("HeadBucket on a missing bucket: want 404, got %v", err)
	}
}

func TestOp_ListBuckets(t *testing.T) {
	name := newBucket(t)
	out, err := direct.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}
	for _, b := range out.Buckets {
		if aws.ToString(b.Name) == name {
			return
		}
	}
	t.Fatalf("bucket %s not in ListBuckets (%d buckets)", name, len(out.Buckets))
}

func TestOp_PutObjectGetObject(t *testing.T) {
	b := newBucket(t)
	data := payload(100*1024, 1)
	_, err := direct.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b), Key: aws.String("dir/obj.bin"), Body: bytes.NewReader(data),
		ContentType: aws.String("application/x-s3cov"), Metadata: map[string]string{"owner": "s3cov"},
	})
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	out, err := direct.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(b), Key: aws.String("dir/obj.bin")})
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	defer out.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(out.Body)
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("GetObject bytes differ (got %d, want %d)", buf.Len(), len(data))
	}
	if ct := aws.ToString(out.ContentType); ct != "application/x-s3cov" {
		t.Errorf("Content-Type = %q", ct)
	}
	if out.Metadata["owner"] != "s3cov" {
		t.Errorf("user metadata = %v", out.Metadata)
	}
	if aws.ToInt64(out.ContentLength) != int64(len(data)) {
		t.Errorf("Content-Length = %d", aws.ToInt64(out.ContentLength))
	}
}

func TestOp_HeadObject(t *testing.T) {
	b := newBucket(t)
	data := payload(4096, 2)
	_, err := direct.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(b), Key: aws.String("h.bin"),
		Body: bytes.NewReader(data), ContentType: aws.String("image/png")})
	if err != nil {
		t.Fatal(err)
	}
	out, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(b), Key: aws.String("h.bin")})
	if err != nil {
		t.Fatalf("HeadObject: %v", err)
	}
	if aws.ToInt64(out.ContentLength) != 4096 || aws.ToString(out.ETag) == "" || aws.ToString(out.ContentType) != "image/png" {
		t.Errorf("HeadObject = len %d etag %q type %q", aws.ToInt64(out.ContentLength), aws.ToString(out.ETag), aws.ToString(out.ContentType))
	}
	_, err = direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(b), Key: aws.String("nope")})
	if !is404(err) {
		t.Fatalf("HeadObject on a missing key: want 404, got %v", err)
	}
}

func TestOp_ListObjectsV2(t *testing.T) {
	b := newBucket(t)
	for _, k := range []string{"a/1", "a/2", "b/1"} {
		putBytes(t, b, k, []byte(k))
	}
	out, err := direct.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(b), Prefix: aws.String("a/")})
	if err != nil {
		t.Fatalf("ListObjectsV2: %v", err)
	}
	var keys []string
	for _, o := range out.Contents {
		keys = append(keys, aws.ToString(o.Key))
	}
	if strings.Join(keys, ",") != "a/1,a/2" {
		t.Fatalf("prefix a/ listed %v, want [a/1 a/2]", keys)
	}
	out, err = direct.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(b), Delimiter: aws.String("/")})
	if err != nil {
		t.Fatalf("ListObjectsV2 delimiter: %v", err)
	}
	if len(out.CommonPrefixes) != 2 {
		t.Errorf("delimiter listing common prefixes = %d, want 2", len(out.CommonPrefixes))
	}
}

func TestOp_DeleteObject(t *testing.T) {
	b := newBucket(t)
	putBytes(t, b, "d.txt", []byte("x"))
	if _, err := direct.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(b), Key: aws.String("d.txt")}); err != nil {
		t.Fatalf("DeleteObject: %v", err)
	}
	if _, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(b), Key: aws.String("d.txt")}); !is404(err) {
		t.Fatalf("object still there after DeleteObject: %v", err)
	}
	if _, err := direct.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(b), Key: aws.String("never-existed")}); err != nil {
		t.Errorf("DeleteObject of a missing key must succeed: %v", err)
	}
}

func TestOp_DeleteObjects(t *testing.T) {
	b := newBucket(t)
	var ids []s3types.ObjectIdentifier
	for _, k := range []string{"x/1", "x/2", "x/3", "y"} {
		putBytes(t, b, k, []byte(k))
		ids = append(ids, s3types.ObjectIdentifier{Key: aws.String(k)})
	}
	out, err := direct.DeleteObjects(ctx, &s3.DeleteObjectsInput{Bucket: aws.String(b),
		Delete: &s3types.Delete{Objects: ids, Quiet: aws.Bool(true)}})
	if err != nil {
		t.Fatalf("DeleteObjects: %v", err)
	}
	if len(out.Errors) > 0 {
		t.Fatalf("DeleteObjects per-key errors: %v", out.Errors)
	}
	lo, err := direct.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(b)})
	if err != nil || len(lo.Contents) != 0 {
		t.Fatalf("bucket not empty after DeleteObjects: %v %d", err, len(lo.Contents))
	}
}

func TestOp_Multipart(t *testing.T) {
	b := newBucket(t)
	key := aws.String("multi.bin")
	cm, err := direct.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(b), Key: key,
		ContentType: aws.String("application/x-multi")})
	if err != nil {
		t.Fatalf("CreateMultipartUpload: %v", err)
	}
	p1, p2 := payload(5<<20, 3), payload(1024, 4)
	var parts []s3types.CompletedPart
	for i, p := range [][]byte{p1, p2} {
		n := int32(i + 1)
		up, err := direct.UploadPart(ctx, &s3.UploadPartInput{Bucket: aws.String(b), Key: key, UploadId: cm.UploadId,
			PartNumber: aws.Int32(n), Body: bytes.NewReader(p)})
		if err != nil {
			t.Fatalf("UploadPart %d: %v", n, err)
		}
		parts = append(parts, s3types.CompletedPart{ETag: up.ETag, PartNumber: aws.Int32(n)})
	}
	lp, err := direct.ListParts(ctx, &s3.ListPartsInput{Bucket: aws.String(b), Key: key, UploadId: cm.UploadId})
	if err != nil || len(lp.Parts) != 2 {
		t.Fatalf("ListParts: %v (%d parts)", err, len(lp.Parts))
	}
	if _, err := direct.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{Bucket: aws.String(b), Key: key,
		UploadId: cm.UploadId, MultipartUpload: &s3types.CompletedMultipartUpload{Parts: parts}}); err != nil {
		t.Fatalf("CompleteMultipartUpload: %v", err)
	}
	got := getBytes(t, b, "multi.bin")
	if !bytes.Equal(got, append(append([]byte{}, p1...), p2...)) {
		t.Fatalf("multipart object differs from the uploaded parts (got %d bytes)", len(got))
	}
	h, err := direct.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(b), Key: key})
	if err != nil || aws.ToString(h.ContentType) != "application/x-multi" {
		t.Errorf("multipart Content-Type = %q err %v", aws.ToString(h.ContentType), err)
	}
}

func TestOp_AbortMultipartUpload(t *testing.T) {
	b := newBucket(t)
	key := aws.String("aborted.bin")
	cm, err := direct.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(b), Key: key})
	if err != nil {
		t.Fatalf("CreateMultipartUpload: %v", err)
	}
	if _, err := direct.UploadPart(ctx, &s3.UploadPartInput{Bucket: aws.String(b), Key: key, UploadId: cm.UploadId,
		PartNumber: aws.Int32(1), Body: bytes.NewReader(payload(5<<20, 5))}); err != nil {
		t.Fatalf("UploadPart: %v", err)
	}
	if _, err := direct.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(b), Key: key, UploadId: cm.UploadId}); err != nil {
		t.Fatalf("AbortMultipartUpload: %v", err)
	}
	if lp, err := direct.ListParts(ctx, &s3.ListPartsInput{Bucket: aws.String(b), Key: key, UploadId: cm.UploadId}); err == nil && len(lp.Parts) > 0 {
		t.Fatalf("ListParts after abort still returns %d parts", len(lp.Parts))
	}
	if _, err := direct.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(b), Key: key}); !is404(err) {
		t.Fatalf("aborted upload produced an object: %v", err)
	}
	mu, err := direct.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{Bucket: aws.String(b)})
	if err != nil || len(mu.Uploads) != 0 {
		t.Fatalf("ListMultipartUploads after abort: %v (%d)", err, len(mu.Uploads))
	}
}
