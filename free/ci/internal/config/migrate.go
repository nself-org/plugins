package config

// Purpose: translate legacy `.ci.yaml` plugin manifests (internal/legacy/
// pipeline.go: plugin, stages{name, cmd, timeout, env_required,
// skip_if_env_missing}) into jobs of the authored `.nself/ci.yaml`.
// Inputs:  a repo root; every `.ci.yaml` found by Discover.
// Outputs: a Migration holding the PipelineConfig, its YAML text and notes; or
// an *Error (E622) naming the file:line and field that cannot become a job.
// Constraints: lossless for every stage field (cmd -> run, timeout seconds ->
// "<n>s", skip_if_env_missing + env_required -> required: false +
// when.env_present, Epic D28); the E7 plugin order is not reproduced and no
// deps are invented, since a legacy stage never declared that it consumed
// another's output; this reader exists only for migration and the legacy
// engine keeps its own parser; the planned file is validated by Parse before
// it is returned.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type legacyStage struct {
	Name             string   `yaml:"name"`
	Cmd              []string `yaml:"cmd"`
	Timeout          int      `yaml:"timeout"`
	EnvRequired      []string `yaml:"env_required"`
	SkipIfEnvMissing bool     `yaml:"skip_if_env_missing"`
}

type legacyFile struct {
	Plugin        string        `yaml:"plugin"`
	Epic          string        `yaml:"epic"`
	Stack         string        `yaml:"stack"`
	Stages        []legacyStage `yaml:"stages"`
	GatewayRoutes []string      `yaml:"gateway_routes"`
}

// Migration is the result of translating every legacy manifest of a repo.
type Migration struct {
	Config  model.PipelineConfig
	Order   []string // job ids in stage order
	Sources []string // legacy files read, repo-relative
	Notes   []string // facts the translation dropped or changed
	YAML    []byte
	notes   map[string][]string
}

func untranslatable(file string, line int, field, why string) error {
	return &Error{Code: CodeUntranslatable, Problems: []Problem{{File: file, Line: line, Pointer: field, Message: why}}}
}

var legacyField = regexp.MustCompile(`line (\d+): field (\S+) not found`)

func parseLegacy(rel string, data []byte) (legacyFile, error) {
	var lf legacyFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&lf); err != nil && !errors.Is(err, io.EOF) {
		if m := legacyField.FindStringSubmatch(err.Error()); m != nil {
			line := 0
			fmt.Sscanf(m[1], "%d", &line)
			return lf, untranslatable(rel, line, m[2], "legacy field "+m[2]+" has no equivalent in .nself/ci.yaml")
		}
		return lf, untranslatable(rel, 0, "", "not a readable legacy manifest: "+strings.TrimPrefix(err.Error(), "yaml: "))
	}
	return lf, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// jobID builds "<plugin short name>-<stage>": plugin-gauth + rust:clippy
// becomes gauth-clippy; the full stage name is used when that collides.
func jobID(plugin, stage string, taken map[string]bool) string {
	prefix := slug(strings.TrimPrefix(plugin, "plugin-"))
	short := stage
	if i := strings.LastIndex(stage, ":"); i >= 0 {
		short = stage[i+1:]
	}
	for _, name := range []string{short, stage} {
		if id := prefix + "-" + slug(name); !taken[id] && slug(name) != "" {
			return id
		}
	}
	return ""
}

// kindFor classifies a stage by its name; anything unrecognised is a test.
func kindFor(name string) model.JobKind {
	n := strings.ToLower(name)
	for _, word := range []string{"clippy", "lint", "vet", "fmt", "static"} {
		if strings.Contains(n, word) {
			return "static"
		}
	}
	if strings.Contains(n, "build") {
		return "build"
	}
	return "test"
}

// Migrate translates every legacy manifest under root.
func Migrate(root string) (*Migration, error) {
	found, err := Discover(root)
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, ErrNothingToMigrate
	}
	m := &Migration{Sources: found, notes: map[string][]string{}}
	m.Config = model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{}}
	taken := map[string]bool{}
	for _, rel := range found {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		lf, err := parseLegacy(rel, data)
		if err != nil {
			return nil, err
		}
		if lf.Plugin == "" {
			m.Notes = append(m.Notes, rel+": no plugin name; the legacy runner skipped this file, so it is skipped here")
			continue
		}
		if lf.Epic != "" || lf.Stack != "" {
			m.Notes = append(m.Notes, rel+": epic and stack are planning metadata and are dropped")
		}
		if len(lf.GatewayRoutes) > 0 {
			return nil, untranslatable(rel, 0, "gateway_routes", "gateway route probes are not jobs; remove the field or move the probes into a job run argv")
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		for i, st := range lf.Stages {
			field := fmt.Sprintf("stages[%d]", i)
			id := jobID(lf.Plugin, st.Name, taken)
			switch {
			case id == "":
				return nil, untranslatable(rel, 0, field+".name", "stage name "+fmt.Sprintf("%q", st.Name)+" does not give a unique job id")
			case len(st.Cmd) == 0:
				return nil, untranslatable(rel, 0, field+".cmd", "stage "+st.Name+" declares no cmd; a job needs at least one argv element")
			case st.Timeout < 0:
				return nil, untranslatable(rel, 0, field+".timeout", "negative timeout")
			}
			taken[id] = true
			job := model.Job{Kind: kindFor(st.Name), Run: st.Cmd}
			if st.Timeout > 0 {
				job.Timeout = fmt.Sprintf("%ds", st.Timeout)
			}
			if dir != "." {
				job.Workdir = dir
				job.When = &model.WhenConfig{Paths: []string{dir + "/**"}}
			}
			if st.SkipIfEnvMissing && len(st.EnvRequired) > 0 {
				if job.When == nil {
					job.When = &model.WhenConfig{}
				}
				job.When.EnvPresent = st.EnvRequired
				off := false
				job.Required = &off
			} else if len(st.EnvRequired) > 0 {
				m.notes[id] = append(m.notes[id], "legacy env_required ("+strings.Join(st.EnvRequired, ", ")+") was not enforced because skip_if_env_missing was false")
			}
			m.notes[id] = append(m.notes[id], "from "+rel+" stage "+st.Name)
			m.Config.Jobs[id] = job
			m.Order = append(m.Order, id)
		}
	}
	if len(m.Order) == 0 {
		return nil, untranslatable(strings.Join(found, ", "), 0, "stages", "no stage to migrate")
	}
	if err := m.render(); err != nil {
		return nil, err
	}
	return m, nil
}
