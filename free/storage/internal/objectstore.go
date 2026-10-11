package internal

import (
	"context"
	"io"
	"net/url"

	"github.com/nself-org/plugin-storage/internal/s3store"
)

// objectStore is the part of *s3store.Store the handlers use; tests substitute a fake.
type objectStore interface {
	EnsureBucket(ctx context.Context, bucket string) error
	Put(ctx context.Context, bucket, key string, body io.Reader, contentType string) (s3store.PutResult, error)
	Get(ctx context.Context, bucket, key string) (*s3store.Object, error)
	Delete(ctx context.Context, bucket, key string) error
	DeleteObjects(ctx context.Context, bucket string, keys []string) error
}

// objectKey is the physical S3 key for a tenant's object: "<escaped source_account_id>/<key>".
// The escape turns "/" in the account id into %2F, so tenant "a/b" can never reach tenant "a"'s keys
// (tenant "a" objects always start with "a/", tenant "a/b" objects with "a%2Fb/").
func objectKey(sourceAccountID, key string) string {
	return url.PathEscape(sourceAccountID) + "/" + key
}
