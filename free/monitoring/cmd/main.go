// monitoring plugin entrypoint — config-render init container.
//
// Purpose: Read monitoring config templates, substitute env vars, write rendered
//          configs to the output directory so the compose stack can start cleanly.
//          Designed to run as an init container before the Prometheus/Grafana/Loki
//          compose services launch.
//
// Inputs:  MONITORING_CONFIGS_SRC (default: /monitoring/configs)
//          MONITORING_CONFIGS_OUT (default: /monitoring/out)
//          PROMETHEUS_SCRAPE_INTERVAL (default: 15s)
// Outputs: Rendered config files in MONITORING_CONFIGS_OUT.
// Constraints: Exits 0 on success, non-zero on any I/O error.
// SPORT: PLUGINS-MONITORING-000
package main

import (
	"fmt"
	"os"

	"github.com/nself-org/plugins/free/monitoring/internal/render"
)

func main() {
	src := render.EnvStr("MONITORING_CONFIGS_SRC", "/monitoring/configs")
	out := render.EnvStr("MONITORING_CONFIGS_OUT", "/monitoring/out")

	if err := render.RenderConfigs(src, out); err != nil {
		fmt.Fprintf(os.Stderr, "monitoring: config render failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("monitoring: configs rendered to %s\n", out)
}
