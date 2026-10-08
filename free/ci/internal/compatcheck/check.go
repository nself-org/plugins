package compatcheck

import (
	"fmt"
	"strconv"
	"strings"
)

// RequiresNself is the v2 manifest range checked again at plugin startup.
const RequiresNself = ">=1.4.12"

// Check accepts an unset CLI version and rejects one below requires.
func Check(env func(string) string, requires string) error {
	version := env("NSELF_CLI_VERSION")
	if version == "" {
		return nil
	}
	if !strings.HasPrefix(requires, ">=") {
		return fmt.Errorf("E600: unsupported nself requirement %q; run nself update", requires)
	}
	actual, err := semver(version)
	if err != nil {
		return fmt.Errorf("E600: invalid nself CLI version %q; run nself update: %w", version, err)
	}
	minimum, err := semver(strings.TrimPrefix(requires, ">="))
	if err != nil {
		return fmt.Errorf("E600: invalid nself requirement: %w", err)
	}
	for i := range actual.parts {
		if actual.parts[i] > minimum.parts[i] {
			return nil
		}
		if actual.parts[i] < minimum.parts[i] {
			return fmt.Errorf("E600: nself %s requires %s; run nself update", version, requires)
		}
	}
	if actual.prerelease && !minimum.prerelease {
		return fmt.Errorf("E600: nself %s requires %s; run nself update", version, requires)
	}
	return nil
}

type parsedVersion struct {
	parts      [3]int
	prerelease bool
}

func semver(s string) (parsedVersion, error) {
	var out parsedVersion
	if n := strings.IndexByte(s, '+'); n >= 0 {
		if n == len(s)-1 {
			return out, fmt.Errorf("empty build metadata")
		}
		s = s[:n]
	}
	if n := strings.IndexByte(s, '-'); n >= 0 {
		if n == len(s)-1 {
			return out, fmt.Errorf("empty prerelease")
		}
		out.prerelease = true
		s = s[:n]
	}
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != 3 {
		return out, fmt.Errorf("expected major.minor.patch")
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, fmt.Errorf("invalid semver component %q", part)
		}
		out.parts[i] = n
	}
	return out, nil
}
