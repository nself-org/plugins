package coordclient

// Purpose: `nself ci --coordinator`: submit a revision to a coordinator and follow its event stream to the
// end (P7-NODE-19, Epic D14).
// Inputs:  project (owner/repo), a full revision, an optional pipeline, the output mode and writers.
// Outputs: with JSON, the contract:ci.events lines exactly as the coordinator wrote them (the last one is
//          the result record holding the signed evidence); otherwise one line per job result and a
//          verdict line. The exit status is 0 for a run that passed and 1 otherwise.
// Constraints: stdout carries only run output (events or the human lines); progress notes and errors go
//          to Err; the result comes from the `finished` record, never from a guess; an interrupted
//          context stops following (the run keeps going on the coordinator) and is reported as such.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/detach"
)

// RunInput is one submit-and-follow.
type RunInput struct {
	Project, Revision, Pipeline string
	JSON                        bool
	Out, Err                    io.Writer
}

// RunResult is how the followed run ended.
type RunResult struct {
	RunID  string
	Result string // the run's final state ("passed", "failed", ...); "" when the stream ended without one
	Exit   int
}

// Run submits and follows.
func (c *Client) Run(ctx context.Context, in RunInput) (RunResult, error) {
	sub, err := c.Submit(ctx, clientapi.SubmitRequest{Project: in.Project, Revision: in.Revision, Pipeline: in.Pipeline})
	if err != nil {
		return RunResult{}, err
	}
	res := RunResult{RunID: sub.ID}
	fmt.Fprintf(in.Err, "submitted %s (%s) as run %s\n", short(in.Revision), sub.Outcome, sub.ID)
	_, err = c.Events(ctx, sub.ID, func(line []byte, rec detach.Record) error {
		if rec.Type == "finished" {
			var e envelope
			if json.Unmarshal(line, &e) == nil {
				res.Result = e.Data.Data.Result
			}
		}
		if in.JSON {
			_, e := in.Out.Write(append(append([]byte(nil), line...), '\n'))
			return e
		}
		if r := humanLine(line, rec); r != "" {
			fmt.Fprintln(in.Out, r)
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	if it, ok, e := c.Get(ctx, sub.ID); e == nil && ok {
		res.RunID = it.ID // the run id the other commands print and accept; the stream names the submit id
	}
	if res.Result != "passed" {
		res.Exit = 1
	}
	if !in.JSON {
		fmt.Fprintf(in.Out, "run %s: %s\n", res.RunID, firstNonEmpty(res.Result, "ended without a result"))
	}
	return res, nil
}

func short(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}

// envelope reads the fields of a ci.events line the human output and the exit status need.
type envelope struct {
	Data struct {
		Type  string `json:"type"`
		JobID string `json:"job_id"`
		Data  struct {
			State        string `json:"state"`
			Check        string `json:"check"`
			FailureClass string `json:"failure_class"`
			Result       string `json:"result"`
		} `json:"data"`
	} `json:"data"`
}

func humanLine(line []byte, rec detach.Record) string {
	var e envelope
	if json.Unmarshal(line, &e) != nil {
		return ""
	}
	d := e.Data.Data
	switch rec.Type {
	case "check_result":
		s := fmt.Sprintf("  %-10s %s", d.State, firstNonEmpty(d.Check, e.Data.JobID))
		if d.FailureClass != "" {
			s += " (" + d.FailureClass + ")"
		}
		return s
	}
	return ""
}
