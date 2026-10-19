package bootstrap

// Purpose: retire the job users of slots a node no longer has (TestBootstrapSlots): fewer slots on a
// re-run remove a surplus user only when it owns no file; its sudoers rule goes either way, so a kept
// user is an inert account, never a way to run the shim.
// Inputs:  the users that exist on the node and the new slot count.
// Outputs: Logf lines; an E669 when a step fails.
// Constraints: only accounts named nself-ci-job-<number> are looked at; a name with a leading zero or an
//          out-of-range number is ignored, never passed to a script.

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

var jobUserRe = regexp.MustCompile(`^` + AgentUser + `-job-([1-9][0-9]{0,2})$`)

// surplusSlots returns the slot numbers above slots among the listed user names, ascending and unique.
func surplusSlots(listed string, slots int) []int {
	seen := map[int]bool{}
	var out []int
	for _, name := range strings.Fields(listed) {
		m := jobUserRe.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if n > slots && n <= 999 && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func (f *pkgFamily) retireSurplus(ctx context.Context, el *elevated, o Options) error {
	listed, err := el.sh(ctx, "list-users", listJobUsersScript())
	if err != nil {
		return err
	}
	for _, n := range surplusSlots(listed, o.Slots) {
		out, rerr := el.sh(ctx, "retire-slot", removeSlotScript(n))
		if rerr != nil {
			return rerr
		}
		o.Logf("bootstrap slots: %s %s", JobUser(n), lastLine(out))
	}
	return nil
}
