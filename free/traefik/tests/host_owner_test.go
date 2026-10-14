package tests

import "testing"

// D-0782: the one-shot renderer must write both output binds as the host
// operator so an unprivileged build can remove them when restoring nginx.
func TestConfigRendererHostOwner(t *testing.T) {
	cfg := get(readYAML(t, "docker-compose.plugin.yml"), "services", "traefik-config").(map[string]any)
	if got := cfg["user"]; got != "${NSELF_HOST_UID}:${NSELF_HOST_GID}" {
		t.Errorf("traefik-config user = %v, want the host UID:GID exported by the CLI", got)
	}
	for _, want := range []string{
		"./.nself/generated/traefik:/dynamic",
		"./.nself/generated/traefik-static:/static",
	} {
		found := false
		for _, mount := range asList(cfg["volumes"]) {
			if mount == want {
				found = true
			}
		}
		if !found {
			t.Errorf("traefik-config must mount the host-owned output writable: %s", want)
		}
	}
	if cfg["read_only"] != true {
		t.Error("traefik-config root filesystem must remain read-only")
	}
	if drops := asList(cfg["cap_drop"]); len(drops) != 1 || drops[0] != "ALL" {
		t.Error("traefik-config must drop all capabilities")
	}
	if cfg["cap_add"] != nil {
		t.Error("traefik-config must not add capabilities to bypass host ownership")
	}
}
