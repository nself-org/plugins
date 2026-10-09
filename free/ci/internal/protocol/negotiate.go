package protocol

import (
	"strconv"
	"strings"
)

// Negotiate selects the highest version both peers support and enforces the
// coordinator's minimum agent release. It returns a wire-ready rejection.
func Negotiate(agentVersions, coordinatorVersions []int, agentVersion, minAgentVersion string) (int, *Reject) {
	if _, ok := parseVersion(agentVersion); !ok {
		return 0, &Reject{CodeVersion, "invalid_agent_version", minAgentVersion}
	}
	if _, ok := parseVersion(minAgentVersion); !ok {
		return 0, &Reject{CodeVersion, "invalid_min_agent_version", minAgentVersion}
	}
	if compareVersion(agentVersion, minAgentVersion) < 0 {
		return 0, &Reject{CodeVersion, "agent_version_too_old", minAgentVersion}
	}
	best := 0
	for _, a := range agentVersions {
		for _, c := range coordinatorVersions {
			if a == c && a == 1 && a > best {
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
	x, _ := parseVersion(a)
	y, _ := parseVersion(b)
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

func parseVersion(s string) ([3]uint64, bool) {
	var out [3]uint64
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return out, false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return out, false
			}
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// AcceptHello gates the welcome on the node's pinned age recipient.
func AcceptHello(hello Hello, pinned *string, coordinatorVersions []int, minAgentVersion string, sessionID string, heartbeatMS, ttlMS int) (any, error) {
	if reject := CheckAgeRecipient(hello, pinned); reject != nil {
		return *reject, nil
	}
	version, reject := Negotiate(hello.Versions, coordinatorVersions, hello.AgentVersion, minAgentVersion)
	if reject != nil {
		return *reject, nil
	}
	return Welcome{Version: version, MinAgentVersion: minAgentVersion, SessionID: sessionID, HeartbeatMS: heartbeatMS, AgentTTLMS: ttlMS}, nil
}

// WriteHelloResponse writes exactly one reply after checking the age pin.
func WriteHelloResponse(enc *Encoder, hello Hello, pinned *string, coordinatorVersions []int, minAgentVersion, sessionID string, heartbeatMS, ttlMS int) error {
	body, err := AcceptHello(hello, pinned, coordinatorVersions, minAgentVersion, sessionID, heartbeatMS, ttlMS)
	if err != nil {
		return err
	}
	kind := "welcome"
	if _, rejected := body.(Reject); rejected {
		kind = "reject"
	}
	return enc.Encode(Message{V: 1, Type: kind, Seq: 1, Body: body})
}

// CheckAgeRecipient rejects a changed node recipient before welcome.
func CheckAgeRecipient(hello Hello, pinned *string) *Reject {
	if (hello.AgeRecipient == nil) != (pinned == nil) || (pinned != nil && *hello.AgeRecipient != *pinned) {
		return &Reject{"E660", "age_recipient_changed", ""}
	}
	return nil
}
