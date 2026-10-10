package render

import (
	"fmt"
	"strings"
)

// Refusal names one model value the renderer will not guess at.
type Refusal struct{ Route, Field, Reason string }

// RefusalError is returned by Render when any refusal exists; no output is produced.
type RefusalError struct{ Items []Refusal }

func (e *RefusalError) Error() string {
	var b strings.Builder
	for i, r := range e.Items {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "refused: route %s: %s: %s", r.Route, r.Field, r.Reason)
	}
	return b.String()
}
