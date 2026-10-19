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
	if r, e := must(ctx, []string{"docker", "image", "ls", "--no-trunc", "--format", "{{.Repository}}:{{.Tag}} {{.ID}}"}, cfg.Dir, baseEnv()); e == nil {
		cfg.logf("images after the first start (project images: %s):\n%s", images, r.Tail)
	}
	coldWant := 0
	if sc.Cold {
		if coldWant = sc.ColdRuns; coldWant < 1 {
			coldWant = cfg.Runs
		}
	}
	// A sample is warm or cold by what docker says about the images right before
	// the start, not by what this loop intended: if the teardown removes the
	// images, every start is cold and no warm sample is recorded (and that is
	// reported below), instead of a cold start being filed as warm.
	warm, cold := 0, 0
	for try := 0; try < 2*(cfg.Runs+coldWant) && (warm < cfg.Runs || cold < coldWant); try++ {
		if coldWant > 0 && cold < coldWant && (warm >= cfg.Runs || try%2 == 1) {
			if err := removeImages(ctx, cfg.Dir, images); err != nil {
				return err
			}
		}
		present := imagesPresent(ctx, cfg.Dir, images)
		ms, _, err := up()
		if err != nil {
			return err
		}
		cfg.logf("service try %d: images present before start: %v, %.0f ms", try+1, present, ms)
		switch {
		case present && warm < cfg.Runs:
			col.Add(MetricServiceWarm, "ms", ms)
			warm++
		case !present && cold < coldWant:
			col.Add(MetricServiceCold, "ms", ms)
			cold++
		}
	}
	if warm < cfg.Runs || cold < coldWant {
		return fmt.Errorf("service scenarios recorded %d warm and %d cold samples, wanted %d and %d (images %s)",
			warm, cold, cfg.Runs, coldWant, images)
	}
	return nil
}

// removeImages removes the images; one that is already gone is the cold state.
func removeImages(ctx context.Context, dir, images string) error {
	for _, img := range strings.Fields(images) {
		r, err := spawn(ctx, []string{"docker", "image", "rm", img}, dir, baseEnv())
		if err != nil {
			return fmt.Errorf("cold start needs the image removed: %w", err)
		}
		if r.ExitCode != 0 && !strings.Contains(r.Tail, "No such image") {
			return fmt.Errorf("cold start needs the image removed: docker image rm %s: exit %d: %s", img, r.ExitCode, r.Tail)
		}
	}
	return nil
}

// imagesPresent reports whether every image is in the local store.
func imagesPresent(ctx context.Context, dir, images string) bool {
	for _, img := range strings.Fields(images) {
		r, err := spawn(ctx, []string{"docker", "image", "inspect", "--format", "{{.Id}}", img}, dir, baseEnv())
		if err != nil || r.ExitCode != 0 {
			return false
		}
	}
	return true
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
