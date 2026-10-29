package preflight

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testLicence = "ci-test-licence-token"

func TestCheckClassifiesEveryEntry(t *testing.T) {
	body := []byte("tarball bytes")
	sum := sha256.Sum256(body)
	good := hex.EncodeToString(sum[:])
	var keyOnFile bool
	mux := http.NewServeMux()
	mux.HandleFunc("/plugins/alpha/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-License-Key") != testLicence {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		http.Redirect(w, r, "/files/alpha.tgz", http.StatusFound)
	})
	mux.HandleFunc("/files/alpha.tgz", func(w http.ResponseWriter, r *http.Request) {
		keyOnFile = keyOnFile || r.Header.Get("X-License-Key") != ""
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/plugins/beta/download", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) })
	mux.HandleFunc("/plugins/gamma/download", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/plugins/delta/download", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	entries := []Entry{{"gamma", "1", good}, {"alpha", "1", good}, {"beta", "1", good}, {"delta", "1", strings.Repeat("0", 64)}}
	rs, err := Check(srv.Client(), srv.URL, testLicence, entries)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{"alpha": {"skip", "build"}, "beta": {"fail", "entitlement"}, "gamma": {"fail", "404"}, "delta": {"fail", "checksum"}}
	for _, x := range rs {
		if w := want[x.Slug]; x.Result != w[0] || x.Class != w[1] {
			t.Errorf("%s: got %s/%s, want %v", x.Slug, x.Result, x.Class, w)
		}
		if strings.Contains(x.Reason, testLicence) {
			t.Errorf("%s: key in reason", x.Slug)
		}
	}
	if rs[0].Slug != "alpha" {
		t.Errorf("results not sorted: %v", rs[0].Slug)
	}
	if keyOnFile {
		t.Error("key sent to redirect target")
	}
	s := Summarize(len(entries), rs)
	if s.OK || s.Skip != 1 || s.Fail != 3 || s.ByClass["entitlement"] != 1 {
		t.Errorf("summary wrong: %+v", s)
	}
	if Summarize(len(entries)+1, rs).OK {
		t.Error("missing entry must fail coverage")
	}
}

func TestCheckNeedsKey(t *testing.T) {
	if _, err := Check(http.DefaultClient, "http://127.0.0.1:1", "", []Entry{{"a", "1", "x"}}); err != ErrNoKey {
		t.Fatalf("got %v, want ErrNoKey", err)
	}
}

func TestSummarizePass(t *testing.T) {
	if !Summarize(1, []Result{{"a", "pass", "", ""}}).OK || Summarize(0, nil).OK {
		t.Error("pass/empty handling wrong")
	}
}
