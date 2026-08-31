// Package auth provides RFC 8785 (JSON Canonicalization Scheme — JCS) serialization
// for signature verification material. Output is byte-identical to the Rust
// implementation at nclaw/core/src/sync/canonical.rs.
//
// Compliance:
//   - Object members sorted by UTF-16 code unit sequence of the member name (RFC 8785 §3.2.3)
//   - No insignificant whitespace (RFC 8785 §3.2.1)
//   - Numbers serialized per ECMA-262 7.1.12.1 (RFC 8785 §3.2.2.3)
//   - Strings escaped per RFC 8259 §7 with lowercase \u00XX hex (RFC 8785 §3.2.2.2)
//   - Arrays preserve insertion order (RFC 8785 §3.2.4)
package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"unicode/utf16"
)

// CanonicalJSON serializes the given JSON-encoded input into RFC 8785-canonical bytes.
//
// Input must be valid JSON. Output is deterministic and byte-identical across runs,
// languages, and Go versions for the same logical input. Used as the JSON payload
// portion of nclaw sync event signing material — must match
// nclaw/core/src/sync/canonical.rs byte-for-byte.
func CanonicalJSON(input []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("canonical: parse: %w", err)
	}
	var buf bytes.Buffer
	if err := writeValue(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CanonicalJSONValue serializes a Go interface{} (as produced by json.Unmarshal
// with UseNumber) into RFC 8785-canonical bytes.
func CanonicalJSONValue(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeValue(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeValue(buf *bytes.Buffer, v interface{}) error {
	switch val := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		writeString(buf, val)
	case json.Number:
		writeNumber(buf, val)
	case float64:
		writeFloat(buf, val)
	case int:
		buf.WriteString(strconv.FormatInt(int64(val), 10))
	case int64:
		buf.WriteString(strconv.FormatInt(val, 10))
	case uint64:
		buf.WriteString(strconv.FormatUint(val, 10))
	case []interface{}:
		buf.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeValue(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]interface{}:
		// RFC 8785 §3.2.3: sort object members by UTF-16 code unit sequence.
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			return utf16Less(keys[i], keys[j])
		})
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeString(buf, k)
			buf.WriteByte(':')
			if err := writeValue(buf, val[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("canonical: unsupported type %T", v)
	}
	return nil
}

// utf16Less compares two strings by UTF-16 code unit sequence (RFC 8785 §3.2.3).
//
// This is NOT equivalent to Go's default lexicographic byte comparison for
// strings containing characters outside the BMP, or for some BMP characters
// whose UTF-8 byte order differs from UTF-16 code unit order.
func utf16Less(a, b string) bool {
	au := utf16.Encode([]rune(a))
	bu := utf16.Encode([]rune(b))
	n := len(au)
	if len(bu) < n {
		n = len(bu)
	}
	for i := 0; i < n; i++ {
		if au[i] != bu[i] {
			return au[i] < bu[i]
		}
	}
	return len(au) < len(bu)
}

// writeString emits a JSON string per RFC 8785 §3.2.2.2 / RFC 8259 §7.
//
// Required escapes: \" \\ \b \f \n \r \t, plus \u00XX (lowercase hex) for
// U+0000..U+001F. All other characters emit as UTF-8 verbatim.
func writeString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(buf, `\u%04x`, r)
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
}

// writeNumber emits a json.Number per ECMA-262 7.1.12.1 / RFC 8785 §3.2.2.3.
//
// Integers (parseable as int64 or uint64) emit without a decimal point. Floats
// route through writeFloat which handles integer-valued floats (100.0 → "100")
// and negative-zero normalization.
func writeNumber(buf *bytes.Buffer, n json.Number) {
	s := n.String()
	// Try int64.
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		buf.WriteString(strconv.FormatInt(i, 10))
		return
	}
	// Try uint64 (for values > MaxInt64).
	if u, err := strconv.ParseUint(s, 10, 64); err == nil {
		buf.WriteString(strconv.FormatUint(u, 10))
		return
	}
	// Float path.
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		// Should not happen for valid JSON; emit verbatim defensively.
		buf.WriteString(s)
		return
	}
	writeFloat(buf, f)
}

// writeFloat emits an f64 per ECMA-262 7.1.12.1 / RFC 8785 §3.2.2.3.
//
// Rules matching the Rust implementation:
//   - Negative zero (-0.0) emits as "0"
//   - Integer-valued finite floats (|f| < 1e16) emit as integers ("100" not "100.0")
//   - Otherwise emit via Go's shortest-round-trip format ('g' with -1 precision),
//     which is ECMA-262-conformant for finite values.
//
// JSON forbids NaN / ±Infinity per RFC 8259, so json.Decoder will reject them
// upstream. We treat them defensively as their string form for parity.
func writeFloat(buf *bytes.Buffer, f float64) {
	if f == 0.0 {
		buf.WriteByte('0')
		return
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		buf.WriteString(strconv.FormatFloat(f, 'g', -1, 64))
		return
	}
	// Integer-valued floats print as integers (matches Rust path).
	if f == math.Trunc(f) && math.Abs(f) < 1e16 {
		buf.WriteString(strconv.FormatInt(int64(f), 10))
		return
	}
	buf.WriteString(strconv.FormatFloat(f, 'g', -1, 64))
}
