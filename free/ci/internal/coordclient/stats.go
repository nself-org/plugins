package coordclient

// Purpose: the client side of GET /v1/stats (P7-SCHED-20): `nself ci runs stats --coordinator <url>`.
// Inputs:  a window and an optional project.
// Outputs: the coordinator's contract:ci.stats document, byte for byte as served.
// Constraints: same transport, pinned root and credential rules as every other call; the answer is read to
//          at most 8 MiB; the project travels in the query, escaped.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
)

// maxStatsAnswer caps the stats document.
const maxStatsAnswer = 8 << 20

// Stats fetches the stats document for the caller's projects; since 0 asks for the server default.
func (c *Client) Stats(ctx context.Context, since time.Duration, project string) (json.RawMessage, error) {
	q := url.Values{}
	if since > 0 {
		q.Set("since", since.String())
	}
	if project != "" {
		q.Set("project", project)
	}
	path := clientapi.PathStats
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxStatsAnswer+1))
	if err != nil || len(raw) > maxStatsAnswer || !json.Valid(raw) {
		return nil, clientapi.Failf(clientapi.ReasonTransport, "the coordinator answer is not a valid stats document")
	}
	return json.RawMessage(raw), nil
}
