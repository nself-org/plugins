// Package preflight holds the pure-Go classification of the licensed
// served-bytes preflight (P7-PLUG-39); scripts/preflight/licensed.sh is the
// operator wrapper. No exec, no environment reads beyond the caller's key.
package preflight

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
)

// Entry is one licensed registry entry.
type Entry struct{ Slug, Version, Checksum string }

// Result is the per-entry outcome: Result is pass|fail|skip, Class names the stage.
type Result struct{ Slug, Result, Class, Reason string }

// Summary counts results. Pass is true only when every expected entry passed.
type Summary struct {
	Expected, Pass, Fail, Skip int
	ByClass                    map[string]int
	OK                         bool
}

// ErrNoKey is returned when the CI licence is absent (strict mode needs it).
var ErrNoKey = errors.New("CI licence key is not set")

// Check downloads every entry from base/plugins/<slug>/download. The key goes
// only on the first hop; a redirect target gets a plain GET. Results are sorted by slug.
func Check(c *http.Client, base, key string, entries []Entry) ([]Result, error) {
	if key == "" {
		return nil, ErrNoKey
	}
	nc := *c
	nc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	out := make([]Result, 0, len(entries))
	for _, e := range entries {
		out = append(out, checkOne(&nc, base, key, e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

func checkOne(c *http.Client, base, key string, e Entry) Result {
	r := func(res, cls, why string) Result { return Result{e.Slug, res, cls, why} }
	req, err := http.NewRequest(http.MethodGet, base+"/plugins/"+e.Slug+"/download", nil)
	if err != nil {
		return r("fail", "404", "bad request URL")
	}
	req.Header.Set("X-License-Key", key)
	resp, err := c.Do(req)
	if err != nil {
		return r("fail", "404", "transport failure")
	}
	if resp.StatusCode/100 == 3 {
		loc, lerr := resp.Location()
		resp.Body.Close()
		if lerr != nil {
			return r("fail", "404", fmt.Sprintf("HTTP %d without Location", resp.StatusCode))
		}
		if resp, err = c.Get(loc.String()); err != nil {
			return r("fail", "404", "redirect target transport failure")
		}
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return r("fail", "entitlement", fmt.Sprintf("CI licence rejected (HTTP %d) for an entry it should cover", resp.StatusCode))
	default:
		return r("fail", "404", fmt.Sprintf("download answered HTTP %d", resp.StatusCode))
	}
	h := sha256.New()
	if _, err := io.Copy(h, resp.Body); err != nil {
		return r("fail", "404", "body read failed")
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != e.Checksum {
		return r("fail", "checksum", "served sha256 differs from the registry")
	}
	// Extraction, manifest and docker build are the wrapper's stages: a skip is not a pass.
	return r("skip", "build", "bytes verified; build stage not run here")
}

// Summarize counts results against the expected entry total.
func Summarize(expected int, rs []Result) Summary {
	s := Summary{Expected: expected, ByClass: map[string]int{}}
	for _, x := range rs {
		switch x.Result {
		case "pass":
			s.Pass++
		case "skip":
			s.Skip++
		default:
			s.Fail++
		}
		if x.Result != "pass" {
			s.ByClass[x.Class]++
		}
	}
	s.OK = expected > 0 && len(rs) == expected && s.Pass == expected
	return s
}
