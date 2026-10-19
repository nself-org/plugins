package enroll

// Purpose: the HTTP handler of the enrollment route and the listener around it.
// Inputs:  a Redeemer (the pairing service), the listen address, a TLS config from pki.EnrollTLSConfig.
// Outputs: JSON replies; Server with Addr and Close.
// Constraints: exactly one route (anything else is 404), POST only, 16 KiB body cap, strict JSON, one
//          request per second per source address; no request body, header or code is ever logged (the
//          handler has no logger); Close stops accepting at once and lets an in-flight redemption finish
//          for up to 5 s, so the node that redeemed the last code still gets its certificate.

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"
)

// Redeemer is what the handler needs from the pairing service.
type Redeemer interface {
	Hello() Hello
	Redeem(ctx context.Context, req Request, src string) (Response, error)
}

// CodedError is an error with a fabric code and a reason key (pairing and pki errors).
type CodedError interface {
	error
	ErrCode() string
	ErrReason() string
}

// HandlerOptions configure Handler.
type HandlerOptions struct {
	Now func() time.Time
}

// Handler serves POST /v1/enroll.
func Handler(r Redeemer, o HandlerOptions) http.Handler {
	if o.Now == nil {
		o.Now = time.Now
	}
	lim := NewLimiter()
	mux := http.NewServeMux()
	mux.HandleFunc(Route, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeErr(w, http.StatusMethodNotAllowed, "E659", "bad_request", "POST only")
			return
		}
		host, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			host = req.RemoteAddr
		}
		if !lim.Allow(host, o.Now()) {
			w.Header().Set("Retry-After", "1")
			writeErr(w, http.StatusTooManyRequests, "E659", "rate_limited", "one request per second per source")
			return
		}
		var body Request
		dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, MaxBody))
		dec.DisallowUnknownFields()
		if err = dec.Decode(&body); err != nil || body.Version != Version {
			writeErr(w, http.StatusBadRequest, "E659", "bad_request", "request is not a valid enrollment request")
			return
		}
		switch body.Phase {
		case PhaseHello:
			writeJSON(w, http.StatusOK, r.Hello())
		case PhaseEnroll:
			resp, err := r.Redeem(req.Context(), body, host)
			if err != nil {
				code, reason, status := classify(err)
				msg := err.Error()
				if reason == "internal" { // a plain error may carry store or file details: never send them to a stranger
					msg = "the coordinator could not process the request"
				}
				writeErr(w, status, code, reason, msg)
				return
			}
			writeJSON(w, http.StatusOK, resp)
		default:
			writeErr(w, http.StatusBadRequest, "E659", "bad_request", "unknown phase")
		}
	})
	return mux
}

func classify(err error) (code, reason string, status int) {
	var ce CodedError
	if !errors.As(err, &ce) {
		return "E659", "internal", http.StatusInternalServerError
	}
	status = http.StatusForbidden
	if ce.ErrCode() == "E660" && ce.ErrReason() == "clock_skew" {
		status = http.StatusBadRequest
	}
	return ce.ErrCode(), ce.ErrReason(), status
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, reason, msg string) {
	writeJSON(w, status, ErrorBody{Code: code, Reason: reason, Message: msg})
}

// Server is a bound enrollment listener.
type Server struct {
	ln   net.Listener
	http *http.Server
	done chan struct{}
}

// Listen binds addr with TLS and serves h until Close.
func Listen(addr string, cfg *tls.Config, h http.Handler) (*Server, error) {
	ln, err := tls.Listen("tcp", addr, cfg)
	if err != nil {
		return nil, err
	}
	s := &Server{ln: ln, http: &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 11 * time.Minute, MaxHeaderBytes: 8 << 10}, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		_ = s.http.Serve(ln)
	}()
	return s, nil
}

// Addr is the bound address.
func (s *Server) Addr() net.Addr { return s.ln.Addr() }

// Close stops accepting now and finishes in-flight requests in the background.
func (s *Server) Close() {
	_ = s.ln.Close()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if s.http.Shutdown(ctx) != nil {
			_ = s.http.Close()
		}
	}()
}

// Done is closed when the server has stopped serving.
func (s *Server) Done() <-chan struct{} { return s.done }
