package internal

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nself-org/plugin-storage/internal/s3store"
)

// uploadIdleTimeout is how long an upload body may stay silent before the read fails and the upload slot frees up.
const uploadIdleTimeout = 60 * time.Second

// downloadWriteTimeout is the response-write window granted after an upload body has been fully read (the server's
// own WriteTimeout started when the request arrived and may have run out during a long upload).
const downloadWriteTimeout = 60 * time.Second

// idleReader bounds how long the CLIENT may stay silent. The read deadline is set (now + idle) right before each
// body read and cleared right after it returns, so time the server spends elsewhere between reads (S3 calls,
// slot waits) never counts as client silence, and the first deadline starts with the first read.
type idleReader struct {
	r           io.Reader
	idle        time.Duration
	setDeadline func(time.Time) error // http.ResponseController.SetReadDeadline; may be unsupported
}

func newIdleReader(w http.ResponseWriter, body io.Reader, idle time.Duration) *idleReader {
	return &idleReader{r: body, idle: idle, setDeadline: http.NewResponseController(w).SetReadDeadline}
}

func (i *idleReader) Read(p []byte) (int, error) {
	_ = i.setDeadline(time.Now().Add(i.idle)) // errors: the server does not support deadlines (tests)
	n, err := i.r.Read(p)
	_ = i.setDeadline(time.Time{})
	return n, err
}

// writeLookupError answers a failed single-row DB lookup: no row is 404 with notFound, anything else is 500
// (a database outage must not look like a missing bucket or object).
func writeLookupError(w http.ResponseWriter, err error, notFound, what string) {
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, notFound, http.StatusNotFound)
		return
	}
	log.Printf("storage: %s: %v", what, err)
	http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
}

// writePutError maps an s3store.Put failure to the HTTP answer.
func writePutError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, s3store.ErrBusy):
		w.Header().Set("Retry-After", "5")
		http.Error(w, `{"error":"upload capacity reached, retry"}`, http.StatusServiceUnavailable)
	case errors.Is(err, s3store.ErrBodyTruncated), errors.Is(err, s3store.ErrLengthMismatch):
		http.Error(w, `{"error":"request body incomplete"}`, http.StatusBadRequest)
	case errors.Is(err, os.ErrDeadlineExceeded):
		http.Error(w, `{"error":"request body stalled"}`, http.StatusRequestTimeout)
	default:
		http.Error(w, errStoreFailed, http.StatusBadGateway)
	}
}

// validContentType accepts printable ASCII only (and tab): anything else is refused before an S3 call, where the
// HTTP client would reject it and the failure would surface as a 502.
func validContentType(ct string) bool {
	for i := 0; i < len(ct); i++ {
		c := ct[i]
		if (c < 0x20 && c != '\t') || c > 0x7e {
			return false
		}
	}
	return true
}

// validateObject checks the logical key and the physical S3 key it becomes (account prefix included).
func validateObject(sai, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if len(objectKey(sai, key)) > maxKeyBytes {
		return ErrInvalidKey
	}
	return nil
}

// streamBody copies an object body to the client. A failure mid-stream aborts the response (the client sees a
// broken transfer, not a complete-looking short file).
func streamBody(w io.Writer, body io.Reader, what string) {
	if _, err := io.Copy(w, body); err != nil {
		log.Printf("storage: %s: copy to client: %v", what, err)
		panic(http.ErrAbortHandler)
	}
}
