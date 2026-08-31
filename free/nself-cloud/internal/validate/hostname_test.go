package validate_test

import (
	"testing"

	"github.com/nself-org/plugins-pro/paid/nself-cloud/internal/validate"
)

func TestValidateHostname(t *testing.T) {
	t.Parallel()

	valid := []string{
		"example.com",
		"sub.example.com",
		"my-host.example.org",
		"xn--nxasmq6b.com", // Punycode
		"a.b.c.d.e",
		"foo123.bar",
		"123.456.com",
		"a",            // single alnum label
		"a1",           // two-char alnum
		"a-b.c-d.net",
	}

	for _, h := range valid {
		h := h
		t.Run("valid/"+h, func(t *testing.T) {
			t.Parallel()
			if err := validate.ValidateHostname(h); err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
		})
	}

	invalid := []struct {
		input string
		desc  string
	}{
		{"", "empty"},
		{"../../../etc/passwd", "path traversal"},
		{"<script>alert(1)</script>", "XSS injection"},
		{"host\x00name", "null byte"},
		{"localhost", "reserved"},
		{"internal", "reserved"},
		{"local", "reserved"},
		{"example", "reserved"},
		{"test", "reserved"},
		{"-bad.com", "label starts with hyphen"},
		{"bad-.com", "label ends with hyphen"},
		{"bad..com", "consecutive dots"},
		{"", "empty string"},
		{
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.com",
			"label > 63 chars",
		},
		{
			// 254 chars: 50 labels of "aa" (2 chars) + dot (1) = 3 chars each = 150 chars,
			// plus a long suffix to exceed 253.
			"aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.aaaa.com",
			"total length > 253",
		},
		{"host/name.com", "slash in hostname"},
		{"host\\name.com", "backslash in hostname"},
	}

	for _, tc := range invalid {
		tc := tc
		t.Run("invalid/"+tc.desc, func(t *testing.T) {
			t.Parallel()
			if err := validate.ValidateHostname(tc.input); err == nil {
				t.Errorf("expected error for %q (%s), got nil", tc.input, tc.desc)
			}
		})
	}
}
