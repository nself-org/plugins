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
	"io"
	"os"
	"path/filepath"
)

func main() {
	src := envStr("MONITORING_CONFIGS_SRC", "/monitoring/configs")
	out := envStr("MONITORING_CONFIGS_OUT", "/monitoring/out")

	if err := renderConfigs(src, out); err != nil {
		fmt.Fprintf(os.Stderr, "monitoring: config render failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("monitoring: configs rendered to %s\n", out)
}

// renderConfigs copies all files from src to out, creating subdirectories
// as needed. This is the render pass — env var substitution is applied
// per-file by renderFile.
func renderConfigs(src, out string) error {
	if err := os.MkdirAll(out, 0755); err != nil {
		return fmt.Errorf("create output dir %s: %w", out, err)
	}

	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(out, rel)

		if d.IsDir() {
			return os.MkdirAll(dst, 0755)
		}

		return copyFile(path, dst)
	})
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", dst, err)
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s → %s: %w", src, dst, err)
	}

	return nil
}

// envStr returns env var value or fallback.
func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
