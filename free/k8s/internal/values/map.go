// Purpose: map the resolved compose model to the k8s values and secrets
// documents. Mapping (EPIC D7): long-running service -> Deployment, postgres
// -> StatefulSet, restart "no" -> Job, nginx -> Ingress rules, a service with
// a build context and no image, or with host-level compose features, ->
// unsupported with a reason. Every compose service lands in Services or in
// Unsupported; nothing is approximated silently (per-service NotMapped lists
// the compose features that have no counterpart).
//
// Inputs: Model, optional Routes.
//
// Outputs: Values and Secrets.
//
// Constraints: values.yaml never receives an env value (Env holds names);
// an unresolved "${" in an image or env value is an error that names the
// service and variable, never the value. Image references are only split,
// never invented.
package values

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Map projects the model. routes may be nil (no routes.json).
func Map(m *Model, routes []Route) (*Values, *Secrets, error) {
	v := &Values{
		Project:       m.Name,
		Services:      map[string]Service{},
		Volumes:       map[string]Volume{},
		Unsupported:   []Unsupported{},
		UnmappedRoute: []Unsupported{},
		Ingress:       Ingress{Rules: []Rule{}},
	}
	sec := &Secrets{Secrets: map[string]map[string]string{}}
	for _, name := range sortedKeys(m.Services) {
		cs := m.Services[name]
		if reason := unsupportedReason(name, cs); reason != "" {
			v.Unsupported = append(v.Unsupported, Unsupported{Name: name, Reason: reason})
			continue
		}
		svc, env, err := mapService(name, cs, v)
		if err != nil {
			return nil, nil, err
		}
		if svc.Kind == "" {
			v.Unsupported = append(v.Unsupported, Unsupported{Name: name, Reason: svc.Image})
			continue
		}
		v.Services[name] = svc
		if len(env) > 0 {
			sec.Secrets[name] = env
		}
	}
	mapRoutes(v, routes)
	return v, sec, nil
}

func unsupportedReason(name string, cs ComposeSvc) string {
	switch {
	case cs.Image == "":
		return "build context and no pullable image"
	case !dnsLabel.MatchString(name) || len(name) > 63:
		return "service name is not a valid Kubernetes DNS label"
	case cs.Privileged:
		return "privileged mode has no mapping"
	case cs.NetworkMode != "" && cs.NetworkMode != "default":
		return "network_mode " + cs.NetworkMode + " has no mapping"
	case cs.Pid != "" || cs.Ipc != "":
		return "pid or ipc namespace sharing has no mapping"
	case len(cs.Devices) > 0:
		return "host devices have no mapping"
	}
	return ""
}

// mapService returns the Service and its env map. A zero-Kind result carries
// the unsupported reason in Image.
func mapService(name string, cs ComposeSvc, v *Values) (Service, map[string]string, error) {
	repo, tag, digest := SplitImage(cs.Image)
	if strings.Contains(cs.Image, "${") {
		return Service{}, nil, fmt.Errorf("service %s: image reference has an unresolved interpolation", name)
	}
	svc := Service{Kind: kindOf(name, repo, cs), Image: repo, Tag: tag, Digest: digest}
	if svc.Kind == KindIngress {
		svc.NotMapped = []string{"nginx-config: only the root location of each route becomes an Ingress rule"}
		return svc, nil, nil
	}
	svc.Entrypoint, svc.Command = cs.Entrypoint, cs.Command
	env := map[string]string{}
	for _, k := range sortedKeys(cs.Environment) {
		val := ""
		if cs.Environment[k] != nil {
			val = *cs.Environment[k]
		}
		if strings.Contains(val, "${") {
			return Service{}, nil, fmt.Errorf("service %s: env %s has an unresolved interpolation", name, k)
		}
		svc.Env = append(svc.Env, k)
		env[k] = val
	}
	svc.Ports = mapPorts(cs)
	var notMapped []string
	for _, mt := range cs.Volumes {
		if mt.Type != "volume" {
			notMapped = append(notMapped, mt.Type+"-mount:"+mt.Target)
			continue
		}
		claim := strings.ReplaceAll(strings.ToLower(mt.Source), "_", "-")
		if !dnsLabel.MatchString(claim) {
			return Service{Image: "volume " + mt.Source + " has no valid claim name"}, nil, nil
		}
		v.Volumes[mt.Source] = Volume{Claim: claim}
		svc.Mounts = append(svc.Mounts, Mount{Volume: mt.Source, Path: mt.Target, ReadOnly: mt.ReadOnly})
	}
	for _, t := range cs.Tmpfs {
		notMapped = append(notMapped, "tmpfs-mount:"+t)
	}
	notMapped = append(notMapped, hostFeatures(cs)...)
	svc.Probe, notMapped = mapProbe(cs.Healthcheck, notMapped)
	sort.Strings(notMapped)
	svc.NotMapped = notMapped
	return svc, env, nil
}

func kindOf(name, repo string, cs ComposeSvc) string {
	base := repo[strings.LastIndex(repo, "/")+1:]
	switch {
	case name == "nginx":
		return KindIngress
	case name == "postgres" || base == "postgres":
		return KindStatefulSet
	case cs.Restart == "no":
		return KindJob
	}
	return KindDeployment
}

func mapPorts(cs ComposeSvc) []Port {
	seen := map[Port]bool{}
	add := func(n int, proto string) {
		if n > 0 {
			seen[Port{Port: n, Protocol: strings.ToUpper(orDefault(proto, "tcp"))}] = true
		}
	}
	for _, p := range cs.Ports {
		add(p.Target, p.Protocol)
	}
	for _, e := range cs.Expose {
		if n, err := strconv.Atoi(fmt.Sprint(e)); err == nil {
			add(n, "tcp")
		}
	}
	out := make([]Port, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Protocol < out[j].Protocol
	})
	return out
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// SplitImage splits a reference into repository, tag and digest. A reference
// without tag or digest gets the tag "latest" (what compose pulls).
func SplitImage(ref string) (repo, tag, digest string) {
	if i := strings.LastIndex(ref, "@"); i >= 0 {
		ref, digest = ref[:i], ref[i+1:]
	}
	slash := strings.LastIndex(ref, "/")
	if c := strings.LastIndex(ref, ":"); c > slash {
		return ref[:c], ref[c+1:], digest
	}
	if digest == "" {
		tag = "latest"
	}
	return ref, tag, digest
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
