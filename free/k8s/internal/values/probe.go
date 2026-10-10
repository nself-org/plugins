// Purpose: convert a compose healthcheck into an exec readiness probe.
//
// Inputs: the resolved healthcheck, the running NotMapped list of the service.
//
// Outputs: *Probe (nil when the check is disabled or unusable) and the
// NotMapped list, extended when the healthcheck could not be mapped.
//
// Constraints: compose defaults (interval 30s, timeout 30s, retries 3,
// start_period 0) apply to empty fields; durations round up to whole seconds.
package values

import (
	"time"
)

func mapProbe(h *ComposeHealth, notMapped []string) (*Probe, []string) {
	if h == nil || h.Disable || len(h.Test) == 0 || h.Test[0] == "NONE" {
		return nil, notMapped
	}
	var cmd []string
	switch h.Test[0] {
	case "CMD":
		cmd = h.Test[1:]
	case "CMD-SHELL":
		if len(h.Test) < 2 {
			return nil, append(notMapped, "healthcheck:empty CMD-SHELL")
		}
		cmd = []string{"sh", "-c", h.Test[1]}
	default:
		cmd = h.Test
	}
	if len(cmd) == 0 {
		return nil, append(notMapped, "healthcheck:empty command")
	}
	p := &Probe{Exec: cmd, Retries: h.Retries}
	if p.Retries <= 0 {
		p.Retries = 3
	}
	var err error
	if p.PeriodS, err = seconds(h.Interval, 30, 1); err != nil {
		return nil, append(notMapped, "healthcheck:bad interval")
	}
	if p.TimeoutS, err = seconds(h.Timeout, 30, 1); err != nil {
		return nil, append(notMapped, "healthcheck:bad timeout")
	}
	if p.StartPeriodS, err = seconds(h.StartPeriod, 0, 0); err != nil {
		return nil, append(notMapped, "healthcheck:bad start_period")
	}
	return p, notMapped
}

// seconds parses a compose duration, rounds up, and applies a default for
// the empty string and a floor.
func seconds(s string, def, floor int) (int, error) {
	if s == "" {
		return def, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	n := int((d + time.Second - 1) / time.Second)
	if n < floor {
		n = floor
	}
	return n, nil
}
