package coordclient

// Purpose: GetRaw, the one generic authenticated GET of the coordinator client, for routes another Ticket
// registers through clientapi.RegisterRoute (P7-CACHE-17 reads release inputs and blobs through it).
// Inputs:  an absolute path starting with /v1/.
// Outputs: the response whatever its status; the caller reads the status and closes the body.
// Constraints: same transport, pinned root, credential and no-redirect rules as every other call; a
//          transport failure is the usual E664 reason key; a non-2xx answer is NOT turned into an error
//          here, because the route's own body (its code and reason) is the caller's to read.

import (
	"context"
	"net/http"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
)

// GetRaw sends an authenticated GET for path and returns the response unread.
func (c *Client) GetRaw(ctx context.Context, path string) (*http.Response, error) {
	if !strings.HasPrefix(path, "/v1/") || strings.ContainsAny(path, "\r\n \\") {
		return nil, clientapi.Failf(clientapi.ReasonBadRequest, "the request path is not a user API path")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, http.NoBody)
	if err != nil {
		return nil, clientapi.Failf(clientapi.ReasonBadRequest, "the request cannot be built")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, netError(err)
	}
	return resp, nil
}
