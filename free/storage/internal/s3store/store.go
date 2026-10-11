// Package s3store is the plugin-storage client for the configured S3 endpoint.
//
// Purpose: store, read and delete object bytes in an S3-compatible endpoint (SeaweedFS, MinIO, AWS)
// through aws-sdk-go-v2, path-style, with multipart uploads above PartSize.
// Inputs: Config (endpoint, region, static credentials, SSL flag) and per-call bucket/key/body.
// Outputs: PutResult, Object, Info, presigned URLs, or errors (ErrNotFound for a missing key).
// Constraints:
//   - New makes no network call; credentials come from the caller (env in cmd/main.go), never from code.
//   - Only aws-sdk-go-v2/aws, /credentials and /service/s3 are used (no config/sso/sts/imds, no feature/s3/manager).
//   - Request/response checksums are "when required" so SeaweedFS and MinIO accept every call.
package s3store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// ErrNotFound is returned (wrapped) when the key or bucket does not exist (NoSuchKey, NotFound, HTTP 404).
var ErrNotFound = errors.New("s3store: not found")

// Config configures New.
type Config struct {
	Endpoint   string // host[:port] or full URL; a bare host gets http:// or https:// from UseSSL
	Region     string // signing region; default us-east-1
	AccessKey  string // both keys set, or both empty (anonymous)
	SecretKey  string
	UseSSL     bool
	HTTPClient s3.HTTPClient // optional; tests inject a counting client
}

// api is the subset of *s3.Client the store uses; unit tests replace it with a fake.
type api interface {
	PutObject(ctx context.Context, in *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadObject(ctx context.Context, in *s3.HeadObjectInput, opts ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, opts ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	DeleteObjects(ctx context.Context, in *s3.DeleteObjectsInput, opts ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
	CreateMultipartUpload(ctx context.Context, in *s3.CreateMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error)
	UploadPart(ctx context.Context, in *s3.UploadPartInput, opts ...func(*s3.Options)) (*s3.UploadPartOutput, error)
	CompleteMultipartUpload(ctx context.Context, in *s3.CompleteMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error)
	AbortMultipartUpload(ctx context.Context, in *s3.AbortMultipartUploadInput, opts ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error)
	CreateBucket(ctx context.Context, in *s3.CreateBucketInput, opts ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	HeadBucket(ctx context.Context, in *s3.HeadBucketInput, opts ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
}

// Store talks to one S3 endpoint. Safe for concurrent use.
type Store struct {
	api    api
	pre    *s3.PresignClient
	region string
}

// normalizeEndpoint returns a base URL with an explicit http:// or https:// scheme.
func normalizeEndpoint(endpoint string, useSSL bool) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", errors.New("s3store: endpoint is empty")
	}
	if !strings.Contains(endpoint, "://") {
		scheme := "http"
		if useSSL {
			scheme = "https"
		}
		endpoint = scheme + "://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("s3store: malformed endpoint: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return "", errors.New("s3store: malformed endpoint (need http(s)://host[:port])")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// New builds a Store. It rejects a half-set credential pair and a malformed endpoint; it never dials.
func New(cfg Config) (*Store, error) {
	if (cfg.AccessKey == "") != (cfg.SecretKey == "") {
		return nil, errors.New("s3store: set both access key and secret key, or neither")
	}
	base, err := normalizeEndpoint(cfg.Endpoint, cfg.UseSSL)
	if err != nil {
		return nil, err
	}
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	opts := s3.Options{
		BaseEndpoint:               aws.String(base),
		Region:                     region,
		UsePathStyle:               true,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
		Credentials:                aws.AnonymousCredentials{},
	}
	if cfg.AccessKey != "" {
		opts.Credentials = credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")
	}
	if cfg.HTTPClient != nil {
		opts.HTTPClient = cfg.HTTPClient
	} else {
		opts.HTTPClient = &http.Client{}
	}
	c := s3.New(opts)
	return &Store{api: c, pre: s3.NewPresignClient(c), region: region}, nil
}

// mapErr turns "missing" S3 errors into ErrNotFound and leaves every other error as is.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "NoSuchKey", "NotFound", "NoSuchBucket":
			return fmt.Errorf("%w: %v", ErrNotFound, err)
		}
	}
	var re *awshttp.ResponseError
	if errors.As(err, &re) && re.HTTPStatusCode() == http.StatusNotFound {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return err
}

// Object is an open object body plus its headers; the caller closes Body.
type Object struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
	ETag        string
}

// Info is the HeadObject result.
type Info struct {
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

// Get opens an object for streaming.
func (s *Store) Get(ctx context.Context, bucket, key string) (*Object, error) {
	out, err := s.api.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return nil, mapErr(err)
	}
	return &Object{Body: out.Body, Size: aws.ToInt64(out.ContentLength), ContentType: aws.ToString(out.ContentType), ETag: aws.ToString(out.ETag)}, nil
}

// Head returns object metadata without the body.
func (s *Store) Head(ctx context.Context, bucket, key string) (Info, error) {
	out, err := s.api.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return Info{}, mapErr(err)
	}
	return Info{Size: aws.ToInt64(out.ContentLength), ContentType: aws.ToString(out.ContentType), ETag: aws.ToString(out.ETag), LastModified: aws.ToTime(out.LastModified)}, nil
}

// Delete removes one object. S3 answers 204 for a missing key, so a missing key is not an error.
func (s *Store) Delete(ctx context.Context, bucket, key string) error {
	_, err := s.api.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	return mapErr(err)
}

// EnsureBucket creates the bucket; when creation fails but the bucket already exists (HeadBucket ok) it succeeds.
func (s *Store) EnsureBucket(ctx context.Context, bucket string) error {
	in := &s3.CreateBucketInput{Bucket: aws.String(bucket)}
	if s.region != "us-east-1" {
		in.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{LocationConstraint: s3types.BucketLocationConstraint(s.region)}
	}
	_, cerr := s.api.CreateBucket(ctx, in)
	if cerr == nil {
		return nil
	}
	if _, herr := s.api.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}); herr == nil {
		return nil
	}
	return cerr
}
