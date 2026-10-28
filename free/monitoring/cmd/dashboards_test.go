package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Socket-less cAdvisor panels must retain the cgroup id and cannot promise
// Docker metadata or per-container network data absent from the raw handler.
func TestProvisionedContainerDashboardUsesCgroupIdentity(t *testing.T) {
	root := "../configs/grafana/provisioning/dashboards"
	b, err := os.ReadFile(filepath.Join(root, "containers.json"))
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		UID    string `json:"uid"`
		Panels []struct {
			Title   string `json:"title"`
			Targets []struct {
				Expr   string `json:"expr"`
				Legend string `json:"legendFormat"`
			} `json:"targets"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	if d.UID == "" || len(d.Panels) != 2 {
		t.Fatalf("dashboard identity/panels: %+v", d)
	}
	for _, p := range d.Panels {
		if len(p.Targets) == 0 {
			t.Fatalf("empty panel %s", p.Title)
		}
		for _, target := range p.Targets {
			if !strings.Contains(target.Expr, "by (instance, id)") || !strings.Contains(target.Expr, `id!="/"`) || !strings.Contains(target.Legend, "{{id}}") {
				t.Fatalf("missing cgroup identity: %+v", target)
			}
			for _, unsupported := range []string{"name", "image", "container_network"} {
				if strings.Contains(target.Expr, unsupported) {
					t.Fatalf("unsupported socket-less query: %s", target.Expr)
				}
			}
		}
	}
	provider, err := os.ReadFile(filepath.Join(root, "dashboards.yml"))
	if err != nil || !strings.Contains(string(provider), "path: /etc/grafana/provisioning/dashboards") {
		t.Fatalf("dashboard provider: %s %v", provider, err)
	}
}
