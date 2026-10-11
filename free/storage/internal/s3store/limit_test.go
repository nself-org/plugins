package s3store

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A body cut off by http.MaxBytesReader must come out of Put as an error that errors.As maps back to
// *http.MaxBytesError (the handler answers 413 from it), on both paths, and must abort a multipart upload in progress.
func TestPut_maxBytesErrorSurvivesAndAborts(t *testing.T) {
	cases := []struct {
		name       string
		size, max  int64
		wantCreate int
	}{
		{"single PutObject path", 3 << 20, 2 << 20, 0},
		{"multipart in progress", 40 << 20, 20 << 20, 1},
	}
	for _, c := range cases {
		f := &fakeAPI{}
		body := http.MaxBytesReader(httptest.NewRecorder(), readCloser{newPattern(c.size)}, c.max)
		_, err := (&Store{api: f}).Put(context.Background(), "b", "k", body, "", -1)
		var mbe *http.MaxBytesError
		if !errors.As(err, &mbe) {
			t.Fatalf("%s: err %v is not *http.MaxBytesError", c.name, err)
		}
		if f.puts != 0 || f.completes != 0 || f.creates != c.wantCreate || f.aborts != c.wantCreate {
			t.Errorf("%s: puts=%d creates=%d completes=%d aborts=%d", c.name, f.puts, f.creates, f.completes, f.aborts)
		}
	}
}

// readCloser adds a no-op Close to a reader.
type readCloser struct{ io.Reader }

func (readCloser) Close() error { return nil }
