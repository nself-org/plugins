package world

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Digest hashes canonical JSON. Integral capability float facts are normalized to JSON integers.
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
	data, err = normalize(data)
	if err != nil {
		return "", failed("normalize world", err)
	}
	canonical, err := trust.Canonical(data)
	if err != nil {
		return "", failed("canonical world", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
func normalize(v any) (any, error) {
	switch n := v.(type) {
	case json.Number:
		if _, err := strconv.ParseInt(string(n), 10, 64); err == nil {
			return n, nil
		}
		f, err := strconv.ParseFloat(string(n), 64)
		if err != nil || f != float64(int64(f)) {
			return nil, fmt.Errorf("non-integral number %s", n)
		}
		return json.Number(strconv.FormatInt(int64(f), 10)), nil
	case map[string]any:
		for k, item := range n {
			next, err := normalize(item)
			if err != nil {
				return nil, err
			}
			n[k] = next
		}
		return n, nil
	case []any:
		for i, item := range n {
			next, err := normalize(item)
			if err != nil {
				return nil, err
			}
			n[i] = next
		}
		return n, nil
	default:
		return v, nil
	}
}
