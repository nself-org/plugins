package notify

// Purpose: the webhook sink: an HTTPS POST of the event JSON.
// Inputs: SinkSpec.URL (https, no credentials, query or fragment), an optional Secret, and
// Deps.AllowPrivate (the operator key notify.allow_private).
// Outputs: a 2xx answer, or E813 naming the sink and a safe reason ("ssrf.private", a status).
// Constraints: every request goes through the HOST-18 REST funnel (providers/rest): its
// connect-time guard refuses a private, loopback, link-local or mapped address unless
// AllowPrivateBase is set, redirects never follow a POST, and nothing is logged with the URL.
// The HMAC-SHA256 signature is sent as `X-Nself-Signature: <hex hmac of the exact body>`.
// A receiver verifies it with the shared secret. The body is capped at MaxBody. The URL may
// carry a token in its path (that is why notify.sinks is a secret key): the funnel logs the
// path template, so the path is passed as parameters, never inlined.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// quiet keeps the funnel's per-request log line off the operator's run output.
var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type webhookSink struct {
	name, secret string
	base         string // scheme://host[:port]
	template     string // "/{p0}/{p1}" for the URL path
	params       map[string]string
	allowPrivate bool
	deps         Deps
}

func newWebhook(spec SinkSpec, d Deps) (Notifier, error) {
	name := nameOf(spec)
	u, err := url.Parse(spec.URL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || len(spec.Argv) != 0 {
		return nil, coded("sink " + name + ": a webhook needs an https url without credentials, query or fragment (and no argv)")
	}
	w := &webhookSink{name: name, secret: spec.Secret, base: "https://" + u.Host, params: map[string]string{}, allowPrivate: d.AllowPrivate, deps: d}
	var tmpl strings.Builder
	for i, seg := range strings.Split(u.EscapedPath(), "/")[min(1, len(u.EscapedPath())):] {
		tmpl.WriteByte('/')
		if seg == "" {
			continue
		}
		plain, uerr := url.PathUnescape(seg)
		if uerr != nil {
			return nil, coded("sink " + name + ": the url path is not valid")
		}
		key := fmt.Sprintf("p%d", i)
		w.params[key] = plain
		tmpl.WriteString("{" + key + "}")
	}
	if w.template = tmpl.String(); w.template == "" {
		w.template = "/"
	}
	return w, nil
}

func (w *webhookSink) Name() string { return w.name }

func (w *webhookSink) Notify(ctx context.Context, ev Event) error {
	raw, err := json.Marshal(ev)
	if err != nil {
		return coded("sink " + w.name + ": event not encodable")
	}
	// The funnel marshals a RawMessage again; that output is the exact body that is signed.
	body, err := json.Marshal(json.RawMessage(raw))
	if err != nil || len(body) > MaxBody {
		return coded(fmt.Sprintf("sink %s: event body over %d bytes", w.name, MaxBody))
	}
	c := &rest.Client{BaseURL: w.base, Provider: "notify", UserAgent: "notify", Logger: quiet, AllowPrivateBase: w.allowPrivate, TLSConfig: w.deps.TLSConfig}
	if w.secret != "" {
		c.Headers = func(body []byte) map[string]string {
			sum := hmac.New(sha256.New, []byte(w.secret))
			sum.Write(body)
			return map[string]string{"X-Nself-Signature": hex.EncodeToString(sum.Sum(nil))}
		}
	}
	if _, err = c.Do(ctx, "POST", w.template, w.params, json.RawMessage(body), nil); err != nil {
		var pe *providers.Error
		if errors.As(err, &pe) {
			return coded(fmt.Sprintf("sink %s: webhook %s (status %d)", w.name, pe.Reason, pe.Status))
		}
		return coded("sink " + w.name + ": webhook failed")
	}
	return nil
}
