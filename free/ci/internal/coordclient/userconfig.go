package coordclient

// Purpose: the user config of the coordinator client (P7-NODE-19, Epic D14): the `ci.coordinator*` keys of
// $NSELF_CI_HOME/user.yaml (or the file NSELF_CI_USER_CONFIG names). It belongs to the person at this
// machine, never to a repository.
// Inputs:  the file.
// Outputs: coordinator URL and credential file paths.
// Constraints: a missing file is empty; the file must be a regular file nobody else can write (mode & 022
//          == 0) and unknown keys are ignored; a token is never stored in this file, only the path of a
//          token file (checked when read: regular file, mode & 077 == 0).

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
)

type userConfig struct {
	Coordinator, CA, Cert, Key, TokenFile string
}

func loadUserConfig(getenv func(string) string, home string) (userConfig, error) {
	path := getenv(EnvUserConfig)
	if path == "" {
		path = filepath.Join(home, "user.yaml")
	}
	fi, err := os.Stat(path)
	if err != nil {
		return userConfig{}, nil // absent: nothing configured
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0o022 != 0 {
		return userConfig{}, clientapi.Failf(clientapi.ReasonCredentialFile, "%s must be a regular file that only you can write", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return userConfig{}, clientapi.Failf(clientapi.ReasonCredentialFile, "cannot read %s", path)
	}
	var doc struct {
		CI struct {
			Coordinator string `yaml:"coordinator"`
			CA          string `yaml:"coordinator_ca"`
			Cert        string `yaml:"coordinator_cert"`
			Key         string `yaml:"coordinator_key"`
			TokenFile   string `yaml:"coordinator_token_file"`
		} `yaml:"ci"`
	}
	if err = yaml.Unmarshal(raw, &doc); err != nil {
		return userConfig{}, clientapi.Failf(clientapi.ReasonBadRequest, "%s is not valid YAML", path)
	}
	c := doc.CI
	return userConfig{Coordinator: c.Coordinator, CA: c.CA, Cert: c.Cert, Key: c.Key, TokenFile: c.TokenFile}, nil
}
