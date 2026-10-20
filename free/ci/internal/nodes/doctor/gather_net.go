package doctor

// Purpose: the live readings of the node checks: pinned host keys, a TCP dial to each ssh node, the node clock
// over the pinned ssh route and the visibility of the repository doctor runs in.
// Inputs:  Facts.Nodes, the CI home, Deps and the doctor Env.
// Outputs: Facts.Pins, Reach, Skew, SkewErr and RepoVisibility.
// Constraints: one fixed argv on a node (`date -u +%s`), only over the D4 option set and the pinned key, never
//          trust-on-first-use (a node with no pin is not asked for its clock); every reading has its own short timeout
//          inside the section budget and runs in parallel; no secret is read or sent; the dial sends no payload. The
//          visibility reading runs `git remote get-url origin` and one `gh api repos/<owner>/<repo>` call, only when a
//          persistent GitHub runner is registered.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/doctor"
)

const (
	dialTimeout = 3 * time.Second
	skewTimeout = 4 * time.Second
	pinPrefix   = "nself-ci-" // the HostKeyAlias prefix `nodes add` pins under
)

func dialTCP(ctx context.Context, addr string) error {
	dctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	c, err := (&net.Dialer{}).DialContext(dctx, "tcp", addr)
	if err != nil {
		return err
	}
	return c.Close()
}

// sshHost is the dial address and ssh destination of a node; ok is false when the node has no hostname (an alias
// resolves only inside ssh_config, so it is not dialled).
func sshHost(n Node) (addr string, spec remote.HostSpec, ok bool) {
	s := n.Cap.Identity.SSH
	if s == nil || s.Hostname == "" {
		return "", remote.HostSpec{}, false
	}
	port := s.Port
	if port <= 0 {
		port = 22
	}
	dest := s.Hostname
	if s.User != "" {
		dest = s.User + "@" + dest
	}
	if s.Port > 0 {
		dest += ":" + strconv.Itoa(s.Port)
	}
	spec, err := remote.ParseHostSpec(dest)
	if err != nil || spec.LegacyPath != "" {
		return "", remote.HostSpec{}, false
	}
	return net.JoinHostPort(s.Hostname, strconv.Itoa(port)), spec, true
}

// readNet fills the pin, reach, skew and visibility facts. The three per-node readings run in parallel.
func (f *Facts) readNet(ctx context.Context, env doctor.Env, home string, d Deps) {
	ssh := f.sshNodes()
	if home == "" {
		ssh = nil // no CI home, no pin file, no route that may be used
	}
	pins := remote.PinnedHostKeys{Path: filepath.Join(home, "known_hosts")}
	for _, n := range ssh {
		keys, err := pins.Lookup(pinPrefix + n.ID())
		if err != nil {
			f.PinsErr = err
			break
		}
		for _, k := range keys {
			if fp, fpErr := remote.Fingerprint(k); fpErr == nil {
				f.Pins[n.ID()] = append(f.Pins[n.ID()], fp)
			}
		}
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, n := range ssh {
		addr, spec, ok := sshHost(n)
		if !ok {
			continue
		}
		id := n.ID()
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := d.Dial(ctx, addr)
			mu.Lock()
			f.Reach[id] = err
			mu.Unlock()
			if err != nil {
				return
			}
			skew, refused, sErr := f.readSkew(ctx, id, spec, home, d)
			mu.Lock()
			defer mu.Unlock()
			if sErr != nil {
				f.SkewErr[id], f.HostKeyRefused[id] = sErr, refused
				return
			}
			f.Skew[id] = skew
		}()
	}
	wg.Wait()
	f.readVisibility(ctx, env)
}

// readSkew returns remote minus local time over the pinned route. A node with no pin or an unusable local ssh is
// not contacted.
func (f *Facts) readSkew(ctx context.Context, id string, spec remote.HostSpec, home string, d Deps) (skew time.Duration, refused bool, err error) {
	if len(f.Pins[id]) == 0 {
		return 0, false, fmt.Errorf("no pinned host key")
	}
	if f.SSHVersionErr != nil || !f.SSHVersion.AtLeast(8, 0) {
		return 0, false, fmt.Errorf("local OpenSSH is not usable")
	}
	opts := append(remote.CISSHFlags(), remote.CIOptions(id, filepath.Join(home, "known_hosts"), f.SSHVersion)...)
	target, tErr := spec.Target(opts)
	if tErr != nil {
		return 0, false, fmt.Errorf("invalid destination")
	}
	rctx, cancel := context.WithTimeout(ctx, skewTimeout)
	defer cancel()
	before := d.Now()
	out, runErr := d.RunRemote(rctx, target, "date", "-u", "+%s")
	after := d.Now()
	if runErr != nil {
		reason, refused := sshReason(runErr)
		return 0, refused, errors.New(reason)
	}
	secs, pErr := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if pErr != nil {
		return 0, false, fmt.Errorf("the clock output was not a time")
	}
	mid := before.Add(after.Sub(before) / 2)
	return time.Unix(secs, 0).Sub(mid), false, nil
}

var githubRemote = regexp.MustCompile(`github\.com[:/]([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?\s*$`)

// readVisibility asks GitHub whether the repository doctor runs in is public, but only when a persistent GitHub
// runner is registered (otherwise nothing depends on the answer).
func (f *Facts) readVisibility(ctx context.Context, env doctor.Env) {
	need := false
	for _, n := range f.Nodes {
		need = need || strings.HasPrefix(n.Cap.Identity.Provider, "github") && boolFact(n.Cap.Availability.Persistence)
	}
	if !need || env.Capture == nil || env.Root == "" {
		return
	}
	out, exit, err := env.Capture(ctx, "git", "-C", env.Root, "remote", "get-url", "origin")
	if err != nil || exit != 0 {
		return
	}
	m := githubRemote.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return
	}
	out, exit, err = env.Capture(ctx, "gh", "api", "repos/"+m[1]+"/"+m[2], "--jq", ".visibility")
	if err != nil || exit != 0 {
		return
	}
	if v := strings.TrimSpace(out); v == "public" || v == "private" || v == "internal" {
		f.RepoVisibility = v
	}
}

// sshReason turns an ssh error into a short reason. refused is true when ssh rejected the host key against the pin:
// the node now presents a key other than the pinned one (CI-DN002 fails on it).
func sshReason(err error) (reason string, refused bool) {
	line, _, _ := strings.Cut(strings.TrimSpace(err.Error()), "\n")
	line = strings.TrimRight(line, "\r")
	if len(line) > 120 {
		line = strings.ToValidUTF8(line[:120], "")
	}
	if strings.Contains(strings.ToLower(err.Error()), "host key verification failed") {
		return "host key verification failed against the pin", true
	}
	return "the clock could not be read: " + line, false
}
