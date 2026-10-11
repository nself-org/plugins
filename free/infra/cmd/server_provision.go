package main

// Purpose: `nself infra server provision` handler — parses flags, builds a
// server.ProvisionRequest, and reports the created server back.
// Inputs: --name, --type, --location, --image, --ssh-key (repeatable),
// --label (repeatable key=value), --user-data-file, --token, --token-env, --json.
// Outputs: printed confirmation (or the v1 envelope) with the new server's ID and IP
// (JSON also carries user_data true|false, never the content); a non-nil error on
// any validation, resolution, or provider failure.
// Constraints: the --user-data-file content may hold a single-use pairing code, so
// it is never printed, logged or returned, and no message carries its bytes or size.

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"unicode/utf8"

	"github.com/nself-org/nself-infra/internal/server"
	"github.com/nself-org/nself-infra/internal/ui"

	"github.com/spf13/cobra"
)

var serverProvisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Create a new Hetzner Cloud server",
	Long: `Create a new Hetzner Cloud server.

Every server nself provisions is labeled managed-by=nself-cli (unless you
pass your own --label managed-by=..., which is respected as-is), so
'nself infra server list' and any future cleanup pass can tell nself-created
servers apart from anything else in the same Hetzner project.`,
	Example: `  nself infra server provision --name ci-runner-3 --type cx22 --location fsn1 --image ubuntu-24.04 --ssh-key deploy
  nself infra server provision --name ci-runner-3 --type cx22 --location fsn1 --image ubuntu-24.04 --label purpose=ci --json`,
	RunE: runServerProvision,
}

func init() {
	f := serverProvisionCmd.Flags()
	f.String("name", "", "server name (required)")
	f.String("type", "", "server type, e.g. cx22 (required)")
	f.String("location", "", "datacenter location, e.g. fsn1 (required)")
	f.String("image", "", "OS image, e.g. ubuntu-24.04 (required)")
	f.StringArray("ssh-key", nil, "Hetzner SSH key name to authorize (repeatable)")
	f.StringArray("label", nil, "label as key=value (repeatable)")
	f.String("user-data-file", "", "path to a cloud-init user-data file sent as user_data; must be a regular file, mode 0600 or stricter, owned by you, at most 32768 bytes; the content is never printed")
	addJSONFlag(serverProvisionCmd)
}

func runServerProvision(cmd *cobra.Command, args []string) error {
	r := begin(cmd)
	defer r.end()
	name, _ := cmd.Flags().GetString("name")
	serverType, _ := cmd.Flags().GetString("type")
	location, _ := cmd.Flags().GetString("location")
	image, _ := cmd.Flags().GetString("image")
	sshKeys, _ := cmd.Flags().GetStringArray("ssh-key")
	labelArgs, _ := cmd.Flags().GetStringArray("label")

	labels, err := parseLabels(labelArgs)
	if err != nil {
		return r.fail(err)
	}

	userData, withUserData, err := userDataFromFlag(cmd)
	if err != nil {
		return r.fail(err)
	}

	client, err := newServerClient(cmd)
	if err != nil {
		return r.fail(err)
	}

	if !r.json {
		ui.CommandHeader("nself infra server provision", fmt.Sprintf("%s (%s, %s)", name, serverType, location))
	}

	srv, err := server.Provision(cmd.Context(), client, server.ProvisionRequest{
		Name: name, ServerType: serverType, Location: location, Image: image,
		SSHKeys: sshKeys, Labels: labels, UserData: userData,
	})
	if err != nil {
		return r.fail(fmt.Errorf("provision %s: %w", name, err))
	}

	if r.json {
		return r.done(map[string]any{"server": srv, "user_data": withUserData})
	}

	ui.Success(fmt.Sprintf("Created server %q (id %d)", srv.Name, srv.ID))
	ui.Info("IPv4: " + srv.IPv4)
	ui.Info("Status: " + srv.Status)
	return nil
}

// parseLabels turns repeated "key=value" flag values into a map, rejecting
// anything that isn't exactly one "=".
func parseLabels(args []string) (map[string]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	labels := make(map[string]string, len(args))
	for _, a := range args {
		k, v, ok := strings.Cut(a, "=")
		if !ok || k == "" {
			return nil, &usageError{msg: fmt.Sprintf("--label must be key=value, got %q", a), safe: "--label must be key=value"}
		}
		labels[k] = v
	}
	return labels, nil
}

// maxUserDataBytes is Hetzner's documented limit for the user_data field (32 KiB).
const maxUserDataBytes = 32768

// userDataSafe is the fixed envelope text for every --user-data-file refusal.
const userDataSafe = "--user-data-file must be a regular file, mode 0600 or stricter, owned by you, at most 32768 bytes"

// userDataFromFlag returns the user-data content when --user-data-file was set.
// An explicitly empty path is a usage error. The bool says the flag was used.
func userDataFromFlag(cmd *cobra.Command) (string, bool, error) {
	if !cmd.Flags().Changed("user-data-file") {
		return "", false, nil
	}
	path, _ := cmd.Flags().GetString("user-data-file")
	if path == "" {
		return "", false, &usageError{msg: "--user-data-file needs a path", safe: userDataSafe}
	}
	data, err := readUserDataFile(path)
	if err != nil {
		return "", false, err
	}
	return data, true, nil
}

// userDataRefusal builds the coded usage error for a refused user-data file.
// reason never carries file content, bytes or the raw size.
func userDataRefusal(path, reason string) error {
	return &usageError{msg: fmt.Sprintf("--user-data-file %q %s", path, reason), safe: userDataSafe}
}

// userDataOpenError maps an open failure to a usage error without wrapping the
// raw *PathError text.
func userDataOpenError(path string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return userDataRefusal(path, "does not exist")
	case errors.Is(err, fs.ErrPermission):
		return userDataRefusal(path, "cannot be read")
	}
	return userDataRefusal(path, "cannot be opened")
}

// readUserDataFile opens path without following symlinks, checks the open
// descriptor (never the path, so there is no check-then-open race) and returns
// the exact content. It refuses anything but a regular, non-empty, valid UTF-8
// file of mode 0600 or stricter, owned by the caller, within the size limit.
func readUserDataFile(path string) (string, error) {
	f, err := openUserDataFile(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return "", userDataRefusal(path, "cannot be opened")
	}
	if !fi.Mode().IsRegular() {
		return "", userDataRefusal(path, "is not a regular file")
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return "", userDataRefusal(path, "must be mode 0600 or stricter")
	}
	if checkUserDataOwner(fi) != nil {
		return "", userDataRefusal(path, "is not owned by you")
	}
	if fi.Size() > maxUserDataBytes {
		return "", userDataRefusal(path, "is larger than the 32768 byte limit")
	}
	b, err := io.ReadAll(io.LimitReader(f, maxUserDataBytes+1))
	if err != nil {
		return "", userDataRefusal(path, "cannot be read")
	}
	switch {
	case len(b) > maxUserDataBytes:
		return "", userDataRefusal(path, "is larger than the 32768 byte limit")
	case len(b) == 0:
		return "", userDataRefusal(path, "is empty")
	case !utf8.Valid(b):
		return "", userDataRefusal(path, "is not valid UTF-8")
	}
	return string(b), nil
}
