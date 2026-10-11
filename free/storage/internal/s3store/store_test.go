package s3store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// patternByte is the byte at absolute offset i of the test payload: position dependent, so a part-ordering,
// offset or buffer-reuse bug changes the hash.
func patternByte(i int64) byte { return byte(i*131 + i>>8 + i>>16) }

// patternReader yields n bytes of the position-dependent payload.
type patternReader struct{ pos, n int64 }

func (z *patternReader) Read(p []byte) (int, error) {
	if z.pos >= z.n {
		return 0, io.EOF
	}
	k := int64(len(p))
	if k > z.n-z.pos {
		k = z.n - z.pos
	}
	for i := int64(0); i < k; i++ {
		p[i] = patternByte(z.pos + i)
	}
	z.pos += k
	return int(k), nil
}

func newPattern(n int64) *patternReader { return &patternReader{n: n} }

// sums returns the SHA-256 (hex) of each chunk-sized slice of the n-byte payload; chunk <= 0 means one slice.
func sums(n, chunk int64) []string {
	var out []string
	for off := int64(0); off < n || off == 0; off += chunk {
		end := off + chunk
		if chunk <= 0 || end > n {
			end = n
		}
		h := sha256.New()
		_, _ = io.Copy(h, &patternReader{pos: off, n: end})
		out = append(out, hex.EncodeToString(h.Sum(nil)))
		if chunk <= 0 {
			break
		}
	}
	return out
}

// fakeAPI records calls and can fail chosen ones. It embeds api so unused methods panic if called.
type fakeAPI struct {
	api
	puts, creates, completes, aborts, deleteCalls int
	partLens                                      []int
	partSums                                      []string // SHA-256 (hex) of each uploaded part, in order
	putSums                                       []string // SHA-256 (hex) of each single PutObject body
	failPart                                      int      // 1-based part number to fail, 0 = none
	deleteBatchSizes                              []int
	deleteErrs                                    []s3types.Error
	getErr, createBucketErr, headBucketErr        error
}

func (f *fakeAPI) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.puts++
	b, _ := io.ReadAll(in.Body)
	if int64(len(b)) != aws.ToInt64(in.ContentLength) {
		return nil, errors.New("content length mismatch")
	}
	h := sha256.Sum256(b)
	f.putSums = append(f.putSums, hex.EncodeToString(h[:]))
	return &s3.PutObjectOutput{ETag: aws.String(`"single"`)}, nil
}
func (f *fakeAPI) CreateMultipartUpload(_ context.Context, _ *s3.CreateMultipartUploadInput, _ ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	f.creates++
	return &s3.CreateMultipartUploadOutput{UploadId: aws.String("u1")}, nil
}
func (f *fakeAPI) UploadPart(_ context.Context, in *s3.UploadPartInput, _ ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	n := int(aws.ToInt32(in.PartNumber))
	if f.failPart == n {
		return nil, errors.New("boom")
	}
	b, _ := io.ReadAll(in.Body)
	f.partLens = append(f.partLens, len(b))
	h := sha256.Sum256(b)
	f.partSums = append(f.partSums, hex.EncodeToString(h[:]))
	return &s3.UploadPartOutput{ETag: aws.String("p")}, nil
}
func (f *fakeAPI) CompleteMultipartUpload(_ context.Context, in *s3.CompleteMultipartUploadInput, _ ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	f.completes++
	return &s3.CompleteMultipartUploadOutput{ETag: aws.String(`"multi-2"`)}, nil
}
func (f *fakeAPI) AbortMultipartUpload(_ context.Context, _ *s3.AbortMultipartUploadInput, _ ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	f.aborts++
	return &s3.AbortMultipartUploadOutput{}, nil
}
func (f *fakeAPI) DeleteObjects(_ context.Context, in *s3.DeleteObjectsInput, _ ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	f.deleteCalls++
	f.deleteBatchSizes = append(f.deleteBatchSizes, len(in.Delete.Objects))
	return &s3.DeleteObjectsOutput{Errors: f.deleteErrs}, nil
}
func (f *fakeAPI) GetObject(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return nil, f.getErr
}
func (f *fakeAPI) CreateBucket(_ context.Context, _ *s3.CreateBucketInput, _ ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	return &s3.CreateBucketOutput{}, f.createBucketErr
}
func (f *fakeAPI) HeadBucket(_ context.Context, _ *s3.HeadBucketInput, _ ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	return &s3.HeadBucketOutput{}, f.headBucketErr
}

func TestPut_exactlyPartSizeIsSingle(t *testing.T) {
	f := &fakeAPI{}
	res, err := (&Store{api: f}).Put(context.Background(), "b", "k", newPattern(PartSize), "text/plain")
	if err != nil || res.Multipart || res.Size != PartSize || f.puts != 1 || f.creates != 0 {
		t.Fatalf("res=%+v err=%v puts=%d creates=%d", res, err, f.puts, f.creates)
	}
}

func TestPut_emptyBodyIsSingle(t *testing.T) {
	f := &fakeAPI{}
	res, err := (&Store{api: f}).Put(context.Background(), "b", "k", strings.NewReader(""), "")
	if err != nil || res.Size != 0 || f.puts != 1 {
		t.Fatalf("res=%+v err=%v puts=%d", res, err, f.puts)
	}
}

func TestPut_17MiBIsTwoPartsThenComplete(t *testing.T) {
	f := &fakeAPI{}
	res, err := (&Store{api: f}).Put(context.Background(), "b", "k", newPattern(17<<20), "")
	if err != nil || !res.Multipart || res.Size != 17<<20 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if f.creates != 1 || f.completes != 1 || f.aborts != 0 || len(f.partLens) != 2 || f.partLens[0] != PartSize || f.partLens[1] != 1<<20 {
		t.Fatalf("creates=%d completes=%d aborts=%d parts=%v", f.creates, f.completes, f.aborts, f.partLens)
	}
	if res.ETag != `"multi-2"` {
		t.Fatalf("etag %q", res.ETag)
	}
}

func TestPut_multipleOfPartSizeHasNoEmptyTail(t *testing.T) {
	f := &fakeAPI{}
	res, err := (&Store{api: f}).Put(context.Background(), "b", "k", newPattern(2*PartSize), "")
	if err != nil || res.Size != 2*PartSize || len(f.partLens) != 2 || f.partLens[1] != PartSize {
		t.Fatalf("res=%+v err=%v parts=%v", res, err, f.partLens)
	}
}

func TestPut_abortsOnUploadPartFailure(t *testing.T) {
	f := &fakeAPI{failPart: 2}
	_, err := (&Store{api: f}).Put(context.Background(), "b", "k", newPattern(17<<20), "")
	if err == nil || f.aborts != 1 || f.completes != 0 {
		t.Fatalf("err=%v aborts=%d completes=%d", err, f.aborts, f.completes)
	}
}

type errReader struct{ sent bool }

func (e *errReader) Read(p []byte) (int, error) {
	if !e.sent {
		e.sent = true
		return copy(p, make([]byte, 100)), nil
	}
	return 0, errors.New("client went away")
}

func TestPut_bodyReadErrorAborts(t *testing.T) {
	f := &fakeAPI{}
	body := io.MultiReader(newPattern(PartSize+5), &errReader{})
	_, err := (&Store{api: f}).Put(context.Background(), "b", "k", body, "")
	if err == nil || f.aborts != 1 {
		t.Fatalf("err=%v aborts=%d", err, f.aborts)
	}
}

func TestDeleteObjects_batches(t *testing.T) {
	f := &fakeAPI{}
	keys := make([]string, 2500)
	for i := range keys {
		keys[i] = "k"
	}
	if err := (&Store{api: f}).DeleteObjects(context.Background(), "b", keys); err != nil {
		t.Fatal(err)
	}
	if f.deleteCalls != 3 || f.deleteBatchSizes[0] != 1000 || f.deleteBatchSizes[2] != 500 {
		t.Fatalf("calls=%d sizes=%v", f.deleteCalls, f.deleteBatchSizes)
	}
	if err := (&Store{api: f}).DeleteObjects(context.Background(), "b", nil); err != nil || f.deleteCalls != 3 {
		t.Fatalf("empty list made calls: %v %d", err, f.deleteCalls)
	}
}

func TestDeleteObjects_perKeyErrorIsError(t *testing.T) {
	f := &fakeAPI{deleteErrs: []s3types.Error{{Key: aws.String("k"), Code: aws.String("AccessDenied")}}}
	if err := (&Store{api: f}).DeleteObjects(context.Background(), "b", []string{"k"}); err == nil {
		t.Fatal("per-key error must surface")
	}
}

type apiErr struct{ code string }

func (e apiErr) Error() string                 { return e.code }
func (e apiErr) ErrorCode() string             { return e.code }
func (e apiErr) ErrorMessage() string          { return e.code }
func (e apiErr) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

func TestMapErr_notFound(t *testing.T) {
	for _, code := range []string{"NoSuchKey", "NotFound", "NoSuchBucket"} {
		if err := mapErr(apiErr{code}); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s not mapped: %v", code, err)
		}
	}
	if err := mapErr(apiErr{"AccessDenied"}); errors.Is(err, ErrNotFound) {
		t.Error("AccessDenied must not map to not found")
	}
	f := &fakeAPI{getErr: &s3types.NoSuchKey{}}
	if _, err := (&Store{api: f}).Get(context.Background(), "b", "k"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get NoSuchKey: %v", err)
	}
	if mapErr(nil) != nil {
		t.Error("nil must stay nil")
	}
}

func TestEnsureBucket(t *testing.T) {
	boom := errors.New("create failed")
	if err := (&Store{api: &fakeAPI{}, region: "us-east-1"}).EnsureBucket(context.Background(), "b"); err != nil {
		t.Errorf("create ok: %v", err)
	}
	if err := (&Store{api: &fakeAPI{createBucketErr: boom}, region: "us-east-1"}).EnsureBucket(context.Background(), "b"); err != nil {
		t.Errorf("exists (head ok) must succeed: %v", err)
	}
	f := &fakeAPI{createBucketErr: boom, headBucketErr: errors.New("nope")}
	if err := (&Store{api: f, region: "us-east-1"}).EnsureBucket(context.Background(), "b"); !errors.Is(err, boom) {
		t.Errorf("want create error, got %v", err)
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	cases := []struct {
		in   string
		ssl  bool
		want string
		bad  bool
	}{
		{"seaweedfs:8333", false, "http://seaweedfs:8333", false},
		{"seaweedfs:8333", true, "https://seaweedfs:8333", false},
		{"https://s3.example.org/", false, "https://s3.example.org", false},
		{"http://10.0.0.1:9000", true, "http://10.0.0.1:9000", false},
		{"", false, "", true},
		{"ftp://host", false, "", true},
		{"http://", false, "", true},
		{"http://%zz", false, "", true},
	}
	for _, c := range cases {
		got, err := normalizeEndpoint(c.in, c.ssl)
		if (err != nil) != c.bad || got != c.want {
			t.Errorf("normalizeEndpoint(%q,%v) = %q, %v", c.in, c.ssl, got, err)
		}
	}
}

func TestNew_credentialRules(t *testing.T) {
	if _, err := New(Config{Endpoint: "h:1", AccessKey: "s3cov-test-access"}); err == nil {
		t.Error("access key without secret must fail")
	}
	if _, err := New(Config{Endpoint: "h:1", SecretKey: "s3cov-test-secret"}); err == nil {
		t.Error("secret without access key must fail")
	}
	if _, err := New(Config{Endpoint: "://bad"}); err == nil {
		t.Error("malformed endpoint must fail")
	}
	if _, err := New(Config{Endpoint: "h:1", AccessKey: "s3cov-test-access", SecretKey: "s3cov-test-secret"}); err != nil {
		t.Errorf("valid config: %v", err)
	}
}

func TestPresign_pathStyleSigV4(t *testing.T) {
	st, err := New(Config{Endpoint: "seaweedfs:8333", AccessKey: "s3cov-test-access", SecretKey: "s3cov-test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.PresignGet(context.Background(), "bkt", "a/b.txt", 60e9)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"http://seaweedfs:8333/bkt/a/b.txt?", "X-Amz-Signature=", "X-Amz-Expires=60", "X-Amz-Algorithm=AWS4-HMAC-SHA256"} {
		if !strings.Contains(u, want) {
			t.Errorf("presigned GET url missing %q: %s", want, u)
		}
	}
	if strings.Contains(u, "s3cov-test-secret") {
		t.Error("secret leaked into URL")
	}
	pu, err := st.PresignPut(context.Background(), "bkt", "k", 60e9)
	if err != nil || !strings.Contains(pu, "http://seaweedfs:8333/bkt/k?") {
		t.Errorf("presigned PUT: %v %s", err, pu)
	}
}

func equalSums(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A part-ordering, offset or buffer-reuse bug changes a part hash; the parts must equal the source split at PartSize.
func TestPut_multipartPartsMatchSourceHashes(t *testing.T) {
	for _, size := range []int64{PartSize + 1, 2 * PartSize, 40 << 20} {
		f := &fakeAPI{}
		res, err := (&Store{api: f}).Put(context.Background(), "b", "k", newPattern(size), "")
		if err != nil || !res.Multipart || res.Size != size {
			t.Fatalf("size %d: res=%+v err=%v", size, res, err)
		}
		if want := sums(size, PartSize); !equalSums(f.partSums, want) {
			t.Fatalf("size %d: part hashes differ from the source split\n got %v\nwant %v", size, f.partSums, want)
		}
	}
}

func TestPut_singleBodiesMatchSourceHash(t *testing.T) {
	for _, size := range []int64{0, 1024, firstChunk, firstChunk + 1, 5 << 20, PartSize} {
		f := &fakeAPI{}
		res, err := (&Store{api: f}).Put(context.Background(), "b", "k", newPattern(size), "")
		if err != nil || res.Multipart || res.Size != size || f.puts != 1 || f.creates != 0 {
			t.Fatalf("size %d: res=%+v err=%v puts=%d creates=%d", size, res, err, f.puts, f.creates)
		}
		if want := sums(size, 0); !equalSums(f.putSums, want) {
			t.Fatalf("size %d: body hash differs from the source", size)
		}
	}
}

func TestPut_smallBodyNeverTakesPoolOrSlot(t *testing.T) {
	st := &Store{api: &fakeAPI{}, slots: make(chan struct{}, 1)}
	st.slots <- struct{}{} // the only slot is taken: a small upload must not need it
	before := poolGets.Load()
	res, err := st.Put(context.Background(), "b", "k", newPattern(1024), "")
	if err != nil || res.Size != 1024 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if poolGets.Load() != before {
		t.Fatal("a 1 KB body took a pooled 16 MiB buffer")
	}
	if len(st.slots) != 1 {
		t.Fatal("a 1 KB body changed the slot count")
	}
}

func TestPut_largeBodyReleasesSlotAndTakesPool(t *testing.T) {
	st := &Store{api: &fakeAPI{}, slots: make(chan struct{}, 2)}
	before := poolGets.Load()
	if _, err := st.Put(context.Background(), "b", "k", newPattern(2<<20), ""); err != nil {
		t.Fatal(err)
	}
	if poolGets.Load() != before+1 || len(st.slots) != 0 {
		t.Fatalf("pool gets +%d, slots held %d", poolGets.Load()-before, len(st.slots))
	}
}

func TestPut_blockedSlotHonoursContext(t *testing.T) {
	f := &fakeAPI{}
	st := &Store{api: f, slots: make(chan struct{}, 1)}
	st.slots <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := st.Put(ctx, "b", "k", newPattern(3<<20), "")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline exceeded, got %v", err)
	}
	if f.puts+f.creates != 0 {
		t.Fatal("S3 was called while waiting for a slot")
	}
	if len(st.slots) != 1 {
		t.Fatal("the held slot was released by the waiter")
	}
}

func TestDefaultHTTPClient(t *testing.T) {
	tr := defaultHTTPClient().Transport.(*http.Transport)
	if tr.MaxIdleConnsPerHost != 32 || tr.ResponseHeaderTimeout != 60*time.Second {
		t.Fatalf("transport = %d idle, %v header timeout", tr.MaxIdleConnsPerHost, tr.ResponseHeaderTimeout)
	}
}
