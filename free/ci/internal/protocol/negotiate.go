package protocol

import "strings"

// Negotiate selects the highest version both peers support and enforces the
// coordinator's minimum agent release. It returns a wire-ready rejection.
func Negotiate(agentVersions, coordinatorVersions []int, agentVersion, minAgentVersion string) (int, *Reject) {
	if compareVersion(agentVersion, minAgentVersion) < 0 {
		return 0, &Reject{CodeVersion, "agent_version_too_old", minAgentVersion}
	}
	best := 0
	for _, a := range agentVersions {
		for _, c := range coordinatorVersions {
			if a == c && a > best {
				best = a
			}
		}
	}
	if best == 0 {
		return 0, &Reject{CodeVersion, "no_common_version", minAgentVersion}
	}
	return best, nil
}

func compareVersion(a, b string) int {
	var x, y [3]int
	for i, s := range strings.SplitN(strings.TrimPrefix(a, "v"), ".", 3) {
		if i < 3 {
			for _, c := range s {
				if c < '0' || c > '9' {
					break
				}
				x[i] = x[i]*10 + int(c-'0')
			}
		}
	}
	for i, s := range strings.SplitN(strings.TrimPrefix(b, "v"), ".", 3) {
		if i < 3 {
			for _, c := range s {
				if c < '0' || c > '9' {
					break
				}
				y[i] = y[i]*10 + int(c-'0')
			}
		}
	}
	for i := range x {
		if x[i] < y[i] {
			return -1
		}
		if x[i] > y[i] {
			return 1
		}
	}
	return 0
}

// CheckAgeRecipient rejects a changed node recipient before welcome.
func CheckAgeRecipient(hello Hello, pinned *string) *Reject {
	if (hello.AgeRecipient == nil) != (pinned == nil) || (pinned != nil && *hello.AgeRecipient != *pinned) {
		return &Reject{"E660", "age_recipient_changed", ""}
	}
	return nil
}
