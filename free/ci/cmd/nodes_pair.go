package main

// Purpose: `nself ci nodes pair` (P7-NODE-07, Epic D5): the foreground enrollment window of a PERSONAL or
//          TEAM coordinator. It also registers the PKI's lifecycle hooks: the revoker that puts a node's
//          certificate serials on the deny list inside the revoke transaction, and the rotate modes
//          `cert` and `node-key` (flags --cert, --node-key, --key-id on `nodes rotate`).
// Inputs:  --enroll-addr (default :3866), --data-dir (the CI home; default $NSELF_CI_HOME), --public-url
//          (its host becomes the SANs of the server leaf), --label, --cert-ttl, --coordinator-id,
//          --agent-url; the admin answers the approval prompt on stdin.
// Outputs: the enroll URL, the pairing code (printed once) and the root fingerprint on stdout. Exit 0 when
//          a node enrolled, 1 when the window ended without one, 2 on an infrastructure error.
// Constraints: V15 only; side-effect class remote (it binds a listener for the window); the listener binds
//          only while the window is open (code redeemed, locked, expired after 10 minutes or Ctrl-C);
//          the code never appears on argv or in the environment and is never logged; the admin approves
//          by typing the CSR key fingerprint the node printed (the default Approver).

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// Rotate modes this file registers with the lifecycle registry.
const (
	rotateCert    = "cert"
	rotateNodeKey = "node-key"
)

// pairFlags are the flags of `nodes pair`.
type pairFlags struct {
	addr, dataDir, publicURL, label, coordinatorID, agentURL string
	certTTL                                                  time.Duration
}

func newPairFlags(f *pairFlags, out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("nodes pair", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&f.addr, "enroll-addr", ":3866", "address of the enrollment listener (open only while pairing)")
	fs.StringVar(&f.dataDir, "data-dir", "", "CI data directory of the coordinator (default $NSELF_CI_HOME, else ~/.nself/ci)")
	fs.StringVar(&f.publicURL, "public-url", "", "URL nodes use to reach this host (its host name or IP goes into the server certificate)")
	fs.StringVar(&f.label, "label", "", "note shown to the admin when a node redeems the code")
	fs.StringVar(&f.coordinatorID, "coordinator-id", "local", "coordinator id in the server certificate")
	fs.StringVar(&f.agentURL, "agent-url", "", "agent API URL handed to the node (empty when no coordinator serves one yet)")
	fs.DurationVar(&f.certTTL, "cert-ttl", 0, "node certificate lifetime (default 7 days; 1 minute to 7 days)")
	return fs
}

func init() {
	registerV15("nodes pair", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { return newPairFlags(&pairFlags{}, io.Discard) },
		Handler: func(args []string) int { return runNodesPair(args, os.Stdin, os.Stdout, os.Stderr) },
	})
	lifecycle.RegisterRevoker(pki.RevokeHook(time.Now))
	lifecycle.RegisterRotateMode(rotateCert, rotateCertMode)
	lifecycle.RegisterRotateMode(rotateNodeKey, rotateNodeKeyMode)
	lifecycle.RegisterFlags("rotate", func(fs *flag.FlagSet) {
		fs.BoolFunc("cert", "rotate the node's certificates: revoke them so the node must pair again", func(string) error { return fs.Set("mode", rotateCert) })
		fs.BoolFunc("node-key", "rotate the pinned node signing key (needs --key-id)", func(string) error { return fs.Set("mode", rotateNodeKey) })
		fs.String("key-id", "", "node-key id the node printed when it generated its new key (with --node-key)")
	})
}

func runNodesPair(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var f pairFlags
	fs := newPairFlags(&f, stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: nself ci nodes pair [--enroll-addr A] [--data-dir D] [--public-url U] [--label L] [--cert-ttl T]")
		return 2
	}
	if f.dataDir != "" {
		_ = os.Setenv("NSELF_CI_HOME", f.dataDir)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	d, closeFn, err := lifecycle.ProductionEnv(currentProject).Open(ctx, "")
	if err != nil {
		fmt.Fprintf(stderr, "E607: the CI store cannot be opened: %v\n", err)
		return 2
	}
	defer closeFn()
	ca, err := pki.New(ctx, pki.Options{Store: d.Store, DataDir: d.Home})
	if err != nil {
		fmt.Fprintf(stderr, "E660: the coordinator PKI cannot be opened: %v\n", err)
		return 2
	}
	public := f.publicURL
	if public == "" {
		public = defaultPublicURL(f.addr, interfaceIPs())
	}
	svc := pairing.New(pairing.Config{
		CA: ca, Store: d.Store, Registry: d.Registry, CoordinatorID: f.coordinatorID, AgentURL: f.agentURL, Actor: d.Actor,
		Approver: &pairing.PromptApprover{In: bufio.NewReader(stdin), Out: stdout},
		AfterIssue: func(ctx context.Context, nodeID string) error {
			return lifecycle.RunHooks(ctx, lifecycle.PointPair, lifecycle.HookEvent{Point: lifecycle.PointPair, NodeID: nodeID, Project: d.Project, Flags: fs})
		},
		Alert: func(a pairing.Alert) {
			fmt.Fprintf(stderr, "ALERT %s: offer %s node %s: a used code was presented again; its certificate was revoked\n", a.Kind, a.OfferID, a.NodeID)
		},
	})
	pairing.Use(svc)
	w, err := svc.OpenWindow(ctx, f.addr, public, pairing.OfferOptions{Label: f.label, CertTTL: f.certTTL})
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "Pairing window open until %s.\n  enroll URL:       %s\n  pairing code:     %s\n  root fingerprint: %s\nOn the node run: nself-ci-agent join %s\nThe node shows the root fingerprint; compare it with the one above before confirming.\n",
		w.Offer.ExpiresAt.Local().Format(time.TimeOnly), w.URL, w.Offer.Code, w.Offer.RootFingerprint, w.URL)
	redeemed, err := w.Wait(ctx, svc)
	switch {
	case err != nil:
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	case !redeemed:
		fmt.Fprintln(stderr, "The pairing window closed without a node joining.")
		return 1
	}
	fmt.Fprintln(stdout, "A node joined. The pairing window is closed.")
	return 0
}

// interfaceIPs lists the addresses of this machine's interfaces.
func interfaceIPs() []net.IP {
	var out []net.IP
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			out = append(out, n.IP)
		}
	}
	return out
}

// defaultPublicURL is what nodes are told to reach when --public-url is absent. A bind address that names
// a host is that host; a wildcard bind (":3866", "0.0.0.0:3866") becomes this machine's first
// non-loopback IPv4 address, so the server certificate carries an IP SAN a LAN node can verify, and
// "localhost" only when the machine has none.
func defaultPublicURL(addr string, ips []net.IP) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "https://localhost"
	}
	if ip := net.ParseIP(host); host != "" && (ip == nil || !ip.IsUnspecified()) {
		return "https://" + addr
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil && !v4.IsLoopback() && !v4.IsLinkLocalUnicast() {
			return "https://" + net.JoinHostPort(v4.String(), port)
		}
	}
	return "https://" + net.JoinHostPort("localhost", port)
}

func pkiOf(ctx context.Context, d *lifecycle.Deps) (*pki.CA, error) {
	return pki.New(ctx, pki.Options{Store: d.Store, DataDir: d.Home})
}

// rotateCertMode denies every certificate of the node; it must pair again (an expired or denied
// certificate cannot renew).
func rotateCertMode(ctx context.Context, d *lifecycle.Deps, r lifecycle.RotateRequest) error {
	ca, err := pkiOf(ctx, d)
	if err != nil {
		return err
	}
	n, err := ca.Revoke(ctx, d.Actor, r.NodeID, "rotate_cert")
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "%d certificate(s) of %s denied. The node must pair again: nself ci nodes pair, then nself-ci-agent join on the node.\n", n, r.NodeID)
	return nil
}

// rotateNodeKeyMode makes the key id the node printed the only key its next hello may present.
func rotateNodeKeyMode(ctx context.Context, d *lifecycle.Deps, r lifecycle.RotateRequest) error {
	id := ""
	if r.Flags != nil {
		if fl := r.Flags.Lookup("key-id"); fl != nil {
			id = strings.TrimSpace(fl.Value.String())
		}
	}
	ca, err := pkiOf(ctx, d)
	if err != nil {
		return err
	}
	if err = ca.RotateNodeKey(ctx, d.Actor, r.NodeID, id); err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "The next hello of %s may present node key %s only.\n", r.NodeID, id)
	return nil
}
