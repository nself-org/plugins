package render

import (
	"fmt"
	"regexp"
	"time"

	"github.com/nself-org/plugins/free/traefik/internal/contract"
)

// nginx defaults when a timeout is null: 60s connect and read; websocket
// locations read for 86400s (the template constant).
const (
	defTimeoutS = 60
	wsReadS     = 86400
)

// upstreamHostRE is a compose service name or DNS name (underscore allowed). It
// keeps ?, #, backticks and other URL syntax out of the service URL.
var upstreamHostRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// service registers the load balancer for one upstream and returns its name.
// The serversTransport carries the timeouts; https upstreams are not verified,
// which is nginx's default for proxy_pass https.
func (b *builder) service(route, field string, l contract.Location, passHost bool) string {
	u := l.Upstream
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" || u.Port <= 0 || !upstreamHostRE.MatchString(u.Host) {
		b.refuse(route, field+".upstream", fmt.Sprintf("%s://%s:%d is not a plain upstream", u.Scheme, u.Host, u.Port))
		return ""
	}
	if l.Timeouts.SendS != nil {
		b.refuse(route, field+".timeouts.send_s", "proxy_send_timeout has no Traefik equivalent")
		return ""
	}
	conn, read := defTimeoutS, defTimeoutS
	if l.WebSocket {
		read = wsReadS
	}
	if l.Timeouts.ConnectS != nil {
		conn = *l.Timeouts.ConnectS
	}
	if l.Timeouts.ReadS != nil {
		read = *l.Timeouts.ReadS
	}
	tr := map[string]any{"forwardingTimeouts": map[string]any{
		"dialTimeout":           (time.Duration(conn) * time.Second).String(),
		"responseHeaderTimeout": (time.Duration(read) * time.Second).String()}}
	if u.Scheme == "https" {
		tr["insecureSkipVerify"] = true
	}
	tname := "transport-" + h8(tr)
	b.put(b.transports, "serversTransport", tname, tr, false)
	name := fmt.Sprintf("svc-%s-%s-%d-%s", u.Scheme, safe(u.Host), u.Port, h8([]any{tname, passHost, u.Host}))
	b.put(b.services, "service", name, map[string]any{"loadBalancer": map[string]any{
		"servers":          []any{map[string]any{"url": fmt.Sprintf("%s://%s:%d", u.Scheme, u.Host, u.Port)}},
		"passHostHeader":   passHost,
		"serversTransport": tname}}, false)
	return name
}
