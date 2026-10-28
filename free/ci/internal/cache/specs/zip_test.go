package specs

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/redact"
)

func TestZipPayloadScan(t *testing.T) {
	makeZip := func(text string) []byte {
		var buf bytes.Buffer
		z := zip.NewWriter(&buf)
		f, err := z.Create("payload")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
		if err := z.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	r := redact.New()
	if err := r.Register([]byte(fakeSecret)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		text   string
		budget int64
		want   error
	}{
		{"clean", 1024, nil}, {fakeSecret, 1024, ErrSecretMatch}, {"longer than budget", 3, ErrTooLarge},
	} {
		inf := newInflater(r)
		inf.left = tc.budget
		if err := inf.scan(bytes.NewReader(makeZip(tc.text))); !errors.Is(err, tc.want) {
			t.Fatalf("budget %d: %v, want %v", tc.budget, err, tc.want)
		}
	}
}
