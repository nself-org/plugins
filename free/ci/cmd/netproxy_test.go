package main

import (
	"bytes"
	"io"
	"net"
	"os"
	"strings"
	"testing"
)

// The sidecar entrypoint is V15 only and hidden: help and the subcommand count never show it.
func TestNetproxyIsHiddenAndV15Only(t *testing.T) {
	e, ok := v15Subcommands["netproxy"]
	if !ok || !e.V15Only || !e.Hidden || e.Flags == nil || e.Handler == nil {
		t.Fatalf("entry %+v registered=%v", e, ok)
	}
	for _, k := range visibleV15() {
		if k == "netproxy" {
			t.Fatal("netproxy is listed")
		}
	}
	if _, ok := subcommands["netproxy"]; ok {
		t.Fatal("netproxy is in the v1.4 table")
	}
	if strings.Contains(v15Help(), "netproxy") {
		t.Fatal("netproxy is in the help text")
	}
}

func TestNetproxyProbeModePrintsWhatConnected(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func(cleanup func() error) { _ = cleanup() }(ln.Close)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	code := runNetproxy([]string{"--probe", ln.Addr().String()})
	os.Stdout = old
	_ = w.Close()
	var out bytes.Buffer
	_, _ = io.Copy(&out, r)
	if code != 0 || !strings.Contains(out.String(), "reached "+ln.Addr().String()) || !strings.Contains(out.String(), "done") {
		t.Fatalf("code %d output %q", code, out.String())
	}
}

func TestNetproxyWithoutConfigFailsWithUsageExit(t *testing.T) {
	t.Setenv("NSELF_NETPROXY_CONFIG", "")
	if code := runNetproxy(nil); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}
