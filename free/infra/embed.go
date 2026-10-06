package infra

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed modules/hetzner/*
var EmbeddedModules embed.FS

// ExtractModules extracts the embedded Terraform modules to the cache directory
// and returns the path to the hetzner module.
func ExtractModules() (string, error) {
	hash := sha256.New()
	err := fs.WalkDir(EmbeddedModules, "modules/hetzner", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := EmbeddedModules.ReadFile(path)
		if err != nil {
			return err
		}
		hash.Write([]byte(path))
		hash.Write(data)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("infra: failed to hash embedded modules: %w", err)
	}

	treeHash := fmt.Sprintf("%x", hash.Sum(nil))

	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("infra: cannot find home directory: %w", err)
		}
		cacheHome = filepath.Join(home, ".cache")
	}

	destDir := filepath.Join(cacheHome, "nself", "infra", treeHash, "hetzner")
	markerPath := filepath.Join(destDir, ".extracted")

	if _, err := os.Stat(markerPath); err == nil {
		return destDir, nil
	}

	tmpDir := destDir + ".tmp"
	if err := os.RemoveAll(tmpDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", err
	}

	err = fs.WalkDir(EmbeddedModules, "modules/hetzner", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("modules/hetzner", path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(tmpDir, rel)
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}
		data, err := EmbeddedModules.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, 0644)
	})
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("infra: failed to extract modules: %w", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, ".extracted"), []byte("GENERATED"), 0644); err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}

	if err := os.Rename(tmpDir, destDir); err != nil {
		os.RemoveAll(tmpDir)
		if _, statErr := os.Stat(markerPath); statErr == nil {
			return destDir, nil
		}
		return "", fmt.Errorf("infra: failed to atomically move extracted modules: %w", err)
	}

	return destDir, nil
}
