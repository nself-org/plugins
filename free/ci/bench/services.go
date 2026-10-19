package bench

// Purpose: the service-start scenarios: how long a disposable service
// dependency (Epic D9) takes to come up through the nself CLI, warm (image
// already pulled) and cold (image removed first, so the pull is in the time).
// Inputs:  an nself CLI binary and a Docker daemon, both on this host.
// Outputs: ci.service_start_warm and ci.service_start_cold samples.
// Constraints: opt-in (-services) because it needs Docker and a v1.5 nself CLI;
// the cold scenario removes only the images the throwaway project itself used,
// and only when docker agrees nothing else uses them (no force), so a
// developer machine loses at most a cached image it can pull again.

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/services"
)

// ServiceConfig selects the service scenarios.
type ServiceConfig struct {
	Nself     string // the nself CLI binary
	Component string // the component to start, default postgres
	Cold      bool   // also measure the cold start
	ColdRuns  int    // cold samples, default the run count
}

// measureServices adds the service samples to col.
func measureServices(ctx context.Context, cfg Config, home string, col *Collector) error {
	sc := cfg.Services
	comp := sc.Component
	if comp == "" {
		comp = "postgres"
	}
	prov := &services.Provisioner{Run: &services.Funnel{Home: home}, Home: home, Nself: sc.Nself}
	specs := []services.Spec{{Name: comp, Component: comp}}
	up := func() (float64, string, error) {
		var h *services.Handle
		ms, err := timeIt(func() (e error) { h, e = prov.Up(ctx, specs); return })
		if err != nil {
			return 0, "", fmt.Errorf("service %s: %w", comp, err)
		}
		images, err := imagesOf(ctx, h.Project)
		if err != nil {
			_ = prov.Down(ctx, h)
			return 0, "", err
		}
		if err := prov.Down(ctx, h); err != nil {
			return 0, "", fmt.Errorf("service %s teardown: %w", comp, err)
		}
		return ms, images, nil
	}
	_, images, err := up() // pulls the image; not a sample
	if err != nil {
		return err
	}
	for i := 0; i < cfg.Runs; i++ {
		ms, _, err := up()
		if err != nil {
			return err
		}
		col.Add(MetricServiceWarm, "ms", ms)
	}
	if !sc.Cold {
		return nil
	}
	n := sc.ColdRuns
	if n < 1 {
		n = cfg.Runs
	}
	for i := 0; i < n; i++ {
		for _, img := range strings.Fields(images) {
			if _, err := must(ctx, []string{"docker", "image", "rm", img}, cfg.Dir, baseEnv()); err != nil {
				return fmt.Errorf("cold start needs the image removed: %w", err)
			}
		}
		ms, _, err := up()
		if err != nil {
			return err
		}
		col.Add(MetricServiceCold, "ms", ms)
	}
	return nil
}

// imagesOf lists the images of the containers of compose project name. A
// project with no container is an error: the cold scenario would otherwise
// remove nothing and record a warm start as a cold one.
func imagesOf(ctx context.Context, project string) (string, error) {
	r, err := must(ctx, []string{"docker", "ps", "-a", "--filter", "label=com.docker.compose.project=" + project,
		"--format", "{{.Image}}"}, "", baseEnv())
	if err != nil {
		return "", fmt.Errorf("list the images of project %s: %w", project, err)
	}
	seen := map[string]bool{}
	var out []string
	for _, img := range strings.Fields(r.Tail) {
		if !seen[img] {
			seen[img] = true
			out = append(out, img)
		}
	}
	if len(out) == 0 {
		return "", fmt.Errorf("project %s has no container, so its image is unknown", project)
	}
	return strings.Join(out, " "), nil
}
