package adhoc

import (
	"context"
	"sort"
	"strings"
	"testing"
)

func TestEnvPolicyIsMinimal(t *testing.T) {
	host := map[string]string{"PATH": "/usr/bin:/bin", "LANG": "en_US.UTF-8", "HOME": "/Users/op", "GITHUB_TOKEN": "planted-token", "SSH_AUTH_SOCK": "/tmp/agent", "AWS_SECRET_ACCESS_KEY": "planted"}
	get := func(k string) string { return host[k] }
	keys := func(p envPolicy) []string {
		env, err := p.Environment(context.Background(), modelJob())
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, kv := range env {
			k, _, _ := strings.Cut(kv, "=")
			out = append(out, k)
			if strings.Contains(kv, "planted") || strings.Contains(kv, "/Users/op") {
				t.Errorf("host value leaked: %s", kv)
			}
		}
		sort.Strings(out)
		return out
	}
	if got := strings.Join(keys(envPolicy{scratch: "/scratch/s-1", getenv: get}), ","); got != "HOME,LANG,PATH,TMPDIR" {
		t.Fatalf("process env keys %s", got)
	}
	if got := strings.Join(keys(envPolicy{container: true, getenv: get}), ","); got != "HOME,LANG,TMPDIR" {
		t.Fatalf("container env keys %s (the image's PATH applies, the host PATH never does)", got)
	}
	env, _ := envPolicy{scratch: "/scratch/s-1", getenv: get}.Environment(context.Background(), modelJob())
	if !contains(env, "HOME=/scratch/s-1") || !contains(env, "TMPDIR=/scratch/s-1") {
		t.Fatalf("HOME and TMPDIR must be inside the scratch dir: %v", env)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
