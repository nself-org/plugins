package gates

import (
	"encoding/json"
	"strings"
)

func advisoryOffline(id, output string, exit int) bool {
	if !strings.Contains(id, "trivy") || exit == 0 || json.Valid([]byte(output)) {
		return false
	}
	for _, line := range strings.Split(output, "\n") {
		s := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(s, "database download failed:") || strings.HasPrefix(s, "database not found:") || strings.HasPrefix(s, "fatal ") && strings.Contains(s, "database") && strings.Contains(s, "failed to download") {
			return true
		}
	}
	return false
}

func findings(id, output string) int {
	if strings.HasPrefix(id, "container.hadolint") {
		var rows []json.RawMessage
		if json.Unmarshal([]byte(output), &rows) == nil {
			return len(rows)
		}
	}
	if strings.Contains(id, "trivy") {
		var doc struct {
			Results []struct {
				Vulnerabilities   []json.RawMessage
				Misconfigurations []json.RawMessage
			}
		}
		if json.Unmarshal([]byte(output), &doc) == nil {
			n := 0
			for _, r := range doc.Results {
				n += len(r.Vulnerabilities) + len(r.Misconfigurations)
			}
			return n
		}
	}
	if id == "go.govulncheck" {
		n := 0
		for _, line := range strings.Split(output, "\n") {
			var row struct{ Finding json.RawMessage }
			if json.Unmarshal([]byte(line), &row) == nil && len(row.Finding) > 0 {
				n++
			}
		}
		return n
	}
	return 0
}
