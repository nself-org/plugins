package coordclient

// Purpose: the client side of TEAM node administration (P7-NODE-26, Epic NODE D14): list, show and the
// seven mutations of `nself ci nodes <sub> --coordinator <url>`, sent to the /v1/nodes routes.
// Inputs:  a verb, a node id and a clientapi.NodeRequest.
// Outputs: the command's clientapi.NodeResult (exit code, stdout, stderr) exactly as the coordinator ran it.
// Constraints: the node id travels as one escaped path segment; a 200 or a 422 answer carries a
//          NodeResult (422 = the command ran and failed); any other non-2xx answer is the E664 error its
//          body carries; same transport, pinned root and credential rules as every other call; no body is
//          read past 8 MiB.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
)

// maxNodeAnswer caps a node command's answer.
const maxNodeAnswer = 8 << 20

// NodeList lists the nodes the coordinator knows.
func (c *Client) NodeList(ctx context.Context, asJSON bool) (clientapi.NodeResult, error) {
	return c.node(ctx, http.MethodGet, clientapi.PathNodes+jsonQuery(asJSON), nil)
}

// NodeShow shows one node.
func (c *Client) NodeShow(ctx context.Context, id string, asJSON bool) (clientapi.NodeResult, error) {
	return c.node(ctx, http.MethodGet, clientapi.PathNodesPfx+url.PathEscape(id)+jsonQuery(asJSON), nil)
}

// NodeDo runs one mutation: verb is set, drain, enable, revoke, remove or rotate on node id, or upgrade
// (id is ignored: the request names the node or all).
func (c *Client) NodeDo(ctx context.Context, verb, id string, req clientapi.NodeRequest) (clientapi.NodeResult, error) {
	path := clientapi.PathNodesPfx + url.PathEscape(id) + "/" + verb
	if verb == "upgrade" {
		path = clientapi.PathNodesUpgrade
	}
	body, _ := json.Marshal(req)
	return c.node(ctx, http.MethodPost, path, body)
}

func jsonQuery(asJSON bool) string {
	if asJSON {
		return "?json=1"
	}
	return ""
}

// nodeHTTP is the client's HTTP client with the response-header timeout lifted: a node command answers when
// it ends, and an upgrade waits for running jobs for minutes. The caller's context carries the deadline.
func (c *Client) nodeHTTP() *http.Client {
	t, ok := c.hc.Transport.(*http.Transport)
	if !ok {
		return c.hc
	}
	t2 := t.Clone()
	t2.ResponseHeaderTimeout = 0
	hc := *c.hc
	hc.Transport = t2
	return &hc
}

// node sends one request and decodes the NodeResult of a 200 or 422 answer.
func (c *Client) node(ctx context.Context, method, path string, body []byte) (clientapi.NodeResult, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return clientapi.NodeResult{}, clientapi.Failf(clientapi.ReasonBadRequest, "the request cannot be built")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	hc := c.nodeHTTP()
	if t, ok := hc.Transport.(*http.Transport); ok && t != c.hc.Transport {
		defer t.CloseIdleConnections()
	}
	resp, err := hc.Do(req)
	if err != nil {
		return clientapi.NodeResult{}, netError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnprocessableEntity {
		var b clientapi.Body
		if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&b) != nil || b.Code != clientapi.CodeUserAPI {
			return clientapi.NodeResult{}, clientapi.Failf(clientapi.ReasonTransport, "the coordinator answered %d", resp.StatusCode)
		}
		return clientapi.NodeResult{}, &clientapi.Error{Reason: b.Reason, Msg: b.Message}
	}
	var out clientapi.NodeResult
	if json.NewDecoder(io.LimitReader(resp.Body, maxNodeAnswer)).Decode(&out) != nil {
		return clientapi.NodeResult{}, clientapi.Failf(clientapi.ReasonTransport, "the coordinator answer is not valid")
	}
	return out, nil
}
