package specs

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
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
		data := makeZip(tc.text)
		left := tc.budget
		err := authz.ScanArchive(bytes.NewReader(data), int64(len(data)), r, &left)
		var got error
		if errors.Is(err, authz.ErrSecretContent) {
			got = ErrSecretMatch
		} else if errors.Is(err, authz.ErrScanBudget) {
			got = ErrTooLarge
		} else {
			got = err
		}
		if !errors.Is(got, tc.want) {
			t.Fatalf("budget %d: %v, want %v", tc.budget, err, tc.want)
		}
	}
}
