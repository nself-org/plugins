package evidence

// Purpose: the input digests a piece of evidence is bound to: the effective
// config, every lockfile and every declared generated input, read from the git
// objects of one revision (never the working tree, so Windows line-ending
// filters and uncommitted edits cannot make the producer and the verifier
// disagree).
// Inputs:  a repository root and a full revision. Outputs: model.InputDigests.
// Constraints: the digests are computed only by internal/attest (a second
// implementation is prohibited); an unreadable lockfile or config is an error,
// never an empty list; the catalogue-bound detectors do not matter here, only
// the lockfile detector runs.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func digestString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// InputsAt digests the inputs of rev.
func InputsAt(ctx context.Context, c Capture, root, rev string) (model.InputDigests, error) {
	tree, err := TreeFS(ctx, c, root, rev)
	if err != nil {
		return model.InputDigests{}, err
	}
	return InputsOf(tree)
}

// InputsOf digests the inputs found in a tree.
func InputsOf(tree fs.FS) (model.InputDigests, error) {
	cfg, err := effectiveConfig(tree)
	if err != nil {
		return model.InputDigests{}, fail(CodeRejected, 11, "config is unreadable at that revision: %s", firstLine(err.Error()))
	}
	facts, err := detect.DefaultRegistry().Detect(tree, func(id string) bool { return id == "inputs.lockfiles" })
	if err != nil {
		return model.InputDigests{}, fail(CodeRejected, 11, "inputs cannot be listed at that revision: %s", firstLine(err.Error()))
	}
	var locks, generated []attest.File
	for _, f := range facts {
		if f.Kind != "lockfile" && f.Kind != "generated" {
			continue
		}
		if f.Confidence != detect.Certain {
			return model.InputDigests{}, fail(CodeRejected, 11, "%s %s cannot be digested: %s", f.Kind, f.Path, f.Reason)
		}
		body, err := fs.ReadFile(tree, f.Path)
		if err != nil {
			return model.InputDigests{}, fail(CodeRejected, 11, "%s %s is unreadable at that revision", f.Kind, f.Path)
		}
		if f.Kind == "lockfile" {
			locks = append(locks, attest.File{Path: f.Path, Content: body})
		} else {
			generated = append(generated, attest.File{Path: f.Path, Content: body})
		}
	}
	sort.Slice(locks, func(i, j int) bool { return locks[i].Path < locks[j].Path })
	d, err := attest.InputDigests(cfg, locks, generated)
	if err != nil {
		return model.InputDigests{}, fail(CodeRejected, 11, "%s", firstLine(err.Error()))
	}
	return d, nil
}

// effectiveConfig is the config of the tree: .nself/ci.yaml, or the built-in
// defaults when the tree has none.
func effectiveConfig(tree fs.FS) (model.PipelineConfig, error) {
	raw, err := fs.ReadFile(tree, config.FileName)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return model.PipelineConfig{Version: 1}, nil
	case err != nil:
		return model.PipelineConfig{}, err
	}
	loaded, err := config.Parse(config.FileName, raw)
	if err != nil {
		return model.PipelineConfig{}, err
	}
	return loaded.Config, nil
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}
