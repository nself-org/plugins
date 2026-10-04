// gateway_env_test.go — table test for resolveGatewayBase.
package main

import (
	"strings"
	"testing"
)

func TestResolveGatewayBase(t *testing.T) {
	cases := []struct {
		name, env, gateway, envVar string
		want                       string
		wantErr                    bool
	}{
		{name: "all empty", want: ""},
		{name: "local preset", env: "local", want: "http://127.0.0.1:3761"},
		{name: "gateway wins over env", env: "local", gateway: "http://203.0.113.10:3761", want: "http://203.0.113.10:3761"},
		{name: "gateway wins over unknown env", env: "staging", gateway: "http://203.0.113.10:3761", want: "http://203.0.113.10:3761"},
		{name: "envVar only when both empty", envVar: "http://203.0.113.10:3761", want: "http://203.0.113.10:3761"},
		{name: "env wins over envVar", env: "local", envVar: "http://203.0.113.10:3761", want: "http://127.0.0.1:3761"},
		{name: "staging removed", env: "staging", wantErr: true},
		{name: "staging removed even with envVar", env: "staging", envVar: "http://203.0.113.10:3761", wantErr: true},
		{name: "unknown env", env: "prod", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveGatewayBase(c.env, c.gateway, c.envVar)
			if c.wantErr {
				if err == nil {
					t.Fatalf("want error, got %q", got)
				}
				for _, s := range []string{"--gateway", "NSELF_CI_GATEWAY"} {
					if !strings.Contains(err.Error(), s) {
						t.Errorf("error %q does not name %s", err, s)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
