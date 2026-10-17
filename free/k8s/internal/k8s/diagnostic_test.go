package k8s

import (
	"strings"
	"testing"
)

func TestD0618SanitizeBeforeTruncation(t *testing.T) {
	secret := strings.Repeat("PRIVATE", 100)
	for _, text := range []string{"password=" + secret, "https://cluster.test/" + secret} {
		got := stderrTail([]byte(text))
		if strings.Contains(got, "PRIVATE") {
			t.Fatalf("truncation leaked a credential tail: %q", got)
		}
	}
}
