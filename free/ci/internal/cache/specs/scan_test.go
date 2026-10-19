package specs

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/redact"
)

func registered(t *testing.T) *redact.Redactor {
	t.Helper()
	r := redact.New()
	if err := r.Register([]byte(fakeSecret)); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestScanWriterFindsAValueInEveryPlace(t *testing.T) {
	r := registered(t)
	b64 := base64.StdEncoding.EncodeToString([]byte(fakeSecret))
	pad := func(n int) []byte { return bytes.Repeat([]byte("."), n) }
	cases := map[string][]byte{
		"raw at the start":         append([]byte(fakeSecret), pad(10)...),
		"raw in the middle":        append(append(pad(500), fakeSecret...), pad(500)...),
		"base64":                   append(append(pad(100), b64...), pad(100)...),
		"across the window edge":   append(append(pad(scanWindow-10), fakeSecret...), pad(scanWindow)...),
		"across the second window": append(append(pad(2*scanWindow-17), fakeSecret...), pad(40)...),
		"at the very end":          append(pad(scanWindow+5), fakeSecret...),
	}
	for name, data := range cases {
		var out bytes.Buffer
		w := newScanWriter(&out, r)
		var err error
		for off := 0; off < len(data) && err == nil; off += 70000 { // uneven chunks
			_, err = w.Write(data[off:min(off+70000, len(data))])
		}
		if err == nil {
			err = w.Close()
		}
		if !errors.Is(err, ErrSecretMatch) {
			t.Errorf("%s: err %v", name, err)
		}
		if bytes.Contains(out.Bytes(), []byte(fakeSecret)) {
			t.Errorf("%s: the value reached the downstream writer", name)
		}
	}
}

func TestScanWriterPassesCleanDataUnchanged(t *testing.T) {
	data := bytes.Repeat([]byte("clean cache bytes "), 200000)
	var out bytes.Buffer
	w := newScanWriter(&out, registered(t))
	for off := 0; off < len(data); off += 33333 {
		if _, err := w.Write(data[off:min(off+33333, len(data))]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil || !bytes.Equal(out.Bytes(), data) {
		t.Fatalf("close %v, equal %v", err, bytes.Equal(out.Bytes(), data))
	}
}

func TestScanWriterWithoutAScannerRefuses(t *testing.T) {
	w := newScanWriter(&bytes.Buffer{}, nil)
	_, _ = w.Write([]byte("x"))
	if err := w.Close(); !errors.Is(err, ErrSecretMatch) {
		t.Fatalf("err %v", err)
	}
}
