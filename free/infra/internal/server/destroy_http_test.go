package server

// Purpose: end-to-end destroy tests over the CONCRETE hetznerClient against a
// scripted httptest server (P7-CANON-12 review): ListPrimaryIPs wire behaviour
// and the full request order GET server, POST create_image, GET image, GET
// primary IPs, PUT auto_delete=false, DELETE. A failure at any step must leave
// the DELETE unsent. No test reaches api.hetzner.cloud (withTestServer).

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// scripted records "METHOD path" per request and answers from a route table;
// a route not in the table answers 404 so an unexpected call is visible.
type scripted struct {
	mu     sync.Mutex
	seen   []string
	bodies []string
	routes map[string]string // "METHOD /path" -> JSON body ("" = 204); prefix "ERR:" = 500
}

func (s *scripted) handler(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	key := r.Method + " " + r.URL.Path
	s.mu.Lock()
	s.seen = append(s.seen, key)
	s.bodies = append(s.bodies, string(b))
	resp, ok := s.routes[key]
	s.mu.Unlock()
	switch {
	case !ok:
		http.Error(w, `{"error":{"code":"not_found","message":"unscripted"}}`, 404)
	case strings.HasPrefix(resp, "ERR:"):
		http.Error(w, `{"error":{"code":"boom","message":"`+resp[4:]+`"}}`, 500)
	case resp == "":
		w.WriteHeader(204)
	default:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp))
	}
}

const (
	srvJSON  = `{"server":{"id":7,"name":"web-1","status":"running","server_type":{"name":"cx22","disk":40},"public_net":{"ipv4":{"id":10,"ip":"203.0.113.10"},"ipv6":{"id":11,"ip":"2001:db8::1"}}}}`
	snapJSON = `{"image":{"id":500,"type":"snapshot","status":"creating"},"action":{"id":1,"status":"running"}}`
	imgOK    = `{"image":{"id":500,"status":"available"}}`
	ip10     = `{"primary_ip":{"id":10,"ip":"203.0.113.10","type":"ipv4","assignee_id":7,"auto_delete":true}}`
	ip11     = `{"primary_ip":{"id":11,"ip":"2001:db8::1","type":"ipv6","assignee_id":7,"auto_delete":true}}`
)

func happyRoutes() map[string]string {
	return map[string]string{
		"GET /servers/7":                       srvJSON,
		"POST /servers/7/actions/create_image": snapJSON,
		"GET /images/500":                      imgOK,
		"GET /primary_ips/10":                  ip10,
		"GET /primary_ips/11":                  ip11,
		"PUT /primary_ips/10":                  `{}`,
		"PUT /primary_ips/11":                  `{}`,
		"DELETE /servers/7":                    "",
	}
}

func runScripted(t *testing.T, routes map[string]string, req DestroyRequest) (*scripted, error) {
	t.Helper()
	s := &scripted{routes: routes}
	withTestServer(t, s.handler)
	if req.SnapshotWait == 0 {
		req.SnapshotWait = 2 * time.Second
	}
	_, err := Destroy(context.Background(), NewHetznerClient("tok"), req)
	return s, err
}

func indexOf(seen []string, key string) int {
	for i, k := range seen {
		if k == key {
			return i
		}
	}
	return -1
}

func TestListPrimaryIPs_Http_BothFamilies(t *testing.T) {
	s := &scripted{routes: happyRoutes()}
	withTestServer(t, s.handler)
	ips, err := NewHetznerClient("tok").ListPrimaryIPs(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListPrimaryIPs: %v", err)
	}
	if len(ips) != 2 || ips[0].ID != 10 || ips[1].ID != 11 || !ips[0].AutoDelete || ips[0].AssigneeID != 7 {
		t.Fatalf("ips = %+v", ips)
	}
	want := "GET /servers/7;GET /primary_ips/10;GET /primary_ips/11"
	if got := strings.Join(s.seen, ";"); got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
}

func TestListPrimaryIPs_Http_SkipsZeroIDsAndPropagatesErrors(t *testing.T) {
	s := &scripted{routes: map[string]string{
		"GET /servers/7":      `{"server":{"id":7,"public_net":{"ipv4":{"id":10,"ip":"1.2.3.4"},"ipv6":{"id":0,"ip":""}}}}`,
		"GET /primary_ips/10": ip10,
	}}
	withTestServer(t, s.handler)
	ips, err := NewHetznerClient("tok").ListPrimaryIPs(context.Background(), 7)
	if err != nil || len(ips) != 1 {
		t.Fatalf("ips=%v err=%v", ips, err)
	}
	s2 := &scripted{routes: map[string]string{"GET /servers/7": srvJSON, "GET /primary_ips/10": "ERR:down"}}
	withTestServer(t, s2.handler)
	if _, err := NewHetznerClient("tok").ListPrimaryIPs(context.Background(), 7); err == nil || !strings.Contains(err.Error(), "get primary IP 10") {
		t.Fatalf("want a wrapped get error, got %v", err)
	}
}

func TestDestroy_Http_SnapshotThenIPThenDelete_Order(t *testing.T) {
	s, err := runScripted(t, happyRoutes(), DestroyRequest{ServerID: 7, TakeSnapshot: true})
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	snap := indexOf(s.seen, "POST /servers/7/actions/create_image")
	img := indexOf(s.seen, "GET /images/500")
	list := indexOf(s.seen, "GET /primary_ips/10")
	put := indexOf(s.seen, "PUT /primary_ips/10")
	del := indexOf(s.seen, "DELETE /servers/7")
	if !(0 <= snap && snap < img && img < list && list < put && put < del) {
		t.Fatalf("order wrong: %v", s.seen)
	}
	if s.seen[len(s.seen)-1] != "DELETE /servers/7" {
		t.Fatalf("DELETE must be the last request: %v", s.seen)
	}
	for i, k := range s.seen {
		if strings.HasPrefix(k, "PUT /primary_ips/") && !strings.Contains(s.bodies[i], `"auto_delete":false`) {
			t.Errorf("%s body %q must set auto_delete false", k, s.bodies[i])
		}
	}
}

func TestDestroy_Http_ReleaseIPWritesAutoDeleteTrue(t *testing.T) {
	s, err := runScripted(t, happyRoutes(), DestroyRequest{ServerID: 7, ForceNoBackup: true, ReleaseIP: true})
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	puts := 0
	for i, k := range s.seen {
		if strings.HasPrefix(k, "PUT /primary_ips/") {
			puts++
			if !strings.Contains(s.bodies[i], `"auto_delete":true`) {
				t.Errorf("%s body %q must set auto_delete true", k, s.bodies[i])
			}
		}
	}
	if puts != 2 || s.seen[len(s.seen)-1] != "DELETE /servers/7" {
		t.Fatalf("want 2 explicit IP writes then DELETE: %v", s.seen)
	}
}

func TestDestroy_Http_NoDeleteOnAnyFailure(t *testing.T) {
	cases := []struct {
		name string
		mut  func(map[string]string)
		req  DestroyRequest
	}{
		{"gate", nil, DestroyRequest{ServerID: 7}},
		{"get server fails", func(r map[string]string) { r["GET /servers/7"] = "ERR:x" }, DestroyRequest{ServerID: 7, TakeSnapshot: true}},
		{"snapshot start fails", func(r map[string]string) { r["POST /servers/7/actions/create_image"] = "ERR:x" }, DestroyRequest{ServerID: 7, TakeSnapshot: true}},
		{"snapshot action error", func(r map[string]string) {
			r["POST /servers/7/actions/create_image"] = `{"image":{"id":500,"status":"creating"},"action":{"id":1,"status":"error","error":{"code":"c","message":"m"}}}`
		}, DestroyRequest{ServerID: 7, TakeSnapshot: true}},
		{"image poll fails", func(r map[string]string) { r["GET /images/500"] = "ERR:x" }, DestroyRequest{ServerID: 7, TakeSnapshot: true}},
		{"snapshot never available", func(r map[string]string) {
			r["GET /images/500"] = `{"image":{"id":500,"status":"creating"}}`
			r["GET /actions/1"] = `{"action":{"id":1,"status":"running"}}`
		},
			DestroyRequest{ServerID: 7, TakeSnapshot: true, SnapshotWait: time.Millisecond}},
		{"ip list fails", func(r map[string]string) { r["GET /primary_ips/10"] = "ERR:x" }, DestroyRequest{ServerID: 7, ForceNoBackup: true}},
		{"ip protect fails", func(r map[string]string) { r["PUT /primary_ips/10"] = "ERR:x" }, DestroyRequest{ServerID: 7, ForceNoBackup: true}},
		{"ip protect fails after snapshot", func(r map[string]string) { r["PUT /primary_ips/11"] = "ERR:x" }, DestroyRequest{ServerID: 7, TakeSnapshot: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := snapshotPollInterval
			snapshotPollInterval = time.Millisecond
			defer func() { snapshotPollInterval = old }()
			routes := happyRoutes()
			if tc.mut != nil {
				tc.mut(routes)
			}
			s, err := runScripted(t, routes, tc.req)
			if err == nil {
				t.Fatalf("destroy must fail; requests: %v", s.seen)
			}
			for _, k := range s.seen {
				if strings.HasPrefix(k, "DELETE ") {
					t.Fatalf("DELETE sent despite failure %q: %v", err, s.seen)
				}
			}
		})
	}
	_ = fmt.Sprint
}
