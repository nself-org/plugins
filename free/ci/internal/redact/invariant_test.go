package redact

// TestInvariant_I06 (channels half): no registered value survives Wrap, over an
// exhaustive grid of encodings, embeddings and chunk splits plus >= 10000
// generated cases. The log line below is what the ticket verify greps.

import (
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
)

const i06Generated = 12000

func i06Seed() int64 {
	if s, err := strconv.ParseInt(os.Getenv("NSELF_CI_INVARIANT_SEED"), 10, 64); err == nil {
		return s
	}
	return 1701
}

// i06Check masks input and verifies the invariant; exact is the expected output
// for forms that leave nothing behind.
func i06Check(t *testing.T, r *Redactor, value, input, exact string, leakCap int, splits []int) {
	t.Helper()
	got := run(r, input)
	if strings.Contains(got, value) {
		t.Fatalf("value survived: input=%q out=%q", input, got)
	}
	if m := r.Scan([]byte(got)); len(m) != 0 {
		t.Fatalf("Scan finds a value in the output: input=%q out=%q", input, got)
	}
	if exact != "" && got != exact {
		t.Fatalf("input=%q\n got=%q\nwant=%q", input, got, exact)
	}
	if leakCap >= 0 {
		if rest, masked := leak(got, "", ""); !masked || len(rest) > leakCap {
			t.Fatalf("leak %d > %d or no mask: input=%q out=%q", len(rest), leakCap, input, got)
		}
	}
	for _, s := range splits {
		if s <= 0 || s >= len(input) {
			continue
		}
		if split := run(r, input[:s], input[s:]); collapse(split) != collapse(got) {
			t.Fatalf("split %d changes the output: input=%q\n whole=%q\n split=%q", s, input, got, split)
		}
	}
}

func TestInvariant_I06(t *testing.T) {
	r := New()
	mustRegister(t, r, secret)
	enumerated := 0
	every := func(n int) []int {
		out := make([]int, 0, n)
		for i := 1; i < n; i++ {
			out = append(out, i)
		}
		return out
	}
	for _, form := range plainForms(secret) {
		in := "pre " + form + " post"
		i06Check(t, r, secret, in, "pre [REDACTED] post", -1, every(len(in)))
		enumerated += len(in)
	}
	for _, prefix := range []string{"", "a", "ab", "abc", "user:", "Authorization: "} {
		for _, suffix := range []string{"", "z", "zz", "zzz"} {
			for _, form := range b64Embedded(prefix, secret, suffix) {
				in := "pre " + form + " post"
				i06Check(t, r, secret, in, "", (len(prefix)+len(suffix))*4/3+8+len("pre ")+len(" post"), every(len(in)))
				enumerated += len(in)
			}
		}
	}

	rnd := rand.New(rand.NewSource(i06Seed()))
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/+=&%\"<>'\\!-_.~ "
	generated := 0
	for generated < i06Generated {
		n := 6 + rnd.Intn(30)
		v := make([]byte, n)
		for i := range v {
			v[i] = alphabet[rnd.Intn(len(alphabet))]
		}
		v[0], v[n-1] = 'K', 'Z' // never all spaces or blank edges
		val := string(v)
		multi := rnd.Intn(5) == 0
		if multi {
			val = val[:n/2] + "\n" + val[n/2:]
		}
		rr := New()
		if err := rr.Register([]byte(val)); err != nil {
			t.Fatal(err)
		}
		var forms []string
		var exact bool
		switch {
		case multi:
			nl := []string{"\n", "\r\n"}[rnd.Intn(2)]
			forms = []string{strings.ReplaceAll(val, "\n", nl)}
		case rnd.Intn(2) == 0:
			pf := plainForms(val)
			keys := []string{"raw", "url", "url-lower", "url-path", "json", "json-plain", "u-lower", "u-upper", "hex", "hex-upper", "xesc"}
			forms, exact = []string{pf[keys[rnd.Intn(len(keys))]]}, true
		default:
			prefix := strings.Repeat("p", rnd.Intn(6))
			suffix := strings.Repeat("s", rnd.Intn(4))
			bf := b64Embedded(prefix, val, suffix)
			keys := []string{"b64", "b64-raw", "b64url", "b64url-raw", "b64-64", "b64-76"}
			forms = []string{bf[keys[rnd.Intn(len(keys))]]}
		}
		in := "x=" + forms[0] + "\nend"
		want := ""
		if exact {
			want = "x=[REDACTED]\nend"
		}
		var splits []int
		for k := 0; k < 3; k++ {
			splits = append(splits, 1+rnd.Intn(len(in)))
		}
		if multi {
			i06CheckMulti(t, rr, val, in, splits)
		} else {
			i06Check(t, rr, val, in, want, -1, splits)
		}
		generated++
	}
	t.Logf("INVARIANT id=I06 enumerated=%d generated=%d seed=%d", enumerated, generated, i06Seed())
}

// i06CheckMulti: a multi-line value leaves none of its lines (>= 6 bytes).
func i06CheckMulti(t *testing.T, r *Redactor, val, in string, splits []int) {
	t.Helper()
	got := run(r, in)
	for _, l := range strings.Split(val, "\n") {
		if l = strings.Trim(l, " \t"); len(l) >= MinValueLen && strings.Contains(got, l) {
			t.Fatalf("line %q survived in %q", l, got)
		}
	}
	for _, s := range splits {
		if split := run(r, in[:s], in[s:]); collapse(split) != collapse(got) {
			t.Fatalf("split %d changes the output: %q vs %q", s, split, got)
		}
	}
}
