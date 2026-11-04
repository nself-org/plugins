package main

// Purpose: write the ci.evidence/v1 document `nself-ci-agent exec` assembles (split out of exec.go).
// Inputs:  the evidence path and the encoded document.
// Outputs: the file at path, written atomically with mode 0600.
// Constraints: the document must validate against the published ci-evidence.v1 schema before anything is
//          written; a failed write leaves no temporary file behind.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// writeEvidence validates the document against the published schema and
// writes it atomically with mode 0600.
func writeEvidence(path string, doc []byte) error {
	schemas, err := model.Schemas()
	if err != nil {
		return err
	}
	if err := model.ValidateJSON(schemas["ci-evidence.v1.schema.json"], doc); err != nil {
		return fmt.Errorf("evidence does not validate against ci.evidence/v1: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".evidence-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(doc); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
