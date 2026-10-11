package s3store

import (
	"bufio"
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

// Put streams body into bucket/key. Memory is bounded by one PartSize buffer; the body is never fully buffered.
// On any multipart failure the upload is aborted (with a context that survives cancellation of ctx).
func (s *Store) Put(ctx context.Context, bucket, key string, body io.Reader, contentType string) (PutResult, error) {
	br := bufio.NewReader(body)
	buf := make([]byte, PartSize)
	n, err := io.ReadFull(br, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return PutResult{}, fmt.Errorf("s3store: read body: %w", err)
	}
	short := err != nil // fewer than PartSize bytes: the stream ended
	if !short {
		if _, perr := br.Peek(1); errors.Is(perr, io.EOF) {
			short = true
		} else if perr != nil {
			return PutResult{}, fmt.Errorf("s3store: read body: %w", perr)
		}
	}
	if short {
		return s.putSingle(ctx, bucket, key, buf[:n], contentType)
	}
	return s.putMultipart(ctx, bucket, key, br, buf, contentType)
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
func (s *Store) putMultipart(ctx context.Context, bucket, key string, br *bufio.Reader, buf []byte, contentType string) (res PutResult, err error) {
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
