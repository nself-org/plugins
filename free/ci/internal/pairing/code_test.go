package pairing

// Purpose: the pairing code (alphabet, shape, normalization, hash) and FuzzPairingInput, which feeds
// arbitrary text to Normalize, ValidShape and HashCode, arbitrary JSON to the strict enroll.Request
// decoder and arbitrary bodies to the real enrollment handler: never a panic, never an echoed code.
// Inputs:  generated strings and bodies. Outputs: pass/fail. Constraints: the rig is built once per process.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
)

func TestCodeGeneration(t *testing.T) {
	seen := map[string]bool{}
	counts := map[rune]int{}
	for i := 0; i < 2000; i++ {
		c, err := GenerateCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(c) != CodeLen || !ValidShape(c) || Normalize(c) != c {
			t.Fatalf("code %q", c)
		}
		if seen[c] {
			t.Fatalf("duplicate code %q in 2000", c)
		}
		seen[c] = true
		for _, r := range c {
			counts[r]++
		}
	}
	if len(counts) != 32 {
		t.Fatalf("only %d of 32 alphabet characters seen", len(counts))
	}
	for r, n := range counts { // 20000 draws over 32 symbols: expected 625 each
		if n < 400 || n > 850 {
			t.Fatalf("character %q drawn %d times: not uniform", r, n)
		}
	}
}

func TestCodeNormalizeAndShape(t *testing.T) {
	for _, tc := range []struct {
		in, norm string
		valid    bool
	}{
		{"abcdefghjk", "ABCDEFGHJK", true},
		{"abcde-fghjk", "ABCDEFGHJK", true},
		{" ABCD EFGH JK ", "ABCDEFGHJK", true},
		{"a-b-c-d-e-f-g-h-j-k", "ABCDEFGHJK", true},
		{"ABCDEFGHJ", "ABCDEFGHJ", false},     // 9
		{"ABCDEFGHJKL", "ABCDEFGHJKL", false}, // 11
		{"ABCDEFGH01", "ABCDEFGH01", false},   // 0 and 1 are not base32
		{"ABCDEFGH89", "ABCDEFGH89", false},
		{"ABCDEFGHéK", "ABCDEFGHÉK", false},
		{"ABCDEFGHJ\n", "ABCDEFGHJ\n", false},
		{"", "", false},
		{"----", "", false},
	} {
		if got := Normalize(tc.in); got != tc.norm {
			t.Errorf("Normalize(%q) = %q want %q", tc.in, got, tc.norm)
		}
		if got := ValidShape(tc.in); got != tc.valid {
			t.Errorf("ValidShape(%q) = %v want %v", tc.in, got, tc.valid)
		}
	}
	a, b := HashCode("abcde-fghjk"), HashCode("ABCDEFGHJK")
	sum := sha256.Sum256([]byte("ABCDEFGHJK"))
	if !bytes.Equal(a, b) || !bytes.Equal(a, sum[:]) || bytes.Equal(a, HashCode("ABCDEFGHJL")) {
		t.Fatal("HashCode is not SHA-256 of the normalized code")
	}
}

func TestPairingErrorsAreE659(t *testing.T) {
	var e error = &Error{Reason: ReasonLocked, Msg: "locked"}
	if ReasonOf(e) != ReasonLocked || ReasonOf(nil) != "" || ReasonOf(context.Canceled) != "" {
		t.Fatal("ReasonOf")
	}
	if !strings.HasPrefix(e.Error(), "E659: ") || e.(*Error).ErrCode() != CodeInvalid || e.(*Error).ErrReason() != ReasonLocked {
		t.Fatalf("error text %q", e)
	}
}

func FuzzPairingInput(f *testing.F) {
	r := newRig(f, nil)
	good := newNodeMat(f)
	off, err := r.svc.Offer(context.Background(), OfferOptions{Approver: approving("policy:fuzz", DefaultTrust())})
	if err != nil {
		f.Fatal(err)
	}
	var mu sync.Mutex
	tick := time.Now()
	h := enroll.Handler(r.svc, enroll.HandlerOptions{Now: func() time.Time { tick = tick.Add(2 * time.Second); return tick }})
	post := func(t *testing.T, body []byte) (int, string) {
		req := httptest.NewRequest(http.MethodPost, enroll.Route, bytes.NewReader(body))
		req.RemoteAddr = "127.0.0.1:4242"
		req.Header.Set("Authorization", "Bearer fuzz-token")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String() + rec.Header().Get("Www-Authenticate")
	}
	for _, s := range []string{"ABCDEFGHJK", "abcde-fghjk", " a b ", "", "ıſ0189", "\x00\xff", off.Code} {
		f.Add(s, "node-f", []byte(`{"phase":"hello","version":1,"client_time":1}`))
	}
	f.Add("ZZZZZZZZZZ", "Node Name", []byte(`{"phase":"enroll","version":1,"client_time":1,"code":"ZZZZZZZZZZ","csr":"AA==","node_key":"AA==","name":"x"}`))
	f.Add("ABCDEFGHJK", "", []byte(`{"phase":"enroll","version":1,"bogus":1}`))
	f.Add("ABCDEFGHJK", "x", []byte(`[[[[[[[[[[`))
	f.Fuzz(func(t *testing.T, code, name string, body []byte) {
		norm := Normalize(code)
		if ValidShape(code) {
			sum := sha256.Sum256([]byte(norm))
			if len(norm) != CodeLen || !bytes.Equal(HashCode(code), sum[:]) || Normalize(norm) != norm || !bytes.Equal(HashCode(strings.ToLower(code)), HashCode(code)) {
				t.Fatalf("code %q normalizes to %q", code, norm)
			}
		}
		if len(HashCode(code)) != 32 {
			t.Fatal("hash length")
		}
		var q enroll.Request
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.DisallowUnknownFields()
		if dec.Decode(&q) == nil {
			if _, err := json.Marshal(q); err != nil {
				t.Fatalf("decoded request does not re-encode: %v", err)
			}
		}
		mu.Lock()
		defer mu.Unlock()
		structured, _ := json.Marshal(enroll.Request{Phase: enroll.PhaseEnroll, Version: enroll.Version, ClientTime: r.clock.Now().Unix(),
			Code: code, CSR: good.csr, NodeKey: good.pub, NodeKeySig: good.sign("local", r.clock.Now().Unix()), Name: name})
		secret := ""
		if ValidShape(code) && code != off.Code {
			secret = norm // a wrong, well-formed code must not come back in any reply
		}
		for _, b := range [][]byte{structured, body} {
			status, reply := post(t, b)
			if status == http.StatusInternalServerError {
				t.Fatalf("500 for body %q: %s", b, reply)
			}
			if secret != "" && strings.Contains(reply, secret) {
				t.Fatalf("the reply echoes the code: %s", reply)
			}
			if strings.Contains(reply, "fuzz-token") {
				t.Fatalf("the reply echoes the Authorization header: %s", reply)
			}
		}
		var parsed enroll.Request
		if json.Unmarshal(body, &parsed) == nil && ValidShape(parsed.Code) && parsed.Code != off.Code {
			if _, reply := post(t, body); strings.Contains(reply, Normalize(parsed.Code)) {
				t.Fatalf("the reply echoes the code of the raw body: %s", reply)
			}
		}
	})
}
