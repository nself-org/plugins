package detect

import (
	"path"

	"gopkg.in/yaml.v3"
)

func detectFlutter(s Snapshot) []Fact {
	var out []Fact
	for _, p := range s.Paths("pubspec.yaml") {
		b, err := s.read(p)
		if err != nil {
			out = append(out, unknown("manifest", p, "unreadable pubspec.yaml"))
			continue
		}
		var manifest struct {
			Dependencies    map[string]any `yaml:"dependencies"`
			DevDependencies map[string]any `yaml:"dev_dependencies"`
		}
		if yaml.Unmarshal(b, &manifest) != nil {
			out = append(out, unknown("manifest", p, "invalid pubspec.yaml"))
			continue
		}
		name := "dart"
		if _, ok := manifest.Dependencies["flutter"]; ok {
			name = "flutter"
		}
		if _, ok := manifest.DevDependencies["flutter_test"]; ok {
			name = "flutter"
		}
		out = append(out, fact("stack", name, p))
		if name == "flutter" {
			out = append(out, fact("test_framework", "flutter test", p))
		}
		if s.has(path.Join(path.Dir(p), "analysis_options.yaml")) {
			out = append(out, fact("lint_tool", "dart analyze", path.Join(path.Dir(p), "analysis_options.yaml")))
		}
	}
	return out
}
