package update

// Purpose: compare two agent release versions (x.y.z or x.y.z-pre) the way the upgrade decides its direction.
// Inputs:  two version strings that verify.ValidVersion accepts.
// Outputs: -1, 0 or 1; Direction of a move.
// Constraints: numeric parts compare as numbers; a pre-release is lower than its release; pre-release identifiers compare
//          per semver (numeric < alphanumeric, shorter is lower when equal so far).

import (
	"strconv"
	"strings"
)

// Direction is how the target version relates to the installed one.
type Direction int

// Directions.
const (
	Downgrade Direction = -1
	Same      Direction = 0
	Upgrade   Direction = 1
)

// Compare returns -1 when a < b, 0 when equal and 1 when a > b.
func Compare(a, b string) int {
	an, ap := split(a)
	bn, bp := split(b)
	for i := range an {
		if an[i] != bn[i] {
			if an[i] < bn[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case ap == "" && bp == "":
		return 0
	case ap == "":
		return 1
	case bp == "":
		return -1
	}
	return comparePre(strings.Split(ap, "."), strings.Split(bp, "."))
}

// DirectionOf is the direction of a move from current to target.
func DirectionOf(current, target string) Direction { return Direction(Compare(target, current)) }

func split(v string) ([3]int, string) {
	core, pre, _ := strings.Cut(v, "-")
	var n [3]int
	for i, p := range strings.SplitN(core, ".", 3) {
		n[i], _ = strconv.Atoi(p)
	}
	return n, pre
}

func comparePre(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			continue
		}
		ai, aerr := strconv.Atoi(a[i])
		bi, berr := strconv.Atoi(b[i])
		switch {
		case aerr == nil && berr == nil:
			if ai < bi {
				return -1
			}
			return 1
		case aerr == nil:
			return -1
		case berr == nil:
			return 1
		case a[i] < b[i]:
			return -1
		default:
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}
