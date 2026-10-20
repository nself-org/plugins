package pipeline

// Purpose: choose the transport of a dispatch (Epic D11): pipeline inputs or variables.
// Inputs:  GET /version, the main .gitlab-ci.yml raw at the dispatch ref, the input table of
//          templates/gitlab.
// Outputs: ModeInputs when the instance takes pipeline inputs (17.10 and later) AND the main file at
//          the ref declares every input of the table; ModeVariables otherwise.
// Constraints: GitLab rejects inputs the main file does not declare and an included file cannot receive
//          pipeline inputs, so the instance version alone proves nothing; a main file that cannot be
//          read or parsed means variables, which every version accepts; a transient failure is returned
//          so the caller retries instead of silently changing transport; the raw file is read through
//          the same download path as the include file.

import (
	"bytes"
	"context"
	"errors"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

const mainFile = ".gitlab-ci.yml"

// declaresInputs reports whether a main CI file's spec header declares every input of the table.
func declaresInputs(main []byte) bool {
	var doc struct {
		Spec struct {
			Inputs map[string]any `yaml:"inputs"`
		} `yaml:"spec"`
	}
	if yaml.NewDecoder(bytes.NewReader(main)).Decode(&doc) != nil {
		return false
	}
	for _, in := range gitlabtmpl.Inputs() {
		if _, ok := doc.Spec.Inputs[in.Name]; !ok {
			return false
		}
	}
	return true
}

// mainDeclares reads the main file at ref and applies declaresInputs.
func (p *Provider) mainDeclares(ctx context.Context, c *rest.Client, project api.Project, ref string) (bool, error) {
	raw, err := p.fetch(ctx, c, api.RawFileURL(c.BaseURL, project, mainFile, ref), 256<<10)
	if err != nil {
		if api.IsTransient(err) || api.IsCredentialMissing(err) {
			return false, err
		}
		return false, nil
	}
	return declaresInputs(raw), nil
}

// refMissing reports whether the branch does not exist (a 404 from the branch route); any other
// answer, including an error, is "do not know" and counts as present.
func (p *Provider) refMissing(ctx context.Context, c *rest.Client, project api.Project, ref string) bool {
	_, err := c.Do(ctx, "GET", api.BranchPath, project.With("branch", ref), nil, nil)
	return err != nil && api.IsStatus(err, 404) && !errors.Is(err, context.Canceled)
}
