package rest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"sync"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// AuthFunc resolves a bearer token at call time.
type AuthFunc func(context.Context) (string, error)

func missingCredential(reason string) error {
	return providers.NewError(providers.Config, "E700", reason, 0, time.Time{})
}

// EnvToken resolves the named variable on demand and caches it for five minutes.
func EnvToken(name string) AuthFunc {
	var mu sync.Mutex
	var cached string
	var expires time.Time
	return func(context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if name == "" {
			return "", missingCredential("token_env")
		}
		if cached != "" && time.Now().Before(expires) {
			return cached, nil
		}
		token := strings.TrimSpace(os.Getenv(name))
		if token == "" {
			return "", missingCredential("env:" + name)
		}
		cached, expires = token, time.Now().Add(5*time.Minute)
		return cached, nil
	}
}

func randomID() string {
	var bytes [12]byte
	_, _ = rand.Read(bytes[:])
	return hex.EncodeToString(bytes[:])
}

// GHToken obtains gh's token through the CI command funnel and caches it for five minutes.
func GHToken(host string) AuthFunc {
	var mu sync.Mutex
	var token string
	var expires time.Time
	return func(ctx context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if token != "" && time.Now().Before(expires) {
			return token, nil
		}
		if host == "" || strings.ContainsAny(host, " /:@\t\n") {
			return "", missingCredential("gh.auth.login")
		}
		var output []byte
		tooLarge := false
		runner := &ciexec.Executor{}
		result := runner.Run(ctx, ciexec.JobSpec{
			AttemptID: "gh-auth-" + randomID(), CoordinatorID: "provider-auth", Command: []string{"gh", "auth", "token", "--hostname", host},
			Timeout: connectTimeout,
			Output: func(chunk []byte) {
				if len(output)+len(chunk) > 16*1024 {
					tooLarge = true
					return
				}
				output = append(output, chunk...)
			},
		})
		if result.Err != nil || result.ExitCode != 0 || tooLarge {
			return "", missingCredential("gh.auth.login")
		}
		candidate := strings.TrimSpace(string(output))
		if candidate == "" || strings.ContainsAny(candidate, "\r\n") {
			return "", missingCredential("gh.auth.login")
		}
		token, expires = candidate, time.Now().Add(5*time.Minute)
		return token, nil
	}
}
