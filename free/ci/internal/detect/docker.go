package detect

import "strings"

func detectDocker(s Snapshot) []Fact {
	var out []Fact
	for p := range s.files {
		base := p
		if i := strings.LastIndexByte(p, '/'); i >= 0 {
			base = p[i+1:]
		}
		if base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile.") || strings.HasSuffix(base, ".Dockerfile") {
			if _, err := s.read(p); err != nil {
				out = append(out, unknown("container", p, "unreadable Dockerfile"))
				continue
			}
			out = append(out, fact("container", "dockerfile", p))
		}
	}
	return out
}
