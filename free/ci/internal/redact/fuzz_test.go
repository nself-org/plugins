package redact

import (
	"strings"
	"testing"
)

var fuzzValues = []string{"fuzz-secret-value-1", "line-one-secret\nline-two-secret"}

func fuzzRedactor() *Redactor {
	r := New()
	for _, v := range fuzzValues {
		_ = r.Register([]byte(v))
	}
	return r
}

// FuzzRedactStream: for any input and any split point, chunked output equals
// whole output, and no registered value survives.
func FuzzRedactStream(f *testing.F) {
	r := fuzzRedactor()
	for _, s := range []string{
		"plain text\n",
		"token fuzz-secret-value-1 end",
		"line-one-secret\r\nline-two-secret\r\n",
		"ZnV6ei1zZWNyZXQtdmFsdWUtMQ== tail",
		"::add-mask::x ##vso[a]b \x1b]52;c;x\x07 \x1b[31mok\x1b[0m",
		"-----BEGIN " + "PRIVATE KEY-----\nabc\n-----END " + "PRIVATE KEY-----\nafter",
		"ghp" + "_" + strings.Repeat("Ab1", 12) + " AKIA" + "ABCDEFGH12345678",
		"fuzz-secret%2Dvalue-1 \u0000 \xc2\x9b",
	} {
		f.Add([]byte(s), uint8(3))
	}
	f.Fuzz(func(t *testing.T, data []byte, split uint8) {
		whole := run(r, string(data))
		for _, v := range fuzzValues {
			if strings.Contains(whole, v) {
				t.Fatalf("value survived: %q -> %q", data, whole)
			}
		}
		cut := int(split) % (len(data) + 1)
		if got := run(r, string(data[:cut]), string(data[cut:])); collapse(got) != collapse(whole) {
			t.Fatalf("split %d differs\n whole=%q\n split=%q\n input=%q", cut, whole, got, data)
		}
		if got := run(r, bytewise(string(data))...); collapse(got) != collapse(whole) {
			t.Fatalf("bytewise differs\n whole=%q\n bytes=%q\n input=%q", whole, got, data)
		}
	})
}
