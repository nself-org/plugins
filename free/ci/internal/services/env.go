package services

// Purpose: the connection environment a started service adds to a job.
// Inputs:  the specs, their host ports and credentials, the compose network.
// Outputs: NSELF_CI_SERVICE_<NAME>_* variables.
// Constraints: both access paths are injected, because the services hook does
// not know whether the job runs as a process or in a container. A process job
// uses HOST/PORT/URL (127.0.0.1 and the random host port). A container job uses
// NET_HOST/NET_PORT/NET_URL (the compose service name and the container port)
// on the network named by NSELF_CI_SERVICE_NETWORK. The password is random per
// provision and is never written to a log.

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// envKey turns a service name into the NAME part of the variables.
var envKeyUnsafe = regexp.MustCompile(`[^A-Z0-9]+`)

func envKey(name string) string {
	return strings.Trim(envKeyUnsafe.ReplaceAllString(strings.ToUpper(name), "_"), "_")
}

// conn is one started service.
type conn struct {
	Spec     Spec
	Comp     component
	HostPort int
	User, DB string
	Password string
}

func (c conn) url(host string, port int) string {
	if c.Comp.Scheme == "" {
		return ""
	}
	u := url.URL{Scheme: c.Comp.Scheme, Host: fmt.Sprintf("%s:%d", host, port)}
	switch {
	case c.User != "":
		u.User = url.UserPassword(c.User, c.Password)
	case c.Password != "":
		u.User = url.UserPassword("", c.Password)
	}
	if c.DB != "" {
		u.Path = "/" + c.DB
	}
	return u.String()
}

// buildEnv assembles the variables for conns and the compose network.
func buildEnv(conns []conn, network string) map[string]string {
	env := map[string]string{}
	var names []string
	for _, c := range conns {
		k := "NSELF_CI_SERVICE_" + envKey(c.Spec.Name) + "_"
		names = append(names, c.Spec.Name)
		env[k+"HOST"] = "127.0.0.1"
		env[k+"PORT"] = fmt.Sprint(c.HostPort)
		env[k+"NET_HOST"] = c.Comp.ComposeName
		env[k+"NET_PORT"] = fmt.Sprint(c.Comp.ContainerPort)
		if c.User != "" {
			env[k+"USER"] = c.User
		}
		if c.DB != "" {
			env[k+"DB"] = c.DB
		}
		if c.Password != "" {
			env[k+"PASSWORD"] = c.Password
		}
		if u := c.url("127.0.0.1", c.HostPort); u != "" {
			env[k+"URL"] = u
			env[k+"NET_URL"] = c.url(c.Comp.ComposeName, c.Comp.ContainerPort)
		}
	}
	sort.Strings(names)
	env["NSELF_CI_SERVICES"] = strings.Join(names, ",")
	if network != "" {
		env["NSELF_CI_SERVICE_NETWORK"] = network
	}
	return env
}
