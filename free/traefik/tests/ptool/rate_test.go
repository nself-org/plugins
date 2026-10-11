package main

import (
	"strings"
	"testing"
	"time"
)

func TestJudgeBurst(t *testing.T) {
	ok := func(adm int) burstRun { return burstRun{first: 200, admitted: adm, limited: 8, max: 12} }
	cases := []struct {
		name          string
		burst         int
		nginx, traefk burstRun
		want          string // "" = pass, else a substring of the failure
	}{
		{"both admit burst+1", 5, ok(6), ok(6), ""},
		{"non-200 upstream still counts as admitted", 2, burstRun{first: 404, admitted: 3, limited: 8, max: 8}, burstRun{first: 404, admitted: 3, limited: 8, max: 8}, ""},
		{"traefik admits nothing", 2, ok(3), burstRun{first: 429, admitted: 0, limited: 10, max: 8}, "traefik admitted nothing"},
		{"nginx admits nothing", 2, burstRun{admitted: 0, limited: 10, max: 8}, ok(3), "nginx admitted nothing"},
		{"traefik burst too small", 5, ok(6), ok(2), "traefik admitted fewer than the burst"},
		{"nginx burst too small", 20, ok(3), ok(21), "nginx admitted fewer than the burst"},
		{"traefik admits too many", 5, ok(6), ok(13), "admitted more than the burst allows"},
		{"never limited", 5, ok(6), burstRun{first: 200, admitted: 13, limited: 0, max: 20}, "never answered 429"},
	}
	for _, c := range cases {
		got := judgeBurst(c.burst, c.nginx, c.traefk)
		if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q, got %q", c.name, c.want, got)
		}
	}
}

func TestRefillWait(t *testing.T) {
	for _, c := range []struct {
		burst  int
		perSec float64
		want   time.Duration
	}{
		{2, 5.0 / 60, 37 * time.Second}, // 3 tokens at 5r/m
		{20, 10, 3100 * time.Millisecond},
		{1000, 0.5, time.Minute}, // capped
		{5, 0, 0},                // unknown rate: no wait
	} {
		if got := refillWait(c.burst, c.perSec); got < c.want-50*time.Millisecond || got > c.want+50*time.Millisecond {
			t.Errorf("refillWait(%d, %v) = %v, want %v", c.burst, c.perSec, got, c.want)
		}
	}
}
