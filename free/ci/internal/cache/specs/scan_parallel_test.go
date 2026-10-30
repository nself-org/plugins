package specs

// Purpose: windows are scanned by a worker pool; the writer must still release
// every clean byte in stream order and nothing from or after a bad window (D-1515).

import (
	"bytes"
	"errors"
	"math/rand"
	"testing"
)

func randomClean(n int, seed int64) []byte {
	b := make([]byte, n)
	rand.New(rand.NewSource(seed)).Read(b)
	for i := range b { // keep it clear of the fake secret's bytes
		if b[i] == fakeSecret[0] {
			b[i] ^= 1
		}
	}
	return b
}

func writeAll(w *scanWriter, data []byte, step int) error {
	for off := 0; off < len(data); off += step {
		if _, err := w.Write(data[off:min(off+step, len(data))]); err != nil {
			return err
		}
	}
	return w.Close()
}

func TestScanWriterManyWindowsKeepStreamOrder(t *testing.T) {
	data := randomClean(7*scanWindow+12345, 1)
	for _, step := range []int{4096, 1 << 20, 3*scanWindow + 1} {
		var out bytes.Buffer
		if err := writeAll(newScanWriter(&out, registered(t)), data, step); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		if !bytes.Equal(out.Bytes(), data) {
			t.Fatalf("step %d: released bytes differ from the stream (%d vs %d)", step, out.Len(), len(data))
		}
	}
}

func TestScanWriterValueInALateWindowReleasesNothingFromIt(t *testing.T) {
	data := randomClean(6*scanWindow, 2)
	at := 4*scanWindow + 777
	copy(data[at:], fakeSecret)
	var out bytes.Buffer
	err := writeAll(newScanWriter(&out, registered(t)), data, 1<<20)
	if !errors.Is(err, ErrSecretMatch) {
		t.Fatalf("err %v", err)
	}
	if bytes.Contains(out.Bytes(), []byte(fakeSecret)) || out.Len() > 4*scanWindow {
		t.Fatalf("released %d bytes, the window holding the value must not be released", out.Len())
	}
	if !bytes.Equal(out.Bytes(), data[:out.Len()]) {
		t.Fatal("released bytes are not a prefix of the stream")
	}
}

func TestScanWriterValueSplitAcrossWindowsIsFound(t *testing.T) {
	data := randomClean(3*scanWindow, 3)
	at := 2*scanWindow - len(fakeSecret)/2 // straddles the second window edge
	copy(data[at:], fakeSecret)
	err := writeAll(newScanWriter(&bytes.Buffer{}, registered(t)), data, 100000)
	if !errors.Is(err, ErrSecretMatch) {
		t.Fatalf("err %v", err)
	}
}

type failAfter struct{ n int }

func (f *failAfter) Write(p []byte) (int, error) {
	if f.n -= len(p); f.n < 0 {
		return 0, errors.New("disk full")
	}
	return len(p), nil
}

func TestScanWriterReportsADownstreamError(t *testing.T) {
	err := writeAll(newScanWriter(&failAfter{n: scanWindow}, registered(t)), randomClean(5*scanWindow, 4), 1<<20)
	if err == nil || errors.Is(err, ErrSecretMatch) {
		t.Fatalf("err %v", err)
	}
}
