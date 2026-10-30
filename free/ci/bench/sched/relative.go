package sched

import "fmt"

// CheckTinyRatio checks the ticket's local fast-path median allowance.
// Statistics are recomputed from samples so a stale summary cannot mask a miss.
func CheckTinyRatio(observations []Observation) error {
	values := make(map[string]float64)
	for _, observation := range observations {
		if observation.Name != "ci.e2e_tiny.v14" && observation.Name != "ci.e2e_tiny.v15" {
			continue
		}
		if _, exists := values[observation.Name]; exists {
			return fmt.Errorf("duplicate metric %s", observation.Name)
		}
		if observation.Unit != "ms" || len(observation.Samples) != 10 {
			return fmt.Errorf("%s requires ten millisecond samples", observation.Name)
		}
		summary, err := Summarize(observation.Name, observation.Samples)
		if err != nil {
			return err
		}
		values[observation.Name] = summary.Median
	}
	legacy, ok := values["ci.e2e_tiny.v14"]
	if !ok {
		return fmt.Errorf("missing ci.e2e_tiny.v14")
	}
	v15, ok := values["ci.e2e_tiny.v15"]
	if !ok {
		return fmt.Errorf("missing ci.e2e_tiny.v15")
	}
	limit := legacy*1.10 + 250
	if v15 > limit {
		return fmt.Errorf("ci.e2e_tiny.v15 median %.6f ms exceeds %.6f ms", v15, limit)
	}
	return nil
}
