package world

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Digest hashes canonical JSON, including permitted fractional capability facts.
func Digest(w sched.World) (string, error) {
	raw, err := json.Marshal(w)
	if err != nil {
		return "", failed("marshal world", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var data any
	if err = dec.Decode(&data); err != nil {
		return "", failed("decode world", err)
	}
	canonical, err := canonicalWorld(data)
	if err != nil {
		return "", failed("canonical world", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// canonicalWorld delegates scalar escaping to trust.Canonical and writes numbers
// in the shortest decimal form that round-trips through a JSON float64.
func canonicalWorld(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := writeCanonicalWorld(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func writeCanonicalWorld(b *bytes.Buffer, v any) error {
	switch n := v.(type) {
	case json.Number:
		if _, err := strconv.ParseInt(string(n), 10, 64); err == nil {
			b.WriteString(string(n))
			return nil
		}
		f, err := strconv.ParseFloat(string(n), 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return failed("invalid world number", err)
		}
		if f == math.Trunc(f) && f >= math.MinInt64 && f < math.MaxInt64 {
			b.WriteString(strconv.FormatInt(int64(f), 10))
			return nil
		}
		b.WriteString(strconv.FormatFloat(f, 'g', -1, 64))
		return nil
	case map[string]any:
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			encoded, err := trust.Canonical(k)
			if err != nil {
				return err
			}
			b.Write(encoded)
			b.WriteByte(':')
			if err := writeCanonicalWorld(b, n[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
		return nil
	case []any:
		b.WriteByte('[')
		for i, item := range n {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeCanonicalWorld(b, item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
		return nil
	default:
		encoded, err := trust.Canonical(v)
		if err != nil {
			return err
		}
		b.Write(encoded)
		return nil
	}
}
