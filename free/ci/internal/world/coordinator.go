package world

import (
	"context"
	"net"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func localAddresses(ctx context.Context, d Deps) (map[string]bool, error) {
	result := map[string]bool{"127.0.0.1": true, "::1": true}
	if d.LocalAddresses != nil {
		addresses, err := d.LocalAddresses(ctx)
		if err != nil {
			return nil, err
		}
		for _, v := range addresses {
			result[v] = true
		}
		return result, nil
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, address := range addresses {
		if network, ok := address.(*net.IPNet); ok {
			result[network.IP.String()] = true
		}
	}
	return result, nil
}
func coordinatorNode(ctx context.Context, d Deps, c model.Capability, machine string, local map[string]bool, peers map[string]string) (bool, error) {
	if machine != "" && c.Identity.MachineID.Value != nil && *c.Identity.MachineID.Value == machine {
		return true, nil
	}
	if c.Identity.Transport == "agent" {
		peer := strings.TrimSpace(peers[c.Identity.ID])
		if peer == "" {
			return true, nil
		} // Missing peer evidence fails closed.
		ip := net.ParseIP(strings.Trim(peer, "[]"))
		if ip == nil {
			return true, nil
		} // Invalid peer evidence is also unknown.
		return ip.IsLoopback() || local[ip.String()], nil
	}
	if c.Identity.SSH == nil {
		return false, nil
	}
	host := strings.Trim(c.Identity.SSH.Hostname, "[]")
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || local[ip.String()], nil
	}
	var addresses []string
	if d.ResolveHost != nil {
		var err error
		addresses, err = d.ResolveHost(ctx, host)
		if err != nil {
			return false, err
		}
	} else {
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return false, err
		}
		for _, ip := range ips {
			addresses = append(addresses, ip.IP.String())
		}
	}
	for _, address := range addresses {
		if ip := net.ParseIP(address); ip != nil && (ip.IsLoopback() || local[ip.String()]) {
			return true, nil
		}
	}
	return false, nil
}
