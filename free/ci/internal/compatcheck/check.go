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
	for i := range actual {
		if actual[i] > minimum[i] {
			return nil
		}
		if actual[i] < minimum[i] {
			return fmt.Errorf("E600: nself %s requires %s; run nself update", version, requires)
		}
	}
	return nil
}

func semver(s string) ([3]int, error) {
	var out [3]int
	if n := strings.IndexAny(s, "-+"); n >= 0 {
		s = s[:n]
	}
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) < 1 || len(parts) > 3 {
		return out, fmt.Errorf("expected major[.minor[.patch]]")
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, fmt.Errorf("invalid semver component %q", part)
		}
		out[i] = n
	}
	return out, nil
}
