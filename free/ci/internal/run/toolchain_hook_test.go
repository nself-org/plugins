package run

import (
	"testing"
	"time"
)

func TestDockerTimeoutLongOnlyForPull(t *testing.T) {
	cases := []struct {
		argv []string
		want time.Duration
	}{
		{[]string{"docker", "pull", "--quiet", "x@sha256:abc"}, pullTimeout},
		{[]string{"docker", "image", "inspect", "--format", "{{.Id}}", "x"}, inspectTimeout},
		{[]string{"docker", "info"}, inspectTimeout},
		{[]string{"docker"}, inspectTimeout},
		{nil, inspectTimeout},
	}
	for _, c := range cases {
		if got := dockerTimeout(c.argv); got != c.want {
			t.Errorf("%v: %s, want %s", c.argv, got, c.want)
		}
	}
	if pullTimeout <= 30*time.Second {
		t.Error("pull must outlive report.Capture's 30 s limit")
	}
}
