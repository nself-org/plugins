package main

import (
	"errors"
	"net/http"
	"testing"
)

func TestDiffTransportErrors(t *testing.T) {
	boom := errors.New("connection refused")
	ok := result{status: 200, hdr: http.Header{}, sum: "0123456789abcdef"}
	cases := []struct {
		name    string
		a, b    result
		wantErr bool // true = a non-empty failure string is required
	}{
		{"both error", result{err: boom}, result{err: boom}, true},
		{"nginx errors", result{err: boom}, ok, true},
		{"traefik errors", ok, result{err: boom}, true},
		{"no error, equal", ok, ok, false},
	}
	for _, c := range cases {
		got := diff(c.a, c.b)
		if c.wantErr && got == "" {
			t.Errorf("%s: diff returned \"\", want a failure", c.name)
		}
		if !c.wantErr && got != "" {
			t.Errorf("%s: diff = %q, want \"\"", c.name, got)
		}
	}
}
