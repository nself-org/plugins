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
	if w.Policy.Preset == sched.AutoPreset { // the only preset whose placement reads the policy keys the view adds
		if err = addPolicyView(data, w); err != nil {
			return "", err
		}
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

// addPolicyView puts the effective trust policy into the hashed data. Policy.Trust keeps
// its fields unexported, so json.Marshal wrote it as {}: two worlds that differ only in a
// policy value placement reads (sched.auto.step_sec, sched.auto.order)
// had one digest. The explicit Snapshot covers every effective key and its source. Only
// `auto` worlds carry it, so the digest of every other preset is unchanged.
func addPolicyView(data any, w sched.World) error {
	root, ok := data.(map[string]any)
	if !ok {
		return failed("world is not an object", nil)
	}
	pol, ok := root["Policy"].(map[string]any)
	if !ok {
		return failed("world has no policy object", nil)
	}
	raw, err := json.Marshal(w.Policy.Trust.Snapshot())
	if err != nil {
		return failed("marshal policy", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var view any
	if err = dec.Decode(&view); err != nil {
		return failed("decode policy", err)
	}
	pol["Trust"] = view
	return nil
}
