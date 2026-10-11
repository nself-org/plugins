package internal

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/nself-org/plugin-storage/internal/s3store"
)

// uploadIdleTimeout is how long an upload body may stay silent before the read fails and the upload slot frees up.
const uploadIdleTimeout = 60 * time.Second

// downloadWriteTimeout is the response-write window granted after an upload body has been fully read (the server's
// own WriteTimeout started when the request arrived and may have run out during a long upload).
const downloadWriteTimeout = 60 * time.Second

// idleReader extends a read deadline after every read that returned data, so a body that keeps flowing may take
// as long as it needs while one that stalls for idle fails (and the caller's slot is released).
type idleReader struct {
	r           io.Reader
	idle        time.Duration
	setDeadline func(time.Time) error // http.ResponseController.SetReadDeadline; may be unsupported
}

func newIdleReader(w http.ResponseWriter, body io.Reader, idle time.Duration) *idleReader {
	rc := http.NewResponseController(w)
	ir := &idleReader{r: body, idle: idle, setDeadline: rc.SetReadDeadline}
	ir.extend()
	return ir
}

// extend pushes the read deadline idle into the future; unsupported servers (tests) are ignored.
func (i *idleReader) extend() { _ = i.setDeadline(time.Now().Add(i.idle)) }

func (i *idleReader) Read(p []byte) (int, error) {
	n, err := i.r.Read(p)
	if n > 0 {
		i.extend()
	}
	return n, err
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
