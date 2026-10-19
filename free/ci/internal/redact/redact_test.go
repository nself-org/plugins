package redact

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const secret = "hunter2/s3cr3t+Tok3n=&x y"

func TestRegisterMinimum(t *testing.T) {
	r := New()
	err := r.Register([]byte("12345"))
	var e *Error
	if !errors.As(err, &e) || e.Code != "E673" {
		t.Fatalf("want E673, got %v", err)
	}
	if !strings.Contains(err.Error(), "6-byte") || strings.Contains(err.Error(), "12345") {
		t.Fatalf("error text must name the minimum and hide the value: %q", err)
	}
	if err := r.Register([]byte("123456")); err != nil {
		t.Fatalf("6 bytes is allowed: %v", err)
	}
	if err := r.Register(nil); err == nil {
		t.Fatal("empty value must be refused, not skipped")
	}
}

func TestRegisterMaximum(t *testing.T) {
	r := New()
	if err := r.Register(make([]byte, 128<<10)); err != nil {
		t.Fatalf("the maximum length is allowed: %v", err)
	}
	err := r.Register(make([]byte, 128<<10+1))
	var e *Error
	if !errors.As(err, &e) || e.Code != "E680" || strings.Contains(err.Error(), "\x00") {
		t.Fatalf("want E680, got %v", err)
	}
}

func TestRawValueEverySplit(t *testing.T) {
	r := New()
	mustRegister(t, r, secret)
	in := "token=" + secret + " end\n"
	want := "token=[REDACTED] end\n"
	if got := run(r, in); got != want {
		t.Fatalf("got %q", got)
	}
	for i := 0; i <= len(in); i++ {
		if got := run(r, in[:i], in[i:]); got != want {
			t.Fatalf("split %d: %q", i, got)
		}
	}
	if got := strings.Join(bytewise(run(r, bytewise(in)...)), ""); got != want {
		t.Fatalf("bytewise: %q", got)
	}
}

func TestPlainFormsMasked(t *testing.T) {
	r := New()
	mustRegister(t, r, secret)
	for name, form := range plainForms(secret) {
		in := "before " + form + " after"
		want := "before [REDACTED] after"
		if got := run(r, in); got != want {
			t.Errorf("%s: got %q want %q", name, got, want)
		}
		for i := 1; i < len(in); i += 3 {
			if got := run(r, in[:i], in[i:]); got != want {
				t.Errorf("%s split %d: got %q", name, i, got)
			}
		}
	}
}

func TestBase64AlignmentsAndWraps(t *testing.T) {
	r := New()
	mustRegister(t, r, secret)
	for _, prefix := range []string{"", "a", "ab", "abc", "user:"} {
		for _, suffix := range []string{"", "z", "zz", "zzz"} {
			for name, form := range b64Embedded(prefix, secret, suffix) {
				in := "before " + form + " after"
				got := run(r, in)
				rest, masked := leak(got, "before ", " after")
				if ctx := (len(prefix)+len(suffix))*4/3 + 8; !masked || len(rest) > ctx {
					t.Errorf("%s p=%q s=%q: masked=%v leak=%q out=%q", name, prefix, suffix, masked, rest, got)
				}
				if split := run(r, in[:len(in)/2], in[len(in)/2:]); collapse(split) != collapse(got) {
					t.Errorf("%s split differs: %q vs %q", name, split, got)
				}
			}
		}
	}
}

func TestBenignUnchanged(t *testing.T) {
	r := New()
	mustRegister(t, r, secret)
	for _, s := range []string{
		"commit 3b18e512dba79e4c8300dd08aeb37f8e728b8dad done\n",
		"id 123e4567-e89b-12d3-a456-426614174000\n",
		"data SGVsbG8gd29ybGQsIHRoaXMgaXMgbm90IGEgc2VjcmV0Lg==\n",
		"use std::error::Error and std::fmt::Display\n",
		"## Heading\n### Sub\n",
		"disk-usage-and-more-long-words-here-and-there\n",
		"AKIA short\n",
		"\x1b[31mred\x1b[0m \x1b[1;32mgreen\x1b[0m\n",
		"ghp_ short and sk_live_ too\n",
		"-----BEGIN not a header\n",
	} {
		if got := run(r, s); got != s {
			t.Errorf("changed %q -> %q", s, got)
		}
		if got := run(r, bytewise(s)...); got != s {
			t.Errorf("bytewise changed %q -> %q", s, got)
		}
	}
}

func TestMultiLineValueEveryLine(t *testing.T) {
	r := New()
	lines := []string{"AAAAAAAAAAAA1", "BBBBBBBBBBBB2", "ccc", "CCCCCCCCCCCC3"}
	val := strings.Join(lines, "\n")
	mustRegister(t, r, val)
	for _, nl := range []string{"\n", "\r\n"} {
		var in strings.Builder
		for i, l := range lines {
			fmt.Fprintf(&in, "12:00:0%d %s%s", i, l, nl)
		}
		got := run(r, in.String())
		for _, l := range lines {
			if len(l) >= MinValueLen && strings.Contains(got, l) {
				t.Errorf("line %q survived %q", l, got)
			}
		}
		if !strings.Contains(got, "ccc") {
			t.Errorf("short line must stay: %q", got)
		}
	}
	whole := run(r, val+"\n", strings.ReplaceAll(val, "\n", "\r\n"))
	if strings.Contains(whole, "AAAAAAAAAAAA1") {
		t.Fatalf("whole value survived: %q", whole)
	}
}

func TestScanFindsRawAndBase64(t *testing.T) {
	r := New()
	mustRegister(t, r, secret)
	pre := string([]byte{0, 1, 2, 255, 254, 0, 9})
	blob := pre + secret + pre + wrapAt(b64Embedded("", secret, "")["b64"], 64, "\n") + pre
	got := r.Scan([]byte(blob))
	if len(got) != 2 || got[0].Offset != len(pre) || got[0].Len != len(secret) || got[1].Offset <= got[0].Offset {
		t.Fatalf("matches %+v", got)
	}
	if m := r.Scan([]byte(pre + pre)); len(m) != 0 {
		t.Fatalf("clean blob matched: %+v", m)
	}
}

func TestWrapFlushAndErrors(t *testing.T) {
	r := New()
	mustRegister(t, r, secret)
	var sb strings.Builder
	w := r.Wrap(&sb)
	for _, c := range bytewise("a " + secret + " b " + secret[:9]) {
		if n, err := w.Write([]byte(c)); n != 1 || err != nil {
			t.Fatalf("write: %d %v", n, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got := sb.String(); got != "a [REDACTED] b "+secret[:9] {
		t.Fatalf("got %q", got)
	}
}

func TestConcurrentRegisterAndUse(t *testing.T) {
	r := New()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			_ = r.Register([]byte(fmt.Sprintf("value-number-%03d", i)))
		}
		close(done)
	}()
	for i := 0; i < 50; i++ {
		_ = run(r, "value-number-001 and more")
		r.Scan([]byte("value-number-002"))
	}
	<-done
	if got := run(r, "x value-number-049 y"); got != "x [REDACTED] y" {
		t.Fatalf("got %q", got)
	}
}

func TestBoundedCarryOver(t *testing.T) {
	r := New()
	mustRegister(t, r, strings.Repeat("a", 40))
	f := r.NewFilter()
	for i := 0; i < 2000; i++ {
		f.Redact([]byte(strings.Repeat("a", 100)))
		if len(f.buf) > 4096 {
			t.Fatalf("carry-over grew to %d bytes", len(f.buf))
		}
	}
}
