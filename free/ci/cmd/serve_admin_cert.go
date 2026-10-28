package main

// Purpose: issue user mTLS credentials on the coordinator host (D-1274, review F3).
// Inputs: cert issue <principal>, --self, --ttl and --json.
// Outputs: the certificate, private key and root once, on stdout.
// Constraints: admin only; --self names only the local operator. Record the subject
// before returning credentials; a failed record leaves a conservative non-self count.

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type adminCertFlags struct {
	json, self bool
	ttl        time.Duration
}

func adminCertFlagSet(f *adminCertFlags, w io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("serve admin cert", flag.ContinueOnError)
	fs.SetOutput(w)
	fs.BoolVar(&f.json, "json", false, "print credentials once in one JSON envelope")
	fs.BoolVar(&f.self, "self", false, "certificate is for the operator's own principal")
	fs.DurationVar(&f.ttl, "ttl", pki.DefaultTTL, "certificate lifetime (1m to 168h)")
	return fs
}

func init() {
	registerV15("serve admin cert", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { var f adminCertFlags; return adminCertFlagSet(&f, io.Discard) },
		Handler: func(args []string) int { return runAdminCert(args, productionAdminEnv()) },
	})
}

func runAdminCert(args []string, env adminEnv) int {
	const command = "ci serve admin cert"
	var f adminCertFlags
	pos, err := adminParse(adminCertFlagSet(&f, env.Err), args)
	if err != nil {
		return adminFail(env, command, adminHasJSON(args), err)
	}
	if len(pos) != 2 || pos[0] != "issue" || !pki.ValidID(pos[1]) {
		return adminFail(env, command, f.json, adminUsage("usage: cert issue <principal> [--self] [--ttl 168h] [--json]"))
	}
	if _, err = pki.CheckTTL(f.ttl); err != nil {
		return adminFail(env, command, f.json, adminUsage("%s", err))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	svc, op, closeFn, err := env.Open(ctx)
	if err != nil {
		return adminFail(env, command, f.json, err)
	}
	defer closeFn()
	if err = op.Can(roles.Admin, ""); err != nil {
		return adminFail(env, command, f.json, err)
	}
	principal, subject := pos[1], pos[1]
	if f.self {
		if svc.Self == "" || principal != svc.Self {
			return adminFail(env, command, f.json, adminUsage("--self applies only to the operator's own identity"))
		}
		subject = store.SubjectSelf
	} else if principal == store.SubjectSelf {
		return adminFail(env, command, f.json, adminUsage("self is reserved for the operator's own identity (--self)"))
	}
	ca, err := pki.New(ctx, pki.Options{Store: svc.St, DataDir: lifecycle.CIHome(), Now: svc.Now})
	if err != nil {
		return adminFail(env, command, f.json, err)
	}
	b, err := ca.IssueUser(ctx, principal, f.ttl)
	if err != nil {
		return adminFail(env, command, f.json, err)
	}
	block, _ := pem.Decode(b.CertPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return adminFail(env, command, f.json, err)
	}
	serial := hex.EncodeToString(cert.SerialNumber.Bytes())
	if err = svc.St.CertSubjectPut(ctx, serial, principal, subject); err != nil {
		return adminFail(env, command, f.json, err)
	}
	if err = svc.Log.Append(ctx, audit.Event{Actor: op.Label(), Action: "cert.issue", Target: principal, Detail: fmt.Sprintf("serial=%s subject=%s", serial, subject)}); err != nil {
		return adminFail(env, command, f.json, err)
	}
	if f.json {
		return adminWriteData(env, command, map[string]any{"principal": principal, "subject": subject, "serial": serial, "cert_pem": string(b.CertPEM), "key_pem": string(b.KeyPEM), "root_pem": string(b.RootPEM), "shown_once": true})
	}
	for _, data := range [][]byte{b.CertPEM, b.KeyPEM, b.RootPEM} {
		if _, err = env.Out.Write(data); err != nil {
			return 2
		}
	}
	return 0
}
