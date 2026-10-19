package schedule

// Purpose: tests of the cron parser and its Next/Prev search (P7-HOST-20, acceptance line 1).
// Inputs:  expressions and instants built in the test (UTC).
// Outputs: pass or fail.
// Constraints: no clock, no disk, no network.

import (
	"strings"
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return t
}

// TestCronTable proves acceptance line 1: `*/15 * * * *`, `0 3 * * 1-5`, `30 2 1 */2 *` and
// `0 0 * * sun` produce the expected next times, and `61 * * * *`, `* * *` and `@daily` are
// rejected (here by Parse and, for the key, by Decode).
func TestCronTable(t *testing.T) {
	next := []struct {
		expr, from string
		want       []string
	}{
		{"*/15 * * * *", "2026-10-19 10:07", []string{"2026-10-19 10:15", "2026-10-19 10:30", "2026-10-19 10:45", "2026-10-19 11:00"}},
		{"*/15 * * * *", "2026-10-19 10:15", []string{"2026-10-19 10:30"}},
		{"0 3 * * 1-5", "2026-10-22 03:00", []string{"2026-10-23 03:00", "2026-10-26 03:00", "2026-10-27 03:00"}}, // Thu -> Fri -> Mon
		{"30 2 1 */2 *", "2026-10-19 00:00", []string{"2026-11-01 02:30", "2027-01-01 02:30", "2027-03-01 02:30"}},
		{"0 0 * * sun", "2026-10-19 12:00", []string{"2026-10-25 00:00", "2026-11-01 00:00"}},
		{"0 0 * * 7", "2026-10-19 12:00", []string{"2026-10-25 00:00"}},
		{"5,35 8-9 * jan-feb *", "2026-12-31 23:59", []string{"2027-01-01 08:05", "2027-01-01 08:35", "2027-01-01 09:05", "2027-01-01 09:35", "2027-01-02 08:05"}},
		{"10/20 * * * *", "2026-10-19 10:00", []string{"2026-10-19 10:10", "2026-10-19 10:30", "2026-10-19 10:50", "2026-10-19 11:10"}},
		{"0 0 29 2 *", "2026-10-19 00:00", []string{"2028-02-29 00:00", "2032-02-29 00:00"}},
		{"0 12 13 * fri", "2026-10-19 00:00", []string{"2026-10-23 12:00", "2026-10-30 12:00", "2026-11-06 12:00", "2026-11-13 12:00"}}, // day-of-month OR day-of-week
		{"  0   0  *  *  *  ", "2026-10-19 00:00", []string{"2026-10-20 00:00"}},
	}
	for _, c := range next {
		s, err := Parse(c.expr)
		if err != nil {
			t.Errorf("%q: %v", c.expr, err)
			continue
		}
		cur := at(c.from)
		for _, w := range c.want {
			cur = s.Next(cur)
			if !cur.Equal(at(w)) {
				t.Errorf("%q: Next = %s, want %s", c.expr, cur.Format("2006-01-02 15:04"), w)
				break
			}
			if !s.Matches(cur) || s.Prev(cur) != cur {
				t.Errorf("%q: %s does not match itself", c.expr, w)
			}
		}
	}
	prev := []struct{ expr, from, want string }{
		{"0 * * * *", "2026-10-19 10:59", "2026-10-19 10:00"},
		{"0 * * * *", "2026-10-19 10:00", "2026-10-19 10:00"},
		{"0 3 * * 1-5", "2026-10-25 12:00", "2026-10-23 03:00"},
		{"30 2 1 */2 *", "2026-10-19 00:00", "2026-09-01 02:30"},
		{"0 0 29 2 *", "2026-10-19 00:00", "2024-02-29 00:00"},
	}
	for _, c := range prev {
		s, err := Parse(c.expr)
		if err != nil {
			t.Fatal(err)
		}
		if got := s.Prev(at(c.from)); !got.Equal(at(c.want)) {
			t.Errorf("%q: Prev(%s) = %s, want %s", c.expr, c.from, got.Format("2006-01-02 15:04"), c.want)
		}
	}
	for _, bad := range []string{"61 * * * *", "* * *", "@daily", "", "* * * * * *", "*/0 * * * *", "-1 * * * *", "5-1 * * * *", "0 24 * * *", "0 0 0 * *",
		"0 0 32 * *", "0 0 * 13 *", "0 0 * * 8", "a * * * *", "1,,2 * * * *", "1- * * * *", "*/ * * * *", "0 0 31 2 *", "0 0 30 feb *", "? * * * *", "L * * * *",
		"0 0 * * mon#2", "+5 * * * *", "0x5 * * * *", strings.Repeat("1,", 200) + "1 * * * *"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	for _, bad := range []string{`[{"repo":"o/r","pipeline":"ci","cron":"61 * * * *"}]`, `[{"repo":"o/r","pipeline":"ci","cron":"* * *"}]`,
		`[{"repo":"o/r","pipeline":"ci","cron":"@daily"}]`, `[{"repo":"o/r","pipeline":"ci","cron":"0 0 * * *","tz":"UTC"}]`, `[{"repo":"nope","cron":"0 0 * * *"}]`,
		`[{"repo":"o/r","pipeline":"a b","cron":"0 0 * * *"}]`, `{"repo":"o/r","cron":"0 0 * * *"}`, `null`, `[]  []`, `"0 0 * * *"`,
		`[{"repo":"o/r","cron":"0 0 * * *"},{"repo":"o/r","cron":"0   0 * * *"}]`} {
		if _, err := Decode([]byte(bad)); err == nil || !strings.HasPrefix(err.Error(), "E672 ") {
			t.Errorf("%s: err = %v, want E672", bad, err)
		}
	}
	got, err := Decode([]byte(`[{"repo":"o/r","pipeline":"nightly","cron":"0 3 * * *"},{"repo":"o/r","cron":"*/15 * * * *"}]`))
	if err != nil || len(got.([]Entry)) != 2 {
		t.Fatalf("good value refused: %v", err)
	}
}

// FuzzCronParse: Parse never panics, and an expression it accepts has a next time that matches and
// lies after the start.
func FuzzCronParse(f *testing.F) {
	for _, s := range []string{"*/15 * * * *", "0 3 * * 1-5", "30 2 1 */2 *", "0 0 * * sun", "61 * * * *", "* * *", "@daily", "0 0 29 2 *", "1-5/2,7 * * jan-mar mon-fri", "9999999999 * * * *"} {
		f.Add(s)
	}
	start := at("2026-10-19 10:07")
	f.Fuzz(func(t *testing.T, expr string) {
		s, err := Parse(expr)
		if err != nil {
			return
		}
		n := s.Next(start)
		if n.IsZero() || !n.After(start) || !s.Matches(n) {
			t.Fatalf("%q: Next = %v", expr, n)
		}
		if p := s.Prev(n); !p.Equal(n) {
			t.Fatalf("%q: Prev(Next) = %v, want %v", expr, p, n)
		}
	})
}
