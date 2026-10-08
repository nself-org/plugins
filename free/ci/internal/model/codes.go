package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Code is one registered machine-readable fabric error.
type Code struct {
	ID      string
	Class   string
	Summary string
	Fix     string
}
type registration struct {
	Code Code
	Site string
}

var codeMu sync.Mutex
var codes []registration

// Register records a code without panicking, leaving conflicts for CodeProblems.
func Register(code Code) {
	codeMu.Lock()
	defer codeMu.Unlock()
	codes = append(codes, registration{code, code.Summary})
}

// CodeProblems reports every duplicate and out-of-range registration with its owner.
func CodeProblems() []string { codeMu.Lock(); defer codeMu.Unlock(); return problems(codes) }

func problems(rows []registration) []string {
	var out []string
	seen := map[string]registration{}
	for _, row := range rows {
		id := row.Code.ID
		n, err := strconv.Atoi(strings.TrimPrefix(id, "E"))
		if err != nil || len(id) != 4 || !strings.HasPrefix(id, "E") || n < 600 || n > 719 {
			out = append(out, fmt.Sprintf("%s (%s): outside E600-E719", id, row.Site))
		}
		if first, ok := seen[id]; ok {
			out = append(out, fmt.Sprintf("%s: duplicate %s and %s", id, first.Site, row.Site))
		} else {
			seen[id] = row
		}
		if row.Code.Class == "" || row.Code.Summary == "" || row.Code.Fix == "" {
			out = append(out, id+": incomplete registration")
		}
	}
	sort.Strings(out)
	return out
}

// CodeRanges reserves disjoint fabric allocations.
var CodeRanges = map[string][2]int{"CI": {600, 649}, "NODE": {650, 669}, "TRUST": {670, 689}, "SCHED": {690, 699}, "HOST": {700, 709}, "CACHE": {710, 719}}

func init() {
	for _, c := range []Code{
		{"E600", "infra", "nself CLI is too old", "Run nself update"},
		{"E601", "usage", "document is invalid against its schema", "Fix the document and retry"},
		{"E602", "usage", "unsupported contract version", "Use a supported contract version"},
		{"E603", "infra", "V15 engine not built yet", "Install a newer nself-ci plugin"},
	} {
		Register(c)
	}
}
