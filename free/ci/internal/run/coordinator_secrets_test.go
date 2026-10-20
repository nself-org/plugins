package run

import "testing"

func TestRequestsSecrets(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want bool
	}{{nil, false}, {[]string{}, false}, {[]string{"none/X"}, false}, {[]string{"project/TOKEN"}, true}, {[]string{"none/X", "release/KEY"}, true}, {[]string{"/X"}, false}} {
		if got := requestsSecrets(c.in); got != c.want {
			t.Errorf("requestsSecrets(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
