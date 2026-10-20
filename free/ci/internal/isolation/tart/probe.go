package tart

// Purpose: find out what Tart can deliver on this runner, as facts.
// Inputs:  `tart --version`, `tart list --format json`, `tart run --help`, the operator's base image setting.
// Outputs: an isolation.Availability with Tart version, licence note, free VM slots and the softnet block
//          option as facts; a probe only ever lowers the level.
// Constraints: no Tart, no Apple-silicon macOS, no base image digest, or no free slot means unavailable
//          (E678); a Tart without the softnet block option stays available (lan/privileged jobs still run)
//          but every scope below lan is refused with E679 by Wrap.

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
)

const probeTimeout = 30 * time.Second

// vmInfo is one row of `tart list --format json`.
type vmInfo struct {
	Source  string `json:"Source"`
	Name    string `json:"Name"`
	State   string `json:"State"`
	Running bool   `json:"Running"`
}

func (v vmInfo) running() bool { return v.Running || strings.EqualFold(v.State, "running") }

func listVMs(ctx context.Context, tool Tool, bin string) ([]vmInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	res, err := tool.Run(ctx, []string{bin, "list", "--format", "json"}, nil, nil)
	if err != nil {
		return nil, err
	}
	if res.Exit != 0 {
		return nil, isolation.Unavailable(isolation.ReasonProbeFailed, "tart list failed (exit %d)", res.Exit)
	}
	var vms []vmInfo
	if err := json.Unmarshal(res.Out, &vms); err != nil {
		return nil, isolation.Unavailable(isolation.ReasonProbeFailed, "tart list printed no JSON: %v", err)
	}
	return vms, nil
}

// Probe reports what the runner can deliver now.
func (p *Profile) Probe(ctx context.Context) isolation.Availability { return p.probe(ctx, false) }

func (p *Profile) probe(ctx context.Context, fresh bool) isolation.Availability {
	p.mu.Lock()
	if !fresh && !p.at.IsZero() && p.opts.Now().Sub(p.at) < p.opts.TTL {
		av := copyAvailability(p.av)
		p.mu.Unlock()
		return av
	}
	p.mu.Unlock()
	av := p.probeNow(ctx)
	p.mu.Lock()
	p.av, p.at = av, p.opts.Now()
	p.mu.Unlock()
	return copyAvailability(av)
}

func copyAvailability(a isolation.Availability) isolation.Availability {
	f := make(map[string]string, len(a.Facts))
	for k, v := range a.Facts {
		f[k] = v
	}
	a.Facts = f
	return a
}

func unavailable(reason, hint string, facts map[string]string) isolation.Availability {
	facts["hint"] = hint
	return isolation.Availability{Reason: reason, Facts: facts}
}

func (p *Profile) probeNow(ctx context.Context) isolation.Availability {
	facts := map[string]string{"licence_note": LicenceNote, "slots_total": strconv.Itoa(MaxSlots)}
	if p.opts.GOOS != "darwin" || p.opts.GOARCH != "arm64" {
		return unavailable(isolation.ReasonRuntimeMissing, "Tart needs an Apple-silicon Mac (darwin/arm64)", facts)
	}
	tool := p.opts.NewTool("tart-probe", p.home(""))
	pctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	ver, err := tool.Run(pctx, []string{p.opts.Tart, "--version"}, nil, nil)
	if err != nil || ver.Exit != 0 {
		return unavailable(isolation.ReasonRuntimeMissing, "install Tart yourself (brew install cirruslabs/cli/tart); nSelf never bundles it", facts)
	}
	facts["tart_version"] = strings.TrimSpace(firstLine(ver.Out))
	image := p.str(p.opts.BaseImage, "")
	if !baseImageRef.MatchString(image) {
		return unavailable(ReasonBaseImage, "set ci.tart.base_image to an image reference with @sha256:<digest> in ~/.nself/ci/config.yaml", facts)
	}
	facts["base_image"] = image
	vms, err := listVMs(pctx, tool, p.opts.Tart)
	if err != nil {
		return unavailable(isolation.ReasonProbeFailed, err.Error(), facts)
	}
	foreign, own := vmCounts(vms)
	if p.slots.Free(foreign, own) == 0 {
		// two orphans of a dead coordinator must not block the runner for good: sweep, then look again
		p.sweepOnce(pctx, "tart-probe")
		if again, err := listVMs(pctx, tool, p.opts.Tart); err == nil {
			vms = again
			foreign, own = vmCounts(vms)
		}
	}
	for _, v := range vms {
		if v.Name == image || strings.TrimPrefix(v.Name, "local/") == image {
			facts["base_image_cached"] = "true"
		}
	}
	if facts["base_image_cached"] == "" {
		facts["base_image_cached"] = "false"
	}
	free := p.slots.Free(foreign, own)
	facts["running_vms"] = strconv.Itoa(foreign + own)
	facts["foreign_running"], facts["own_running"] = strconv.Itoa(foreign), strconv.Itoa(own)
	facts["free_slots"] = strconv.Itoa(free)
	help, err := tool.Run(pctx, []string{p.opts.Tart, "run", "--help"}, nil, nil)
	block := err == nil && softnetBlockSupported(help.Out)
	facts["softnet_block"] = strconv.FormatBool(block)
	if free == 0 {
		return unavailable(ReasonSlots, "both macOS VM slots are in use; the job waits", facts)
	}
	return isolation.Availability{Level: p.level, Facts: facts}
}

// softnetBlockSupported reports whether `tart run --help` offers softnet with allow and block lists.
func softnetBlockSupported(help []byte) bool {
	s := string(help)
	return strings.Contains(s, "--net-softnet-block") && strings.Contains(s, "--net-softnet-allow")
}

func firstLine(b []byte) string {
	s, _, _ := strings.Cut(string(b), "\n")
	return s
}
