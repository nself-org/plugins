package coordclient

// Purpose: the operations of the coordinator client (P7-NODE-19, Epic D14): submit a revision, list and
// show runs, cancel, read stored logs and read the signed evidence.
// Inputs:  contexts, run references, the wire types of internal/coordinator/clientapi.
// Outputs: those wire types; evidence as the exact bytes the coordinator signed.
// Constraints: a run reference travels as one escaped path segment; evidence is returned unparsed, so the
//          client can never alter a signature; a 404 on show is (zero, false, nil), every other failure is
//          an E664 error.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/logs"
)

// Submit asks the coordinator to run a revision of project.
func (c *Client) Submit(ctx context.Context, r clientapi.SubmitRequest) (clientapi.SubmitResult, error) {
	body, _ := json.Marshal(r)
	resp, err := c.do(ctx, http.MethodPost, clientapi.PathRuns, bytes.NewReader(body))
	if err != nil {
		return clientapi.SubmitResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out clientapi.SubmitResult
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out) != nil || out.ID == "" {
		return out, clientapi.Failf(clientapi.ReasonTransport, "the coordinator answer to the submit is not valid")
	}
	return out, nil
}

// List returns the newest runs the caller may see.
func (c *Client) List(ctx context.Context, limit int) ([]clientapi.RunItem, error) {
	var out struct {
		Runs []clientapi.RunItem `json:"runs"`
	}
	err := c.getJSON(ctx, clientapi.PathRuns+"?limit="+strconv.Itoa(limit), &out)
	return out.Runs, err
}

// Get shows one run; ok is false when the coordinator has no such run for this caller.
func (c *Client) Get(ctx context.Context, ref string) (clientapi.RunItem, bool, error) {
	var it clientapi.RunItem
	err := c.getJSON(ctx, clientapi.PathRunsPfx+url.PathEscape(refOrLatest(ref)), &it)
	if clientapi.ReasonOf(err) == clientapi.ReasonNotFound {
		return it, false, nil
	}
	return it, err == nil, err
}

func refOrLatest(ref string) string {
	if ref == "" {
		return "latest"
	}
	return ref
}

// Cancel stops the live attempts of a run and returns how many it asked.
func (c *Client) Cancel(ctx context.Context, ref string) (int, error) {
	resp, err := c.do(ctx, http.MethodPost, clientapi.PathRunsPfx+url.PathEscape(refOrLatest(ref))+"/cancel", nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Cancelled int `json:"cancelled"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&out) != nil {
		return 0, clientapi.Failf(clientapi.ReasonTransport, "the coordinator answer to the cancel is not valid")
	}
	return out.Cancelled, nil
}

// Evidence returns the run's signed evidence document byte for byte.
func (c *Client) Evidence(ctx context.Context, ref string) (json.RawMessage, error) {
	resp, err := c.do(ctx, http.MethodGet, clientapi.PathEvidence+url.PathEscape(refOrLatest(ref)), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, clientapi.Failf(clientapi.ReasonTransport, "the evidence could not be read")
	}
	return raw, nil
}

// Logs streams the stored log lines of a job; ok is false when the coordinator has none.
func (c *Client) Logs(ctx context.Context, q clientapi.LogQuery, fn func(logs.Line) error) (bool, error) {
	v := url.Values{}
	if q.Run != "" {
		v.Set("run", q.Run)
	}
	v.Set("attempt", strconv.Itoa(q.Attempt))
	v.Set("after", strconv.FormatInt(q.After, 10))
	if q.Follow {
		v.Set("follow", "1")
	}
	resp, err := c.do(ctx, http.MethodGet, clientapi.PathLogsPfx+url.PathEscape(q.Job)+"?"+v.Encode(), nil)
	if err != nil {
		if clientapi.ReasonOf(err) == clientapi.ReasonNotFound {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		var l logs.Line
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			return true, clientapi.Failf(clientapi.ReasonTransport, "a log line is not valid")
		}
		if err = fn(l); err != nil {
			return true, err
		}
	}
	if err = sc.Err(); err != nil && ctx.Err() == nil {
		return true, clientapi.Failf(clientapi.ReasonTransport, "the log stream broke")
	}
	return true, nil
}
