package s3store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// PartSize is both the multipart part size and the single-PUT ceiling: a body of PartSize bytes or fewer
// goes up in one PutObject, anything longer uses a multipart upload.
const PartSize = 16 << 20

// maxParts is the S3 limit on parts per multipart upload.
const maxParts = 10000

// PutResult describes a stored object.
type PutResult struct {
	Size      int64
	ETag      string
	Multipart bool
}

// firstChunk is how much of the body Put reads into a small, growable buffer before it decides whether the
// full PartSize buffer (and an upload slot) is needed. Bodies up to this size never touch the pool.
const firstChunk = 1 << 20

// Put streams body into bucket/key. A body of firstChunk bytes or fewer is read into a small buffer and sent with
// one PutObject. A longer body takes an upload slot (bounded by Config.MaxLargeUploads, waiting honours ctx) and one
// pooled PartSize buffer; it goes up as one PutObject when it ends within PartSize bytes, otherwise as a multipart
// upload that is aborted (with a context that survives cancellation of ctx) on any failure.
func (s *Store) Put(ctx context.Context, bucket, key string, body io.Reader, contentType string) (res PutResult, err error) {
	head, rerr := io.ReadAll(io.LimitReader(body, firstChunk+1))
	if rerr != nil {
		return PutResult{}, fmt.Errorf("s3store: read body: %w", rerr)
	}
	if len(head) <= firstChunk {
		return s.putSingle(ctx, bucket, key, head, contentType)
	}

	release, aerr := s.acquire(ctx)
	if aerr != nil {
		return PutResult{}, aerr
	}
	defer release()
	bp := getBuf()
	defer func() {
		// After a failure the transport may still hold a slice of the buffer; only a clean run recycles it.
		if err == nil {
			putBuf(bp)
		}
	}()
	buf := *bp
	n := copy(buf, head)
	m, rerr := io.ReadFull(body, buf[n:])
	n += m
	if rerr != nil && !errors.Is(rerr, io.EOF) && !errors.Is(rerr, io.ErrUnexpectedEOF) {
		return PutResult{}, fmt.Errorf("s3store: read body: %w", rerr)
	}
	if rerr != nil { // the stream ended within PartSize bytes
		return s.putSingle(ctx, bucket, key, buf[:n], contentType)
	}
	// The buffer is full: peek one more byte to tell "exactly PartSize" (single PutObject) from "more follows".
	var one [1]byte
	if _, perr := io.ReadFull(body, one[:]); perr != nil {
		if errors.Is(perr, io.EOF) {
			return s.putSingle(ctx, bucket, key, buf[:n], contentType)
		}
		return PutResult{}, fmt.Errorf("s3store: read body: %w", perr)
	}
	return s.putMultipart(ctx, bucket, key, io.MultiReader(bytes.NewReader(one[:]), body), buf, contentType)
}

// putSingle uploads data with one PutObject call.
func (s *Store) putSingle(ctx context.Context, bucket, key string, data []byte, contentType string) (PutResult, error) {
	in := &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	out, err := s.api.PutObject(ctx, in)
	if err != nil {
		return PutResult{}, mapErr(err)
	}
	return PutResult{Size: int64(len(data)), ETag: aws.ToString(out.ETag)}, nil
}

// putMultipart uploads first (a full buf) and the rest of br as sequential PartSize parts.
func (s *Store) putMultipart(ctx context.Context, bucket, key string, br io.Reader, buf []byte, contentType string) (res PutResult, err error) {
	in := &s3.CreateMultipartUploadInput{Bucket: aws.String(bucket), Key: aws.String(key)}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	mp, err := s.api.CreateMultipartUpload(ctx, in)
	if err != nil {
		return PutResult{}, mapErr(err)
	}
	uploadID := mp.UploadId
	defer func() {
		if err == nil {
			return
		}
		actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_, _ = s.api.AbortMultipartUpload(actx, &s3.AbortMultipartUploadInput{
			Bucket: aws.String(bucket), Key: aws.String(key), UploadId: uploadID,
		})
	}()

	var parts []s3types.CompletedPart
	var total int64
	// upload sends data as the next part number.
	upload := func(data []byte) error {
		num := int32(len(parts) + 1)
		if num > maxParts {
			return fmt.Errorf("s3store: body needs more than %d parts", maxParts)
		}
		up, uerr := s.api.UploadPart(ctx, &s3.UploadPartInput{
			Bucket: aws.String(bucket), Key: aws.String(key), UploadId: uploadID,
			PartNumber: aws.Int32(num), Body: bytes.NewReader(data), ContentLength: aws.Int64(int64(len(data))),
		})
		if uerr != nil {
			return mapErr(uerr)
		}
		parts = append(parts, s3types.CompletedPart{ETag: up.ETag, PartNumber: aws.Int32(num)})
		total += int64(len(data))
		return nil
	}

	if err = upload(buf); err != nil { // the first part is already in buf
		return PutResult{}, err
	}
	for {
		n, rerr := io.ReadFull(br, buf)
		if errors.Is(rerr, io.EOF) { // no more bytes
			break
		}
		if rerr != nil && !errors.Is(rerr, io.ErrUnexpectedEOF) {
			err = fmt.Errorf("s3store: read body: %w", rerr)
			return PutResult{}, err
		}
		if err = upload(buf[:n]); err != nil {
			return PutResult{}, err
		}
		if rerr != nil { // short read: that was the last part
			break
		}
	}
	out, cerr := s.api.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket: aws.String(bucket), Key: aws.String(key), UploadId: uploadID,
		MultipartUpload: &s3types.CompletedMultipartUpload{Parts: parts},
	})
	if cerr != nil {
		err = mapErr(cerr)
		return PutResult{}, err
	}
	return PutResult{Size: total, ETag: aws.ToString(out.ETag), Multipart: true}, nil
}
