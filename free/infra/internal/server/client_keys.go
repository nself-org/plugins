package server

// Purpose: SSH key operations and a non-creating token scope probe.
// Inputs: public key metadata or an authenticated client.
// Outputs: SSH key metadata and read-write/read-only scope strings.
// Constraints: the scope probe sends no key material and cleans up unexpected creation.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// SSHKey is public SSH key metadata stored by Hetzner.
type SSHKey struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	PublicKey   string            `json:"public_key"`
	Fingerprint string            `json:"fingerprint"`
	Labels      map[string]string `json:"labels"`
}

// CreateSSHKey registers a public SSH key with optional labels.
func (c *hetznerClient) CreateSSHKey(ctx context.Context, name, publicKey string, labels map[string]string) (*SSHKey, error) {
	body := map[string]interface{}{"name": name, "public_key": publicKey}
	if len(labels) != 0 {
		body["labels"] = labels
	}
	var out struct {
		SSHKey SSHKey `json:"ssh_key"`
	}
	if err := c.do(ctx, "POST", "/ssh_keys", body, &out); err != nil {
		return nil, fmt.Errorf("create SSH key: %w", err)
	}
	return &out.SSHKey, nil
}

// DeleteSSHKey deletes a registered SSH key by ID.
func (c *hetznerClient) DeleteSSHKey(ctx context.Context, id int64) error {
	return c.do(ctx, "DELETE", fmt.Sprintf("/ssh_keys/%d", id), nil, nil)
}

// ListSSHKeysByLabel returns every SSH key matching a Hetzner label selector.
func (c *hetznerClient) ListSSHKeysByLabel(ctx context.Context, selector string) ([]SSHKey, error) {
	query := url.Values{}
	if selector != "" {
		query.Set("label_selector", selector)
	}
	return listPages[SSHKey](ctx, c, "/ssh_keys", "ssh_keys", query)
}

// ProbeWriteScope posts an empty body so validation tests scope without creating a key.
// Unexpected creation is an error, and any returned key ID is deleted first.
func (c *hetznerClient) ProbeWriteScope(ctx context.Context) (string, error) {
	var out struct {
		SSHKey SSHKey `json:"ssh_key"`
	}
	status, err := c.doStatus(ctx, "POST", "/ssh_keys", nil, &out)
	if status == http.StatusCreated {
		if out.SSHKey.ID <= 0 {
			return "", fmt.Errorf("scope probe unexpectedly created an SSH key without a usable ID")
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if deleteErr := c.DeleteSSHKey(cleanup, out.SSHKey.ID); deleteErr != nil {
			return "", fmt.Errorf("scope probe unexpectedly created SSH key %d; cleanup failed: %w", out.SSHKey.ID, deleteErr)
		}
		return "", fmt.Errorf("scope probe unexpectedly created an SSH key; key deleted")
	}
	var ae *hetznerResponseError
	if errors.As(err, &ae) {
		switch {
		case (ae.envelope.Error.Code == "invalid_input" && (status == 400 || status == 422)) ||
			(ae.envelope.Error.Code == "json_error" && status == http.StatusBadRequest):
			// Hetzner authorizes before it parses: a write-scoped token reaches body
			// validation, which rejects the empty body as invalid_input or json_error.
			return "read-write", nil
		case ae.envelope.Error.Code == "forbidden" && status == http.StatusForbidden:
			return "read-only", nil
		}
	}
	if err != nil {
		return "", fmt.Errorf("probe write scope: %w", err)
	}
	return "", fmt.Errorf("scope probe returned unexpected HTTP %d", status)
}
