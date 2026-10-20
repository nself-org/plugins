// Package schedule fires pipelines on a cron schedule from the TEAM coordinator (P7-HOST-20, Epic D10):
// the operator-scope key triggers.schedules, a 5-field cron parser and a coordinator Reconciler.
package schedule

// Purpose: the 5-field cron parser and its next/previous-time search.
// Inputs:  an expression "minute hour day-of-month month day-of-week" (UTC).
// Outputs: Spec with Next(t) (first match strictly after t) and Prev(t) (last match at or before t).
// Constraints: fields take `*`, a number, a range `a-b`, a step `*/n`, `a-b/n` or `a/n` (a to the field
//          maximum), and comma lists of those; month and day-of-week also take three-letter names
//          (jan..dec, sun..sat; day-of-week 7 means Sunday). No seconds, no `@daily` macros, no `?`, `L`
//          or `#`. When both day-of-month and day-of-week are restricted either may match (vixie cron);
//          a field starting with `*` counts as unrestricted. An expression that can never match (31
//          February) is refused. Pure stdlib; no clock, no goroutine.

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// maxExprLen bounds the expression the parser reads.
const maxExprLen = 256

// searchYears bounds how far Next and Prev look; 31 February style expressions hit it.
const searchYears = 9

type fieldDef struct {
	name     string
	min, max int
	names    []string // names[i] stands for min+i
}

var (
	minuteDef = fieldDef{name: "minute", min: 0, max: 59}
	hourDef   = fieldDef{name: "hour", min: 0, max: 23}
	domDef    = fieldDef{name: "day-of-month", min: 1, max: 31}
	monthDef  = fieldDef{name: "month", min: 1, max: 12, names: []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}}
	dowDef    = fieldDef{name: "day-of-week", min: 0, max: 7, names: []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}}
)

// Spec is a parsed cron expression.
type Spec struct {
	expr                  string
	minute                uint64
	hour, dom, month, dow uint32
	domStar, dowStar      bool
}

// String returns the expression with single spaces between the fields.
func (s Spec) String() string { return s.expr }

// Parse reads a 5-field cron expression.
func Parse(expr string) (Spec, error) {
	if len(expr) > maxExprLen {
		return Spec{}, fmt.Errorf("cron: expression longer than %d bytes", maxExprLen)
	}
	f := strings.Fields(expr)
	if len(f) != 5 {
		return Spec{}, fmt.Errorf("cron: want 5 fields (minute hour day-of-month month day-of-week), got %d", len(f))
	}
	var s Spec
	var err error
	var minute, hour, dom, month, dow uint64
	for i, p := range []struct {
		def  fieldDef
		into *uint64
	}{{minuteDef, &minute}, {hourDef, &hour}, {domDef, &dom}, {monthDef, &month}, {dowDef, &dow}} {
		if *p.into, err = parseField(f[i], p.def); err != nil {
			return Spec{}, err
		}
	}
	if dow&(1<<7) != 0 { // 7 is Sunday
		dow = dow&^(1<<7) | 1
	}
	s = Spec{expr: strings.Join(f, " "), minute: minute, hour: uint32(hour), dom: uint32(dom), month: uint32(month), dow: uint32(dow),
		domStar: strings.HasPrefix(f[2], "*"), dowStar: strings.HasPrefix(f[4], "*")}
	if s.Next(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)).IsZero() {
		return Spec{}, fmt.Errorf("cron: %q never matches a date", s.expr)
	}
	return s, nil
}

// parseField returns the bit set of one field; bit i is value i.
func parseField(text string, d fieldDef) (uint64, error) {
	var set uint64
	for _, part := range strings.Split(text, ",") {
		bits, err := parsePart(part, d)
		if err != nil {
			return 0, err
		}
		set |= bits
	}
	return set, nil
}

func parsePart(part string, d fieldDef) (uint64, error) {
	bad := func(why string) (uint64, error) {
		return 0, fmt.Errorf("cron: %s field %q: %s", d.name, part, why)
	}
	rng, stepText, hasStep := strings.Cut(part, "/")
	step := 1
	if hasStep {
		n, err := atoi(stepText)
		if err != nil || n < 1 {
			return bad("step must be a positive number")
		}
		step = n
	}
	lo, hi := d.min, d.max
	switch {
	case rng == "*":
		if d.max == 7 { // 7 is only an alias of 0 for an explicit value
			hi = 6
		}
	case strings.Contains(rng, "-"):
		a, b, _ := strings.Cut(rng, "-")
		var err1, err2 error
		lo, err1 = value(a, d)
		hi, err2 = value(b, d)
		if err1 != nil || err2 != nil {
			return bad("value out of range or not understood")
		}
		if lo > hi {
			return bad("range runs backwards")
		}
	default:
		v, err := value(rng, d)
		if err != nil {
			return bad("value out of range or not understood")
		}
		lo, hi = v, v
		if hasStep { // a/n runs from a to the field maximum
			hi = d.max
		}
	}
	var bits uint64
	for v := lo; v <= hi; v += step {
		bits |= 1 << uint(v)
	}
	return bits, nil
}

// atoi accepts plain decimal digits only (no sign, no space).
func atoi(s string) (int, error) {
	if s == "" || len(s) > 3 {
		return 0, fmt.Errorf("not a number")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a number")
		}
	}
	return strconv.Atoi(s)
}

// value reads a number or a three-letter name of the field.
func value(s string, d fieldDef) (int, error) {
	if n, err := atoi(s); err == nil {
		if n < d.min || n > d.max {
			return 0, fmt.Errorf("out of range")
		}
		return n, nil
	}
	for i, name := range d.names {
		if strings.EqualFold(s, name) {
			return d.min + i, nil
		}
	}
	return 0, fmt.Errorf("unknown value")
}

func has(set uint64, v int) bool { return set&(1<<uint(v)) != 0 }

// dayMatches applies the day-of-month / day-of-week rule.
func (s Spec) dayMatches(t time.Time) bool {
	dom := has(uint64(s.dom), t.Day())
	dow := has(uint64(s.dow), int(t.Weekday()))
	if !s.domStar && !s.dowStar {
		return dom || dow
	}
	return dom && dow
}

// Matches reports whether t (to the minute, in UTC) is a scheduled time.
func (s Spec) Matches(t time.Time) bool {
	t = t.UTC()
	return has(uint64(s.month), int(t.Month())) && s.dayMatches(t) && has(uint64(s.hour), t.Hour()) && has(s.minute, t.Minute())
}

// Next returns the first scheduled time strictly after t, or the zero time when none falls within
// nine years.
func (s Spec) Next(t time.Time) time.Time {
	t = t.UTC().Truncate(time.Minute).Add(time.Minute)
	limit := t.Year() + searchYears
	for t.Year() <= limit {
		switch {
		case !has(uint64(s.month), int(t.Month())):
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		case !s.dayMatches(t):
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, time.UTC)
		case !has(uint64(s.hour), t.Hour()):
			t = t.Truncate(time.Hour).Add(time.Hour)
		case !has(s.minute, t.Minute()):
			t = t.Add(time.Minute)
		default:
			return t
		}
	}
	return time.Time{}
}

// Prev returns the last scheduled time at or before t, or the zero time when none falls within nine
// years.
func (s Spec) Prev(t time.Time) time.Time {
	t = t.UTC().Truncate(time.Minute)
	limit := t.Year() - searchYears
	for t.Year() >= limit {
		switch {
		case !has(uint64(s.month), int(t.Month())):
			t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).Add(-time.Minute)
		case !s.dayMatches(t):
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Add(-time.Minute)
		case !has(uint64(s.hour), t.Hour()):
			t = t.Truncate(time.Hour).Add(-time.Minute)
		case !has(s.minute, t.Minute()):
			t = t.Add(-time.Minute)
		default:
			return t
		}
	}
	return time.Time{}
}
