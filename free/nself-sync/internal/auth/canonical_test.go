package auth

import (
	"bytes"
	"testing"
)

// Golden fixtures shared with nclaw/core/src/sync/canonical.rs tests.
// Any change here MUST be mirrored there or signatures will diverge across
// the Rust client and Go server.

func TestCanonical_Primitives(t *testing.T) {
	cases := []struct {
		in  string
		out string
	}{
		{"null", "null"},
		{"true", "true"},
		{"false", "false"},
		{"42", "42"},
		{"-7", "-7"},
		{"0", "0"},
		{"\"\"", `""`},
	}
	for _, c := range cases {
		got, err := CanonicalJSON([]byte(c.in))
		if err != nil {
			t.Fatalf("input %q: %v", c.in, err)
		}
		if string(got) != c.out {
			t.Errorf("input %q: got %q, want %q", c.in, got, c.out)
		}
	}
}

func TestCanonical_NegativeZeroNormalizesToZero(t *testing.T) {
	got, err := CanonicalJSON([]byte("-0.0"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "0" {
		t.Errorf("got %q, want %q", got, "0")
	}
}

func TestCanonical_IntegerValuedFloatPrintsAsInt(t *testing.T) {
	got, err := CanonicalJSON([]byte("100.0"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "100" {
		t.Errorf("got %q, want %q", got, "100")
	}
}

func TestCanonical_ObjectKeysSortedUtf16(t *testing.T) {
	got, err := CanonicalJSON([]byte(`{"c":1,"a":2,"b":3}`))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":2,"b":3,"c":1}`
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCanonical_ObjectKeyReorderProducesSameOutput(t *testing.T) {
	in1 := []byte(`{"z":1,"a":2,"m":3}`)
	in2 := []byte(`{"a":2,"m":3,"z":1}`)
	in3 := []byte(`{"m":3,"z":1,"a":2}`)
	c1, err := CanonicalJSON(in1)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := CanonicalJSON(in2)
	if err != nil {
		t.Fatal(err)
	}
	c3, err := CanonicalJSON(in3)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(c1, c2) || !bytes.Equal(c2, c3) {
		t.Errorf("reordered inputs produce different canonical outputs:\n  c1=%q\n  c2=%q\n  c3=%q", c1, c2, c3)
	}
	want := `{"a":2,"m":3,"z":1}`
	if string(c1) != want {
		t.Errorf("got %q, want %q", c1, want)
	}
}

func TestCanonical_NestedObjectsSortedRecursively(t *testing.T) {
	in := []byte(`{"outer_b":{"y":1,"x":2},"outer_a":{"d":1,"c":2}}`)
	got, err := CanonicalJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"outer_a":{"c":2,"d":1},"outer_b":{"x":2,"y":1}}`
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCanonical_ArraysPreserveOrder(t *testing.T) {
	in := []byte(`[3,1,2,"z","a"]`)
	got, err := CanonicalJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	want := `[3,1,2,"z","a"]`
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCanonical_EmptyObjectAndArray(t *testing.T) {
	got1, _ := CanonicalJSON([]byte("{}"))
	if string(got1) != "{}" {
		t.Errorf("empty object got %q", got1)
	}
	got2, _ := CanonicalJSON([]byte("[]"))
	if string(got2) != "[]" {
		t.Errorf("empty array got %q", got2)
	}
}

func TestCanonical_StringEscapes(t *testing.T) {
	// Input: "a\"b\\c\nd\re\tf\bg\fh"
	in := []byte(`"a\"b\\c\nd\re\tf\bg\fh"`)
	got, err := CanonicalJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	want := `"a\"b\\c\nd\re\tf\bg\fh"`
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCanonical_ControlCharHexEscape(t *testing.T) {
	// JSON input uses \uXXXX escapes for control chars (literal 0x01/0x1F
	// bytes are illegal in JSON strings per RFC 8259 §7).
	// Canonical output must emit them as \u00XX lowercase hex
	// per RFC 8785 §3.2.2.2.
	in := []byte("\"\\u0001\\u001f\"")
	got, err := CanonicalJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	want := "\"\\u0001\\u001f\""
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCanonical_UnicodePassthroughNoEscape(t *testing.T) {
	// "é中" should emit as UTF-8 bytes verbatim.
	in := []byte(`"é中"`)
	got, err := CanonicalJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{'"', 0xC3, 0xA9, 0xE4, 0xB8, 0xAD, '"'}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestCanonical_DeeplyNestedGoldenFixture(t *testing.T) {
	in1 := []byte(`{"z_field":[1,2,{"nested_b":"v","nested_a":"u"}],"a_field":{"deep":{"c":3,"a":1,"b":2}},"m_field":null}`)
	in2 := []byte(`{"a_field":{"deep":{"b":2,"c":3,"a":1}},"m_field":null,"z_field":[1,2,{"nested_a":"u","nested_b":"v"}]}`)
	c1, err := CanonicalJSON(in1)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := CanonicalJSON(in2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(c1, c2) {
		t.Errorf("reordered nested inputs produced different canonical outputs:\n  c1=%s\n  c2=%s", c1, c2)
	}
	want := `{"a_field":{"deep":{"a":1,"b":2,"c":3}},"m_field":null,"z_field":[1,2,{"nested_a":"u","nested_b":"v"}]}`
	if string(c1) != want {
		t.Errorf("got %s\nwant %s", c1, want)
	}
}

func TestCanonical_Utf16CodeunitOrderingDiffersFromByteOrdering(t *testing.T) {
	// UTF-16 of U+FFFD = 0xFFFD; UTF-16 of U+1F600 = D83D DE00 (first unit
	// 0xD83D < 0xFFFD). So in UTF-16 order, 😀 sorts before U+FFFD.
	// In UTF-8 byte order, the opposite is true (F0 > EF), so a byte-wise
	// sort would diverge from RFC 8785 §3.2.3.
	in := []byte("{\"�\":1,\"\U0001F600\":2}")
	got, err := CanonicalJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	expectedPrefix := "{\"\U0001F600\":2"
	if len(s) < len(expectedPrefix) || s[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("UTF-16 ordering wrong: got %s", s)
	}
}

func TestCanonical_Idempotent(t *testing.T) {
	// Canonical output must be valid JSON and re-canonicalize to itself.
	in := []byte(`{"users":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}],"count":2}`)
	got, err := CanonicalJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := CanonicalJSON(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, got2) {
		t.Errorf("canonicalization not idempotent:\n  first=%s\n  second=%s", got, got2)
	}
}

// TestCanonical_ByteIdenticalToRustGolden verifies the Go canonical output
// matches the byte-for-byte exact output of the Rust implementation for the
// canonical golden fixture. This is the primary cross-language compatibility
// guarantee for signature verification. The Rust test
// `deeply_nested_golden_fixture` produces the same string.
func TestCanonical_ByteIdenticalToRustGolden(t *testing.T) {
	in := []byte(`{"z_field":[1,2,{"nested_b":"v","nested_a":"u"}],"a_field":{"deep":{"c":3,"a":1,"b":2}},"m_field":null}`)
	got, err := CanonicalJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	// Exact bytes the Rust test asserts:
	rustGolden := `{"a_field":{"deep":{"a":1,"b":2,"c":3}},"m_field":null,"z_field":[1,2,{"nested_a":"u","nested_b":"v"}]}`
	if string(got) != rustGolden {
		t.Errorf("Go output diverges from Rust golden:\n  go  =%s\n  rust=%s", got, rustGolden)
	}
}
