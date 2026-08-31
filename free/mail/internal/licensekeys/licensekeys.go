// Package licensekeys reimplements the one function of the CLI's
// internal/license package that the mail plugin needs: collecting
// configured license keys so mail.go can pick the first one to send to
// ping_api.
//
// Purpose: the plugin is its own Go module and cannot import
// github.com/nself-org/cli/internal/license (Go's internal/ visibility
// rule forbids it across module boundaries), so this file copies
// CollectLicenseKeys and its GetKey dependency byte-for-byte from
// internal/license/{keys.go,manager.go} (CLI-R11). Only the collection
// logic is copied — validation (ValidateKeyFormat, DetectProduct) is not,
// since mail.go never calls it: resolveMailClient only needs "is there at
// least one key" and "what is it," not "is it well-formed."
//
// Inputs: environment variables and the stored key file at
// ~/.nself/license/key.
//
// Outputs: CollectLicenseKeys returns all configured, deduplicated,
// non-empty keys in the same order core does: NSELF_PLUGIN_LICENSE_KEY,
// then NSELF_LICENSE_KEY_1..10, then the stored key file.
//
// Constraints: read-only — never writes the key file.
package licensekeys

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CollectLicenseKeys reads all configured license keys from environment
// variables and the stored key file, byte-for-byte matching
// internal/license.CollectLicenseKeys.
func CollectLicenseKeys() []string {
	seen := make(map[string]bool)
	var keys []string

	add := func(k string) {
		k = strings.TrimSpace(k)
		if k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}

	add(os.Getenv("NSELF_PLUGIN_LICENSE_KEY"))

	for i := 1; i <= 10; i++ {
		add(os.Getenv(fmt.Sprintf("NSELF_LICENSE_KEY_%d", i)))
	}

	if fileKey, err := getKey(); err == nil {
		add(fileKey)
	}

	return keys
}

// getKey is a byte-for-byte copy of internal/license.GetKey.
func getKey() (string, error) {
	if envKey := os.Getenv("NSELF_PLUGIN_LICENSE_KEY"); envKey != "" {
		return strings.TrimSpace(envKey), nil
	}

	dir, err := licenseDirPath()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(filepath.Join(dir, "key"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading license key: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// licenseDirPath mirrors internal/license's own resolution of
// ~/.nself/license.
func licenseDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".nself", "license"), nil
}
