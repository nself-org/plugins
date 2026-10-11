package s3store

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// deleteBatch is the S3 DeleteObjects limit per request.
const deleteBatch = 1000

// DeleteObjects removes keys in batches of 1000. Any per-key error in a response is returned as an error.
func (s *Store) DeleteObjects(ctx context.Context, bucket string, keys []string) error {
	for start := 0; start < len(keys); start += deleteBatch {
		end := start + deleteBatch
		if end > len(keys) {
			end = len(keys)
		}
		ids := make([]s3types.ObjectIdentifier, 0, end-start)
		for _, k := range keys[start:end] {
			ids = append(ids, s3types.ObjectIdentifier{Key: aws.String(k)})
		}
		out, err := s.api.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &s3types.Delete{Objects: ids, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return mapErr(err)
		}
		if len(out.Errors) > 0 {
			e := out.Errors[0]
			return fmt.Errorf("s3store: delete %d of %d keys failed (first: %s %s: %s)",
				len(out.Errors), end-start, aws.ToString(e.Key), aws.ToString(e.Code), aws.ToString(e.Message))
		}
	}
	return nil
}

// PresignGet returns a SigV4 query-signed GET URL valid for ttl.
func (s *Store) PresignGet(ctx context.Context, bucket, key string, ttl time.Duration) (string, error) {
	req, err := s.pre.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)},
		s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// PresignPut returns a SigV4 query-signed PUT URL valid for ttl.
func (s *Store) PresignPut(ctx context.Context, bucket, key string, ttl time.Duration) (string, error) {
	req, err := s.pre.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)},
		s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
