// Purpose: serialise the values and secrets documents and write them under
// .nself/generated/k8s/.
//
// Inputs: Values, Secrets, an output directory.
//
// Outputs: values.yaml (0644) and secrets.yaml (0600) in the output directory.
//
// Constraints: keys are emitted sorted (yaml.v3 sorts maps; slices are sorted
// by the mapper), so the same model gives the same bytes. secrets.yaml is
// created 0600 and chmod-ed 0600 (umask cannot widen it, an old looser file
// is tightened). Secret values are never written anywhere else.
package values

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	// ValuesFile and SecretsFile are the output file names.
	ValuesFile  = "values.yaml"
	SecretsFile = "secrets.yaml"
)

// MarshalValues renders values.yaml.
func MarshalValues(v *Values) ([]byte, error) { return marshal(v, "") }

// MarshalSecrets renders secrets.yaml.
func MarshalSecrets(s *Secrets) ([]byte, error) {
	return marshal(s, "# Holds every env value of the compose model. Mode 0600. Never commit, never log.\n")
}

func marshal(doc interface{}, note string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(GeneratedMarker + "\n" + note)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encoding yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Write writes both documents into dir.
func Write(dir string, v *Values, s *Secrets) error {
	vb, err := MarshalValues(v)
	if err != nil {
		return err
	}
	sb, err := MarshalSecrets(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, ValuesFile), vb, 0o644); err != nil {
		return err
	}
	return writePrivate(filepath.Join(dir, SecretsFile), sb)
}

func writePrivate(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
