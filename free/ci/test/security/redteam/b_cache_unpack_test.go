//go:build security && unix

package redteam

// Purpose: red team attacks on crafted cache entries (TM-14): a hostile tar fed to the real
// specs.Unpack (traversal, links, special files, set-id, PAX and GNU long names, case and
// normalisation clashes, duplicates, ratio and size bombs).
// Inputs:  tar streams built in memory with archive/tar. Outputs: pass or fail.
// Constraints: local temp dirs only; every case also checks that nothing appeared outside the root.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
)

type tarEnt struct {
	h    tar.Header
	body string
}

func buildTar(t *testing.T, ents []tarEnt, gz bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	var w interface {
		Write([]byte) (int, error)
		Close() error
	} = nopCloser{&buf}
	if gz {
		w = gzip.NewWriter(&buf)
	}
	tw := tar.NewWriter(w)
	for _, e := range ents {
		h := e.h
		if h.Typeflag == tar.TypeReg && h.Size == 0 {
			h.Size = int64(len(e.body))
		}
		if h.Mode == 0 {
			h.Mode = 0o644
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatalf("header %q: %v", h.Name, err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	_ = tw.Close()
	_ = w.Close()
	return buf.Bytes()
}

type nopCloser struct{ *bytes.Buffer }

func (nopCloser) Close() error { return nil }

func reg(name, body string) tarEnt {
	return tarEnt{h: tar.Header{Name: name, Typeflag: tar.TypeReg}, body: body}
}
func dir(name string) tarEnt {
	return tarEnt{h: tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0o755}}
}
func sym(name, to string) tarEnt {
	return tarEnt{h: tar.Header{Name: name, Linkname: to, Typeflag: tar.TypeSymlink, Mode: 0o777}}
}

// unpackInto runs Unpack with root = tmp/base/job and returns the error and the outside dir.
func unpackInto(t *testing.T, data []byte) (error, string, string) {
	t.Helper()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "base"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(filepath.Join(tmp, "base"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	_, uerr := specs.Unpack(bytes.NewReader(data), root, "job", authz.Limits{}, specs.UnpackOptions{})
	return uerr, tmp, filepath.Join(tmp, "base", "job")
}

// escaped lists anything under tmp that is not under tmp/base.
func escaped(tmp string) (out []string) {
	_ = filepath.Walk(tmp, func(p string, info os.FileInfo, err error) error {
		rel, _ := filepath.Rel(tmp, p)
		if err == nil && rel != "." && rel != "base" && !strings.HasPrefix(rel, "base"+string(filepath.Separator)) {
			out = append(out, rel)
		}
		return nil
	})
	return out
}

func TestB_CacheEntry_HostileTarCorpus(t *testing.T) {
	cases := map[string][]tarEnt{
		"traversal":         {reg("../../escape", "x")},
		"nested-traversal":  {reg("a/b/../../../escape", "x")},
		"absolute":          {reg("/tmp/abs-escape", "x")},
		"backslash":         {reg(`..\..\escape`, "x")},
		"symlink-out":       {sym("l", "../../outside"), reg("l/f", "x")},
		"symlink-abs":       {sym("l", "/tmp"), reg("l/f", "x")},
		"symlink-then-file": {dir("sub"), sym("l", "sub"), reg("l/payload", "x")},
		"symlink-chain":     {sym("a", "b"), sym("b", "../.."), reg("a/f", "x")},
		"hardlink":          {tarEnt{h: tar.Header{Name: "h", Linkname: "/etc/hosts", Typeflag: tar.TypeLink}}},
		"hardlink-internal": {reg("real", "x"), tarEnt{h: tar.Header{Name: "h", Linkname: "real", Typeflag: tar.TypeLink}}},
		"fifo":              {tarEnt{h: tar.Header{Name: "p", Typeflag: tar.TypeFifo}}},
		"chardev":           {tarEnt{h: tar.Header{Name: "c", Typeflag: tar.TypeChar, Devmajor: 1, Devminor: 3}}},
		"blockdev":          {tarEnt{h: tar.Header{Name: "b", Typeflag: tar.TypeBlock, Devmajor: 7, Devminor: 0}}},
		"setuid":            {tarEnt{h: tar.Header{Name: "s", Typeflag: tar.TypeReg, Mode: 0o4755}, body: "x"}},
		"setgid-dir":        {tarEnt{h: tar.Header{Name: "d", Typeflag: tar.TypeDir, Mode: 0o2755}}},
		"sticky":            {tarEnt{h: tar.Header{Name: "t", Typeflag: tar.TypeReg, Mode: 0o1777}, body: "x"}},
		"case-clash-file":   {reg("Readme", "a"), reg("README", "b")},
		"case-clash-dir":    {reg("Dir", "a"), dir("dir")},
		"dotgit-hook":       {reg(".git/hooks/pre-commit", "#!/bin/sh\n")},
		"dotgit-fold":       {reg(".GiT./config", "x")},
		"dotgit-symlink":    {sym("g", ".git"), reg("g/config", "x")},
		"duplicate-file":    {reg("d", "one"), reg("d", "two")},
		"dir-then-file":     {dir("x"), reg("x", "y")},
		"file-then-dir":     {reg("x", "y"), reg("x/z", "w")},
		"empty-name":        {reg("", "x")},
		"dot-only":          {reg("...", "x")},
		"control-name":      {reg("a\nb", "x")},
		"bidi-name":         {reg("txt‮gpj.exe", "x")},
		"too-deep":          {reg(strings.Repeat("d/", 200)+"f", "x")},
		"too-long-name":     {reg(strings.Repeat("n", 300), "x")},
	}
	for name, ents := range cases {
		t.Run(name, func(t *testing.T) {
			err, tmp, job := unpackInto(t, buildTar(t, ents, false))
			if err == nil {
				t.Errorf("hostile archive extracted; tree under %s", job)
			}
			if esc := escaped(tmp); len(esc) > 0 {
				t.Errorf("files appeared outside the root: %v", esc)
			}
		})
	}
}

// On a normalisation-insensitive file system (APFS, HFS+) "café" and "café" are one
// name: a link under one spelling redirects a later write under the other spelling.
func TestB_CacheEntry_NormalisationRedirect(t *testing.T) {
	ents := []tarEnt{dir("sub"), sym("café", "sub"), reg("café/payload", "redirected")}
	err, tmp, job := unpackInto(t, buildTar(t, ents, false))
	t.Logf("result: %.90v", err)
	if esc := escaped(tmp); len(esc) > 0 {
		t.Fatalf("files outside the root: %v", esc)
	}
	if _, serr := os.Stat(filepath.Join(job, "sub", "payload")); serr == nil && err == nil {
		t.Fatalf("a write under the NFD spelling went through the NFC symlink into sub/")
	}
}

func TestB_CacheEntry_RatioAndSizeBombs(t *testing.T) {
	zeros := strings.Repeat("\x00", 64<<20)
	bomb := buildTar(t, []tarEnt{reg("zeros", zeros)}, true)
	if len(bomb) > 1<<20 {
		t.Fatalf("fixture is not a bomb: %d bytes", len(bomb))
	}
	err, tmp, job := unpackInto(t, bomb)
	if err == nil {
		t.Error("64 MiB of zeros from a tiny gzip was extracted")
	}
	if esc := escaped(tmp); len(esc) > 0 {
		t.Errorf("outside: %v", esc)
	}
	_ = job
	// a header that claims a huge size with almost no data
	h := tar.Header{Name: "huge", Typeflag: tar.TypeReg, Size: 1 << 41, Mode: 0o644}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&h)
	if err, _, _ := unpackInto(t, buf.Bytes()); err == nil {
		t.Error("an entry claiming 2 TiB with no data was accepted")
	}
	// many empty files
	many := make([]tarEnt, 0, 3000)
	for i := 0; i < 3000; i++ {
		many = append(many, reg(strings.Repeat("a", 5)+string(rune('A'+i%26))+"/"+strings.Repeat("b", i%7)+"x"+itoa(i), ""))
	}
	if err, _, _ := unpackInto(t, buildTar(t, many, true)); err != nil {
		t.Logf("3000 small entries refused (informational): %v", err)
	}
	// truncated stream mid-body must not look like success
	good := buildTar(t, []tarEnt{reg("f", strings.Repeat("y", 4096))}, false)
	if err, _, _ := unpackInto(t, good[:700]); err == nil {
		t.Error("a truncated archive was accepted")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

// rawBlock writes one 512-byte ustar header (and the padded body) by hand, so a typeflag or a
// name encoding that archive/tar refuses to write can still be fed to the reader.
func rawBlock(name string, flag byte, body string) []byte {
	h := make([]byte, 512)
	copy(h[0:], name)
	copy(h[100:], "0000644\x00")
	copy(h[108:], "0000000\x00")
	copy(h[116:], "0000000\x00")
	copy(h[124:], fmt.Sprintf("%011o\x00", len(body)))
	copy(h[136:], "00000000000\x00")
	h[156] = flag
	copy(h[257:], "ustar\x0000")
	copy(h[148:], "        ")
	sum := 0
	for _, b := range h {
		sum += int(b)
	}
	copy(h[148:], fmt.Sprintf("%06o\x00 ", sum))
	out := append(h, body...)
	if pad := len(body) % 512; pad != 0 {
		out = append(out, make([]byte, 512-pad)...)
	}
	return out
}

func paxRec(k, v string) string {
	n := len(k) + len(v) + 3
	for len(fmt.Sprint(n))+n-len(fmt.Sprint(n)) != n || len(fmt.Sprintf("%d %s=%s\n", n, k, v)) != n {
		n++
	}
	return fmt.Sprintf("%d %s=%s\n", n, k, v)
}

func TestB_CacheEntry_RawNameEncodings(t *testing.T) {
	end := make([]byte, 1024)
	long := "../../" + strings.Repeat("g", 120) + "/gnuescape"
	cases := map[string][]byte{
		"control-plain-name":   append(rawBlock("plain", '0', "x"), end...),
		"pax-path-override":    append(append(rawBlock("PaxHeader", 'x', paxRec("path", "../../paxescape")), rawBlock("short", '0', "x")...), end...),
		"pax-linkpath":         append(append(rawBlock("PaxHeader", 'x', paxRec("linkpath", "/etc")), rawBlock("l", '2', "")...), end...),
		"pax-global-path":      append(append(rawBlock("global", 'g', paxRec("path", "../../globalescape")), rawBlock("short", '0', "x")...), end...),
		"gnu-longname":         append(append(rawBlock("././@LongLink", 'L', long+"\x00"), rawBlock("short", '0', "x")...), end...),
		"gnu-longlink":         append(append(rawBlock("././@LongLink", 'K', "/etc/passwd\x00"), rawBlock("l", '2', "")...), end...),
		"nul-inside-name":      append(rawBlock("ok\x00/../../nulescape", '0', "x"), end...),
		"contiguous-file-flag": append(rawBlock("../../contescape", '7', "x"), end...),
		"sparse-flag":          append(rawBlock("../../sparseescape", 'S', "x"), end...),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			err, tmp, job := unpackInto(t, data)
			t.Logf("result: %.90v", err)
			if esc := escaped(tmp); len(esc) > 0 {
				t.Errorf("files appeared outside the root: %v", esc)
			}
			if name == "control-plain-name" {
				if err != nil {
					t.Errorf("fixture control failed: %v", err)
				}
				return
			}
			if err == nil && (strings.HasPrefix(name, "pax-path") || strings.HasPrefix(name, "gnu-long") || name == "pax-global-path" || name == "pax-linkpath") {
				ents, _ := os.ReadDir(job)
				t.Errorf("hostile name encoding extracted without error; job dir holds %d entries", len(ents))
			}
		})
	}
}
