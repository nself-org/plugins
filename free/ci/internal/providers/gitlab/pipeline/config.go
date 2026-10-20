package pipeline

// Purpose: prove the pipeline ran the generated job and nothing else put in its way (Epic D11):
//          hostedev compares the include file's bytes, but GitLab merges every CI file of a project, so a
//          local `nself-ci:` job, a global before_script, a variable or a default cache could change what
//          the generated job does while the include file stays byte-identical. See checkConfig for the
//          two bindings (untrusted ref: byte-exact root; default branch: merged configuration).
// Inputs:  GET /projects/:id/ci/lint?content_ref=<pipeline sha> (the merged configuration and the
//          includes it was built from, at the pipeline's own commit) and the include file bytes.
// Outputs: nil, or E705: workflow.edited when the merged nself-ci job differs from the include file's
//          job or a global setting that every job inherits is present; config.unverified when the
//          merged configuration cannot be read or does not parse; a transient failure stays transient.
// Constraints: this runs after hostedev.Verify, so a wrong include file keeps its own reason; the
//          comparison is structural (parsed YAML, not text); only inheritance that changes what a job
//          executes is refused (before_script, after_script, hooks, services, cache at the top level
//          and under default:), the project's other jobs and variables are its own business; every
//          include except the generated local file is refused (D-1347); nothing here reads a token or
//          sends a body.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/hostedev"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

// ReasonConfig is the E705 reason for a merged configuration that cannot be read or parsed.
const ReasonConfig = "config.unverified"

// inherited are the keywords whose top-level or default: value every job runs with.
var inherited = []string{"before_script", "after_script", "hooks", "services", "cache"}

type lintAnswer struct {
	Valid      bool          `json:"valid"`
	MergedYAML string        `json:"merged_yaml"`
	Includes   *[]lintSource `json:"includes"`
}

// lintSource is one file the lint resolved into the merged configuration.
type lintSource struct {
	Type     string `json:"type"`
	Location string `json:"location"`
}

// onlyGeneratedInclude refuses every include except the generated local file (and the main file, should
// the lint list it): a remote, project, template or component include, a second local file, or a
// generated file that is not among the includes at all can change the merged job after creation or hide
// behind rules the static lint cannot evaluate (D-1347). An answer without the includes key (an older
// GitLab) cannot be checked and is unverified, not accepted.
func onlyGeneratedInclude(includes *[]lintSource) error {
	if includes == nil {
		return api.NotEvidence(ReasonConfig)
	}
	found := false
	for _, in := range *includes {
		loc := strings.TrimPrefix(in.Location, "/")
		if in.Type != "local" || (loc != gitlabtmpl.Path && loc != gitlabtmpl.RootPath) {
			return api.NotEvidence(hostedev.ReasonEdited)
		}
		found = found || loc == gitlabtmpl.Path
	}
	if !found {
		return api.NotEvidence(hostedev.ReasonEdited)
	}
	return nil
}

func (p *Provider) checkMerged(ctx context.Context, c *rest.Client, project api.Project, sha string, include []byte) error {
	var a lintAnswer
	if _, err := c.DoQuery(ctx, "GET", api.LintPath, project.Params(), url.Values{"content_ref": {sha}}, nil, &a); err != nil {
		if api.IsTransient(err) {
			return err
		}
		return api.NotEvidence(ReasonConfig)
	}
	if !a.Valid || a.MergedYAML == "" {
		return api.NotEvidence(ReasonConfig)
	}
	if err := onlyGeneratedInclude(a.Includes); err != nil {
		return err
	}
	return compareMerged(a.MergedYAML, include)
}

// jobOf returns the nself-ci job of a YAML stream (the include file has a spec header document and a
// body document) and the top-level mapping it was found in.
func jobOf(stream []byte) (map[string]any, map[string]any, error) {
	dec := yaml.NewDecoder(bytes.NewReader(stream))
	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				return nil, nil, nil
			}
			return nil, nil, err
		}
		if job, ok := doc[gitlabtmpl.Job].(map[string]any); ok {
			return job, doc, nil
		}
	}
}

func compareMerged(merged string, include []byte) error {
	want, _, err := jobOf(include)
	if err != nil || want == nil {
		return api.NotEvidence(hostedev.ReasonTemplate)
	}
	got, top, err := jobOf([]byte(merged))
	if err != nil {
		return api.NotEvidence(ReasonConfig)
	}
	if got == nil || !reflect.DeepEqual(got, want) {
		return api.NotEvidence(hostedev.ReasonEdited)
	}
	for _, k := range inherited {
		if _, present := top[k]; present {
			return api.NotEvidence(hostedev.ReasonEdited)
		}
		if def, ok := top["default"].(map[string]any); ok {
			if _, present := def[k]; present {
				return api.NotEvidence(hostedev.ReasonEdited)
			}
		}
	}
	return nil
}

// checkConfig applies the configuration binding that fits the dispatch. Untrusted work ran on the
// generated untrusted ref, which anyone with push rights can write: the main .gitlab-ci.yml at the
// pipeline's own sha must be byte for byte the generated root of the journalled transport (the include
// file was already compared by hostedev), so no other mapping, include, variable or job can exist.
// Trusted work ran on the project's default branch, whose CI file its maintainers own and who can run
// anything on this coordinator's runners anyway: there the merged configuration is checked (checkMerged,
// best effort, not a security boundary: lint is static, so a remote include can still change after the
// run; every include except the generated local file is refused, see onlyGeneratedInclude).
func (p *Provider) checkConfig(ctx context.Context, c *rest.Client, project api.Project, sha string, include []byte, rec Record) error {
	if rec.Trusted {
		return p.checkMerged(ctx, c, project, sha, include)
	}
	root, err := p.fetch(ctx, c, api.RawFileURL(c.BaseURL, project, gitlabtmpl.RootPath, sha), 256<<10)
	if err != nil {
		if api.IsTransient(err) {
			return err
		}
		return api.NotEvidence(hostedev.ReasonEdited)
	}
	want := gitlabtmpl.RootVariables
	if rec.Mode == ModeInputs {
		want = gitlabtmpl.RootInputs
	}
	if gitlabtmpl.RootKindOf(root) != want {
		return api.NotEvidence(hostedev.ReasonEdited)
	}
	return nil
}
