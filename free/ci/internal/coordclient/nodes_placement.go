package coordclient

// Purpose: the client of the TEAM node labels and projects routes (P7-NODE-30): `nself ci nodes label` and
// `nself ci nodes projects ... --coordinator <url>`.
// Inputs:  a node id and a clientapi.PlacementRequest.
// Outputs: the coordinator's NodeResult (exit code, stdout, stderr), or an E664 error.
// Constraints: the node id travels as one path segment; mutations are POST, reads are GET with ?json=1.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
)

// NodeLabels reads a node's labels (write false) or edits them (write true; the request carries Labels and Remove).
func (c *Client) NodeLabels(ctx context.Context, id string, write bool, req clientapi.PlacementRequest) (clientapi.NodeResult, error) {
	return c.placement(ctx, id, "labels", write, req)
}

// NodeProjects lists a node's authorized projects (write false) or changes them (write true; req.Action is add or remove).
func (c *Client) NodeProjects(ctx context.Context, id string, write bool, req clientapi.PlacementRequest) (clientapi.NodeResult, error) {
	return c.placement(ctx, id, "projects", write, req)
}

func (c *Client) placement(ctx context.Context, id, leaf string, write bool, req clientapi.PlacementRequest) (clientapi.NodeResult, error) {
	path := clientapi.PathNodesPfx + url.PathEscape(id) + "/" + leaf
	if !write {
		return c.node(ctx, http.MethodGet, path+jsonQuery(req.JSON), nil)
	}
	body, _ := json.Marshal(req)
	return c.node(ctx, http.MethodPost, path, body)
}
