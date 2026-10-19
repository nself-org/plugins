package agent

// Purpose: the HTTP half of the pull client: one mTLS connection pool to the agent listener and the four
//          requests of the runner protocol (hello, lease long-poll, control stream, upstream stream).
// Inputs:  the listener URL and a tls.Config built by pki.TLSClientConfig (pinned root, coordinator-kind
//          server leaf only, client certificate re-read per handshake).
// Outputs: decoded protocol frames, response bodies, typed errors (*HTTPError) for refusals.
// Constraints: TLS 1.3 only; the token goes in a request header of the upstream stream and nowhere else; no
//          request body or header value is logged; the coordinator clock (Date header) is compared with the
//          agent clock on hello and a skew beyond pki.ClockSkew fails the hello with E660 clock_skew; flows
//          that continue across requests (lease, control) are decoded from the first seq the coordinator
//          sends, because a frame lost in transit leaves a gap the agent must not treat as corruption.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// HTTPError is a refusal answered with a JSON body {code, reason, message}.
type HTTPError struct {
	Status       int
	Code, Reason string
	Message      string
	LastSeq      string // X-Nself-Last-Seq of a resume refusal
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("agent listener answered %d %s %s: %s", e.Status, e.Code, e.Reason, e.Message)
}

// noSession reports a refusal that means "say hello again".
func (e *HTTPError) noSession() bool {
	return e.Status == http.StatusConflict && e.Reason == "no_session"
}

// Options configure a client.
type Options struct {
	// BaseURL is https://host:port of the agent listener.
	BaseURL string
	// TLS is the client TLS config (pki.TLSClientConfig); the client clones it.
	TLS *tls.Config
	// Wait is the lease long-poll wait (default 20 s).
	Wait time.Duration
	// Now is the clock (default time.Now).
	Now func() time.Time
	// OnSkew hears the coordinator clock offset after every hello.
	OnSkew func(skew time.Duration)
	// Transport, when set, replaces the default (tests).
	Transport http.RoundTripper
}

type client struct {
	o  Options
	hc *http.Client
}

func newClient(o Options) *client {
	if o.Wait <= 0 {
		o.Wait = 20 * time.Second
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	rt := o.Transport
	if rt == nil {
		rt = &http.Transport{TLSClientConfig: o.TLS.Clone(), ForceAttemptHTTP2: true, ResponseHeaderTimeout: o.Wait + 15*time.Second, TLSHandshakeTimeout: 15 * time.Second}
	}
	return &client{o: o, hc: &http.Client{Transport: rt}}
}

func (c *client) close() {
	if t, ok := c.hc.Transport.(interface{ CloseIdleConnections() }); ok {
		t.CloseIdleConnections()
	}
}

func (c *client) req(ctx context.Context, method, path, sess string, body io.Reader) (*http.Request, error) {
	rq, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.o.BaseURL, "/")+path, body)
	if err != nil {
		return nil, err
	}
	if sess != "" {
		rq.Header.Set(agentapi.HeaderSession, sess)
	}
	return rq, nil
}

// refusal turns a non-2xx response into an *HTTPError and closes the body.
func refusal(resp *http.Response) error {
	defer resp.Body.Close()
	e := &HTTPError{Status: resp.StatusCode, LastSeq: resp.Header.Get(agentapi.HeaderLastSeq)}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var body struct{ Code, Reason, Message string }
	if json.Unmarshal(b, &body) == nil {
		e.Code, e.Reason, e.Message = body.Code, body.Reason, body.Message
	}
	return e
}

// firstFrame reads the first frame of a response body and returns a decoder positioned after it, for flows
// whose seq continues across requests: the decoder starts from whatever seq the coordinator sent.
func firstFrame(body io.Reader, policy protocol.CarrierPolicy) (*protocol.Decoder, error) {
	br := bufio.NewReaderSize(body, 4096)
	line, err := readLine(br)
	if err != nil {
		return nil, err
	}
	var head struct {
		Seq uint64 `json:"seq"`
	}
	if err = json.Unmarshal(line, &head); err != nil || head.Seq == 0 {
		return nil, &protocol.Error{Code: protocol.CodeInvalid, Reason: "version_seq_or_body"}
	}
	return protocol.NewResumedDecoder(io.MultiReader(bytes.NewReader(line), br), policy, head.Seq-1, head.Seq-1)
}

func readLine(br *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		frag, err := br.ReadSlice('\n')
		line = append(line, frag...)
		if len(line) > protocol.MaxFrame {
			return nil, &protocol.Error{Code: protocol.CodeInvalid, Reason: "frame_too_large"}
		}
		if err == nil {
			return line, nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			if errors.Is(err, io.EOF) && len(line) == 0 {
				return nil, io.EOF
			}
			return nil, io.ErrUnexpectedEOF
		}
	}
}

// hello posts the hello frame and returns the welcome (or reject) message and the session id.
func (c *client) hello(ctx context.Context, frame []byte) (m protocol.Message, sess string, err error) {
	rq, err := c.req(ctx, http.MethodPost, "/v1/agent/hello", "", bytes.NewReader(frame))
	if err != nil {
		return m, "", err
	}
	resp, err := c.hc.Do(rq)
	if err != nil {
		return m, "", err
	}
	defer resp.Body.Close()
	if skew, ok := c.skew(resp); ok {
		if c.o.OnSkew != nil {
			c.o.OnSkew(skew)
		}
		if skew > pki.ClockSkew || skew < -pki.ClockSkew {
			return m, "", &pki.Error{Reason: pki.ReasonClockSkew, Msg: fmt.Sprintf("this clock differs from the coordinator's by %s (tolerance %s)", skew.Round(time.Second), pki.ClockSkew)}
		}
	}
	if ct := resp.Header.Get("Content-Type"); resp.StatusCode != http.StatusOK && !strings.HasPrefix(ct, "application/x-ndjson") {
		return m, "", refusal(resp)
	}
	m, err = protocol.NewDecoder(io.LimitReader(resp.Body, protocol.MaxFrame+1), protocol.CarrierPolicy{Carrier: "B"}).Decode()
	if err != nil {
		return m, "", err
	}
	return m, resp.Header.Get(agentapi.HeaderSession), nil
}

// skew is the coordinator clock minus this clock, from the Date header.
func (c *client) skew(resp *http.Response) (time.Duration, bool) {
	t, err := http.ParseTime(resp.Header.Get("Date"))
	if err != nil {
		return 0, false
	}
	return t.Sub(c.o.Now()), true
}

// lease long-polls once; a nil message is 204.
func (c *client) lease(ctx context.Context, sess string) (*protocol.Message, error) {
	rq, err := c.req(ctx, http.MethodGet, "/v1/agent/leases", sess, nil)
	if err != nil {
		return nil, err
	}
	rq.Header.Set(agentapi.HeaderWait, strconv.Itoa(int(c.o.Wait/time.Second)))
	resp, err := c.hc.Do(rq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNoContent {
		resp.Body.Close()
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, refusal(resp)
	}
	defer resp.Body.Close()
	m, err := (&flowReader{body: resp.Body}).next()
	return &m, err
}

// flowReader decodes a response body whose first seq is whatever the coordinator sends.
type flowReader struct {
	body io.Reader
	dec  *protocol.Decoder
}

func (f *flowReader) next() (protocol.Message, error) {
	if f.dec == nil {
		dec, err := firstFrame(f.body, protocol.CarrierPolicy{Carrier: "B"})
		if err != nil {
			return protocol.Message{}, err
		}
		f.dec = dec
	}
	return f.dec.Decode()
}

// control opens the coordinator-to-agent stream.
func (c *client) control(ctx context.Context, sess string) (*flowReader, io.Closer, error) {
	rq, err := c.req(ctx, http.MethodGet, "/v1/agent/control", sess, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.hc.Do(rq)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, refusal(resp)
	}
	return &flowReader{body: resp.Body}, resp.Body, nil
}

// stream opens the agent-to-coordinator stream. claim is the last transport seq the agent knows the
// coordinator accepted; tokens are the job tokens by lease id. A 200 response is returned open; its body
// carries the coordinator's answers for this stream, starting at seq 1.
func (c *client) stream(ctx context.Context, sess string, claim uint64, tokens map[string]string, body io.Reader) (*http.Response, error) {
	rq, err := c.req(ctx, http.MethodPost, "/v1/agent/stream", sess, body)
	if err != nil {
		return nil, err
	}
	rq.Header.Set(agentapi.HeaderLastSeq, strconv.FormatUint(claim, 10))
	rq.Header.Set("Content-Type", "application/x-ndjson")
	for lease, tok := range tokens {
		rq.Header.Add(agentapi.HeaderToken, lease+"="+tok)
	}
	resp, err := c.hc.Do(rq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, refusal(resp)
	}
	return resp, nil
}
