package detect

import (
	"strings"

	"gopkg.in/yaml.v3"
)

var composeNames = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

func detectCompose(s Snapshot) []Fact {
	var out []Fact
	for _, name := range composeNames {
		for _, p := range s.Paths(name) {
			b, err := s.read(p)
			if err != nil {
				out = append(out, unknown("compose", p, "unreadable compose file"))
				continue
			}
			var doc struct {
				Services map[string]struct {
					Image string `yaml:"image"`
					Build any    `yaml:"build"`
				} `yaml:"services"`
			}
			if yaml.Unmarshal(b, &doc) != nil || doc.Services == nil {
				out = append(out, unknown("compose", p, "invalid compose services"))
				continue
			}
			out = append(out, fact("container", "compose", p))
			for service, def := range doc.Services {
				if service == "" {
					continue
				}
				name := service
				if strings.Contains(def.Image, "${") {
					out = append(out, unknown("service", p, "interpolated image for "+service))
					continue
				}
				out = append(out, fact("service", name, p))
			}
		}
	}
	return out
}
