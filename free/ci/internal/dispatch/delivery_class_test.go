package dispatch

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func TestDeliveryFailureClass(t *testing.T) {
	for _, tc := range []struct {
		class      providers.ErrorClass
		code, want string
	}{
		{providers.Config, "E700", "code"}, {providers.Auth, "E701", "code"}, {providers.Transient, "E704", "infra"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			st := openStore(t)
			id := queueAttempt(t, st, "P", "J", "N")
			a, err := tryProbe(st, id, sched.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			d := &Dispatcher{cfg: Config{Store: st}, life: context.Background()}
			d.lose(a, fmt.Errorf("wrapped: %w", providers.NewError(tc.class, tc.code, "test", 0, time.Time{})))
			rows, err := st.PipelineAttempts(context.Background(), "P")
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].FailureClass != tc.want {
				t.Fatalf("attempts: %+v, want %s", rows, tc.want)
			}
		})
	}
}
