// Package render provides config-rendering helpers for the monitoring plugin.
//
// Purpose: Copy monitoring config templates from a source directory tree to an
//          output directory. Designed for use in init containers that prepare
//          configs before Prometheus/Grafana/Loki compose services start.
// Inputs:  src dir (config templates), out dir (render destination).
// Outputs: Copied files mirroring the src tree structure under out.
// Constraints: No network calls. All errors are wrapped with context.
// SPORT: PLUGINS-MONITORING-000
package render

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// RenderConfigs copies all files from src to out, creating subdirectories as
// needed. This is the render pass — env var substitution is applied per-file
// by copyFile.
func RenderConfigs(src, out string) error {
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

		return CopyFile(path, dst)
	})
}

// CopyFile copies a single file from src to dst.
func CopyFile(src, dst string) error {
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

// EnvStr returns the value of the named environment variable, or fallback if
// the variable is unset or empty.
func EnvStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
