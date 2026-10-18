package toolchain

// Purpose: the `toolchain-pins` section of `nself ci doctor` (P7-CI-50
// registry): the embedded table matches the pins, the toolchain image row
// state, and the tool versions installed on this machine against the pinned
// ones.
// Inputs:  doctor.Env (repository root, process runner).
// Outputs: CI-DI01 table and pins agree, CI-DI02 image row, CI-DI03 one check
// per tool whose installed version differs from the pin (a summary pass
// otherwise). Ids with prefix CI-DI belong to this package.
// Constraints: a missing tool is not reported here (CI-D001 owns presence);
// version differences are warnings, never failures; probes honour ctx.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
)

// SectionName is the registered section.
const SectionName = "toolchain-pins"

// probeBudget bounds one version probe so a slow tool cannot eat the section's budget.
const probeBudget = 2 * time.Second

func init() {
	doctor.RegisterSection(doctor.Section{Name: SectionName, Selector: doctor.SelectAlways, Run: runSection})
}

var versionRe = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?`)

func runSection(ctx context.Context, env doctor.Env) []doctor.Check {
	table, err := Embedded()
	if err != nil {
		return []doctor.Check{{ID: "CI-DI01", Name: "toolchain pins", Status: doctor.Fail, Message: err.Error(),
			Remediation: "Rebuild nself-ci from a checkout where `go generate ./internal/toolchain/...` is clean"}}
	}
	out := []doctor.Check{checkDrift(env.Root, table), checkImageRow(table)}
	return append(out, checkInstalled(ctx, env, table)...)
}

// pinsDir finds the pins of this plugin in the checkout doctor runs in; any other
// repository (even one with its own toolchain/pins) is not ours to judge.
func pinsDir(root string) string {
	d := filepath.Join(root, "free", "ci", "toolchain", "pins")
	if st, err := os.Stat(d); err == nil && st.IsDir() {
		return d
	}
	return ""
}

func checkDrift(root string, table Versions) doctor.Check {
	c := doctor.Check{ID: "CI-DI01", Name: "toolchain pins", Status: doctor.Pass}
	dir := pinsDir(root)
	if dir == "" {
		c.Message = fmt.Sprintf("embedded table has %d rows; this repository has no toolchain pins to compare", len(table.Rows))
		return c
	}
	pins, err := LoadPins(os.DirFS(dir), ".")
	if err == nil {
		err = compareRendered(pins, dir)
	}
	if err != nil {
		c.Status, c.Message = doctor.Fail, err.Error()
		c.Remediation = "Run `go generate ./internal/toolchain/...` in free/ci, commit the regenerated files and rebuild nself-ci"
		return c
	}
	c.Message = fmt.Sprintf("toolchain/versions.yaml matches its %d pin sources", len(pins))
	return c
}

// compareRendered fails when the pins render something other than the table
// compiled into this binary, or other than toolchain/versions.yaml on disk.
func compareRendered(pins []Pin, dir string) error {
	want, err := Render(pins)
	if err != nil {
		return err
	}
	if !bytes.Equal(embedded, want) {
		return fail("the table embedded in this nself-ci differs from the pins in this checkout")
	}
	have, err := os.ReadFile(filepath.Join(filepath.Dir(dir), "versions.yaml"))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return fail("cannot read toolchain/versions.yaml: %v", err)
	case !bytes.Equal(have, want):
		return fail("toolchain/versions.yaml differs from what the pins render")
	}
	return nil
}

func checkImageRow(table Versions) doctor.Check {
	c := doctor.Check{ID: "CI-DI02", Name: "toolchain image", Status: doctor.Pass}
	row, ok := table.Row(ImageRow)
	switch {
	case !ok:
		c.Status, c.Message = doctor.Fail, "versions.yaml has no "+ImageRow+" row"
		c.Remediation = "Regenerate versions.yaml from the pins"
	case row.Status == StatusPinned:
		c.Message = "pinned image " + row.Image + "@" + row.Digest
	default:
		c.Message = "image " + row.Image + " is pending its first publish; --container needs --image <ref@digest> until then"
	}
	return c
}

func checkInstalled(ctx context.Context, env doctor.Env, table Versions) []doctor.Check {
	var out []doctor.Check
	probed := 0
	for _, p := range table.Rows {
		if p.Kind != KindTool || len(p.Probe) == 0 || env.Capture == nil || ctx.Err() != nil {
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, probeBudget)
		text, rc, err := env.Capture(pctx, p.Probe...)
		cancel()
		if err != nil || rc != 0 {
			continue
		}
		have := installedVersion(p.Name, text)
		if have == "" {
			continue
		}
		probed++
		if have != p.Version {
			out = append(out, doctor.Check{ID: "CI-DI03", Name: "toolchain version", Status: doctor.Warn, Subject: p.Name,
				Message:     fmt.Sprintf("%s %s is installed, the toolchain image pins %s", p.Name, have, p.Version),
				Remediation: "Run with --container for the pinned version, or install " + p.Name + " " + p.Version})
		}
	}
	if len(out) == 0 {
		out = append(out, doctor.Check{ID: "CI-DI03", Name: "toolchain version", Status: doctor.Pass,
			Message: fmt.Sprintf("%d installed tools match their pins (tools not installed are not listed)", probed)})
	}
	return out
}

// installedVersion reads the version a probe printed: "<name>@v1.2.3" or
// "<name> 1.2.3" when the output names the tool, otherwise the first number.
func installedVersion(name, out string) string {
	named := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(name) + `[@ ]v?(\d+\.\d+(?:\.\d+)?)`)
	if m := named.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return versionRe.FindString(strings.TrimSpace(out))
}
