// Purpose: the subset of `docker compose config --format json` that the k8s
// mapper reads. Docker Compose has already resolved ${VAR} interpolation,
// merged every -f file and applied every --env-file when this JSON is
// produced, so nothing here parses compose syntax.
//
// Inputs: the JSON bytes printed by `docker compose config --format json`.
//
// Outputs: Model (ParseModel).
//
// Constraints: unknown fields are ignored on purpose (compose adds fields);
// the mapper decides what to do with the ones it knows.
package values

import (
	"encoding/json"
	"fmt"
)

// Model is the resolved compose project.
type Model struct {
	Name     string                 `json:"name"`
	Services map[string]ComposeSvc  `json:"services"`
	Volumes  map[string]interface{} `json:"volumes"`
}

// ComposeSvc is one resolved compose service.
type ComposeSvc struct {
	Image       string             `json:"image"`
	Build       json.RawMessage    `json:"build"`
	Restart     string             `json:"restart"`
	Entrypoint  []string           `json:"entrypoint"`
	Command     []string           `json:"command"`
	Environment map[string]*string `json:"environment"`
	Ports       []ComposePort      `json:"ports"`
	Volumes     []ComposeMount     `json:"volumes"`
	Healthcheck *ComposeHealth     `json:"healthcheck"`
	NetworkMode string             `json:"network_mode"`
	Privileged  bool               `json:"privileged"`
	Devices     []json.RawMessage  `json:"devices"`
	Pid         string             `json:"pid"`
	Ipc         string             `json:"ipc"`
	ExtraHosts  json.RawMessage    `json:"extra_hosts"`
	Expose      []interface{}      `json:"expose"`
	Tmpfs       []string           `json:"tmpfs"`
	User        string             `json:"user"`
	ReadOnly    bool               `json:"read_only"`
	CapAdd      []string           `json:"cap_add"`
	CapDrop     []string           `json:"cap_drop"`
	SecurityOpt []string           `json:"security_opt"`
	Deploy      json.RawMessage    `json:"deploy"`
	DependsOn   json.RawMessage    `json:"depends_on"`
}

// ComposePort is a resolved port. Published is a string in compose JSON.
type ComposePort struct {
	Target    int    `json:"target"`
	Published string `json:"published"`
	Protocol  string `json:"protocol"`
}

// ComposeMount is a resolved volume entry.
type ComposeMount struct {
	Type     string `json:"type"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

// ComposeHealth is a resolved healthcheck; durations are Go-style strings.
type ComposeHealth struct {
	Test        []string `json:"test"`
	Interval    string   `json:"interval"`
	Timeout     string   `json:"timeout"`
	Retries     int      `json:"retries"`
	StartPeriod string   `json:"start_period"`
	Disable     bool     `json:"disable"`
}

// ParseModel decodes the compose JSON. It never echoes the input in errors:
// the document holds secret values.
func ParseModel(data []byte) (*Model, error) {
	var m Model
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("compose config output is not valid JSON (%d bytes)", len(data))
	}
	if len(m.Services) == 0 {
		return nil, fmt.Errorf("compose config output has no services")
	}
	return &m, nil
}
