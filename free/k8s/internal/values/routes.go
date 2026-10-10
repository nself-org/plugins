// Purpose: read the nginx route model (contract cli.proxy-routes v1,
// .nself/generated/routes.json) and turn the root location of each route into
// an Ingress rule. nginx itself is not deployed; Ingress replaces it.
//
// Inputs: project directory (LoadRoutes), mapped services (mapRoutes).
//
// Outputs: []Route, Values.Ingress.Rules, Values.UnmappedRoute.
//
// Constraints: only the fields below are read (the contract is additive). A
// route whose root upstream is not a mapped workload port is listed in
// UnmappedRoute with the reason, never dropped silently. A missing file means
// no routes, not an error.
package values

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const routesFile = ".nself/generated/routes.json"

// Route is the subset of a proxy-routes v1 route this package needs.
type Route struct {
	ID          string     `json:"id"`
	ServerNames []string   `json:"server_names"`
	Locations   []Location `json:"locations"`
}

// Location is one route location.
type Location struct {
	Path     string    `json:"path"`
	Match    string    `json:"match"`
	Upstream *Upstream `json:"upstream"`
}

// Upstream is a location's proxy target.
type Upstream struct {
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
}

// LoadRoutes reads routes.json under projectDir; absent means no routes.
func LoadRoutes(projectDir string) ([]Route, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, routesFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", routesFile, err)
	}
	var doc struct {
		Routes []Route `json:"routes"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", routesFile, err)
	}
	return doc.Routes, nil
}

var nonName = regexp.MustCompile(`[^a-z0-9]+`)

func mapRoutes(v *Values, routes []Route) {
	for _, r := range routes {
		name := strings.Trim(nonName.ReplaceAllString(strings.ToLower(r.ID), "-"), "-")
		up, why := rootUpstream(r)
		if why == "" {
			why = upstreamProblem(v, up)
		}
		if why != "" {
			v.UnmappedRoute = append(v.UnmappedRoute, Unsupported{Name: r.ID, Reason: why})
			continue
		}
		v.Ingress.Rules = append(v.Ingress.Rules, Rule{
			Name: name, Host: r.ServerNames[0], Service: up.Host, Port: up.Port, Scheme: up.Scheme,
		})
	}
	sort.Slice(v.Ingress.Rules, func(i, j int) bool { return v.Ingress.Rules[i].Name < v.Ingress.Rules[j].Name })
}

func rootUpstream(r Route) (*Upstream, string) {
	if len(r.ServerNames) == 0 {
		return nil, "route has no server name"
	}
	for _, l := range r.Locations {
		if l.Path == "/" && l.Match == "prefix" && l.Upstream != nil {
			return l.Upstream, ""
		}
	}
	return nil, "route has no root location with an upstream"
}

func upstreamProblem(v *Values, up *Upstream) string {
	svc, ok := v.Services[up.Host]
	if !ok || (svc.Kind != KindDeployment && svc.Kind != KindStatefulSet) {
		return fmt.Sprintf("upstream %s:%d is not a mapped workload service", up.Host, up.Port)
	}
	for _, p := range svc.Ports {
		if p.Port == up.Port {
			return ""
		}
	}
	return fmt.Sprintf("upstream port %d is not a port of service %s", up.Port, up.Host)
}
