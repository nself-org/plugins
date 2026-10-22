package clientapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/roles"
)

func TestRoleDenialsAreLimitedPerActor(t *testing.T) {
	if err := RegisterRoute("POST", "/v1/test-denial-limit", "admin", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("denied handler ran") })); err != nil {
		t.Fatal(err)
	}
	r := newRig(t)
	c := r.client()
	for _, actor := range []string{"limit-alice", "limit-bob"} {
		token := r.token(actor, roles.Member, time.Hour, "acme/app")
		for i := 0; i <= authFailMax; i++ {
			status, _ := r.do(c, "POST", "/v1/test-denial-limit", token, "")
			want := http.StatusForbidden
			if i == authFailMax {
				want = http.StatusTooManyRequests
			}
			if status != want {
				t.Fatalf("actor %s attempt %d: status %d, want %d", actor, i, status, want)
			}
		}
	}
}
