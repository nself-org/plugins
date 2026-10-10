// Purpose: extract the embedded Helm chart to a private temp directory that
// helm can read.
//
// Inputs: an fs.FS holding the chart (the plugin's embed.FS) and the chart
// root inside it.
//
// Outputs: the path of the extracted chart and a cleanup func that removes it.
//
// Constraints: the directory is created by os.MkdirTemp (mode 0700) and the
// extracted files are written 0600 (directories 0700), so no other user can
// read the chart or the install overrides written next to it. Paths are taken
// from fs.WalkDir, which never yields ".." or absolute names.
package k8s

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// ExtractChart copies the tree at root inside chart to a new temp directory
// and returns that directory (the chart path to give helm) with a cleanup func.
func ExtractChart(chart fs.FS, root string) (dir string, cleanup func(), err error) {
	if chart == nil {
		return "", nil, fmt.Errorf("k8s: no embedded chart")
	}
	base, err := os.MkdirTemp("", "nself-k8s-chart-")
	if err != nil {
		return "", nil, fmt.Errorf("k8s: create chart dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(base) }
	dest := filepath.Join(base, "nself")
	werr := fs.WalkDir(chart, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := p[len(root):]
		if rel != "" {
			rel = rel[1:]
		}
		target := filepath.Join(dest, filepath.FromSlash(path.Clean(rel)))
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := fs.ReadFile(chart, p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
	if werr != nil {
		cleanup()
		return "", nil, fmt.Errorf("k8s: extract chart: %w", werr)
	}
	return dest, cleanup, nil
}

// parentDir returns the private temp directory that holds the extracted chart.
func parentDir(chartDir string) string { return filepath.Dir(chartDir) }
