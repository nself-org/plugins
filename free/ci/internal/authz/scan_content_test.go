package authz_test

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"testing"
	"unicode/utf16"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

func TestScanContentEncodings(t *testing.T) {
	const secret = "synthetic-secret-with-雪-123456"
	r := redact.New()
	if err := r.Register([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	forms := map[string][]byte{"raw": []byte(secret), "nul": bytes.Join(bytes.Split([]byte(secret), nil), []byte{0})}
	for _, order := range []struct {
		name  string
		order binary.ByteOrder
	}{{"le", binary.LittleEndian}, {"be", binary.BigEndian}} {
		var b bytes.Buffer
		for _, u := range utf16.Encode([]rune(secret)) {
			if err := binary.Write(&b, order.order, u); err != nil {
				t.Fatal(err)
			}
		}
		forms[order.name] = b.Bytes()
	}
	for name, b := range forms {
		t.Run(name, func(t *testing.T) {
			if authz.CleanCandidates(r, append([]byte{'!'}, b...)) {
				t.Fatal("encoded secret missed")
			}
		})
	}
	if !authz.CleanCandidates(r, []byte("clean content")) {
		t.Fatal("clean text refused")
	}
}

func TestScanArchiveAllMembersAndBudget(t *testing.T) {
	const secret = "synthetic-archive-secret-123456"
	r := redact.New()
	if err := r.Register([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"gzip", "zip"} {
		for _, tc := range []struct {
			name, payload string
			budget        int64
			want          error
		}{
			{"secret", secret, 1 << 20, authz.ErrSecretContent},
			{"clean", "clean data", 1 << 20, nil},
			{"budget", "clean data", 1, authz.ErrScanBudget},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				var b bytes.Buffer
				if kind == "gzip" {
					for _, text := range []string{"clean first member", tc.payload} {
						w := gzip.NewWriter(&b)
						if _, err := w.Write([]byte(text)); err != nil {
							t.Fatal(err)
						}
						if err := w.Close(); err != nil {
							t.Fatal(err)
						}
					}
				} else {
					w := zip.NewWriter(&b)
					for i, text := range []string{"clean first entry", tc.payload} {
						f, err := w.Create(string(rune('a' + i)))
						if err != nil {
							t.Fatal(err)
						}
						if _, err := f.Write([]byte(text)); err != nil {
							t.Fatal(err)
						}
					}
					if err := w.Close(); err != nil {
						t.Fatal(err)
					}
				}
				left := tc.budget
				err := authz.ScanArchive(bytes.NewReader(b.Bytes()), int64(b.Len()), r, &left)
				if !errors.Is(err, tc.want) {
					t.Fatalf("got %v, want %v", err, tc.want)
				}
			})
		}
	}
}
