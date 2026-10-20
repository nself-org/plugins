package main

// Purpose: `nself ci serve admin` (P7-TRUST-12, Epic D19/D20): the group help and the helpers the roles,
// token and audit commands share. The commands run on the coordinator host, where access to the data dir
// is admin access (Epic D1), so the caller is the local operator; the role rules still apply to the
// identities the commands create.
// Inputs:  $NSELF_CI_HOME (state.db), the OS user (the operator's own principal id).
// Outputs: one v1 JSON envelope with --json, else text; exit 0, 1 (usage, refusal, failed verify), 2 (infrastructure).
// Constraints: V15 only; every command here is side-effect class write on the local store only; no
//          command prints a token more than once; the operator principal is the OS user name lowercased
//          and reduced to the id alphabet.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/backup"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// adminEnv is the environment of the serve admin commands; tests replace Open.
type adminEnv struct {
	Open     func(ctx context.Context) (*roles.Service, roles.Actor, func(), error)
	Out, Err io.Writer
	DataDir  string // the data dir the commands write into; "" skips the restore.pending check (tests)
}

// adminRestorePending refuses a command that revokes while an unfinished `serve restore` left its marker: a
// revocation written into the half-restored dir is lost or wrong once the restore runs again (D-1307).
func adminRestorePending(env adminEnv) error {
	if env.DataDir == "" || !backup.Pending(env.DataDir) {
		return nil
	}
	return &adminError{"E668", "an earlier `nself ci serve restore` did not finish on " + env.DataDir + "; run it again (or undo it) before revoking (incomplete_restore)"}
}

var idAlphabet = regexp.MustCompile(`[^a-z0-9._-]+`)

// operatorPrincipal is the operator's own identity: the OS user name in the id alphabet.
func operatorPrincipal() string {
	name := os.Getenv("USER")
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
	}
	p := strings.Trim(idAlphabet.ReplaceAllString(strings.ToLower(name), "-"), "-._")
	if p == "" {
		return "operator"
	}
	if len(p) > 63 {
		p = p[:63]
	}
	return p
}

func productionAdminEnv() adminEnv {
	return adminEnv{Out: os.Stdout, Err: os.Stderr, DataDir: lifecycle.CIHome(), Open: func(ctx context.Context) (*roles.Service, roles.Actor, func(), error) {
		st, err := store.Open(filepath.Join(lifecycle.CIHome(), "state.db"), store.Options{})
		if err != nil {
			return nil, roles.Actor{}, nil, err
		}
		audit.SetScanner(cirun.Redactions)
		svc := roles.New(st, &audit.Log{St: st, Scrub: cirun.Redactions})
		svc.Self = operatorPrincipal()
		roles.Use(svc)
		return svc, svc.Operator(adminActor()), func() { _ = st.Close() }, nil
	}}
}

type adminError struct{ code, msg string }

func (e *adminError) Error() string { return e.msg }

func adminUsage(format string, a ...any) error {
	return &adminError{"E651", fmt.Sprintf(format, a...)}
}

// adminFail prints err (a JSON error envelope with --json) and returns the exit code.
func adminFail(env adminEnv, command string, asJSON bool, err error) int {
	code, exit := "E607", output.ExitInfra
	var ae *adminError
	var rerr *roles.Error
	var aerr *audit.Error
	var serr *store.Error
	switch {
	case errors.As(err, &ae):
		code, exit = ae.code, output.ExitUser
	case errors.As(err, &rerr):
		code, exit = roles.CodeNotAuthorized, output.ExitUser
	case errors.As(err, &aerr):
		code, exit = audit.CodeRefused, output.ExitUser
	case errors.As(err, &serr):
		code = serr.Code
	}
	msg := strings.Join(strings.Fields(err.Error()), " ")
	if asJSON {
		_ = output.WriteError(env.Out, command, output.ErrorDetail{Code: code, Message: msg, ExitCode: exit}, nil)
	} else {
		_, _ = io.WriteString(env.Err, msg+"\n")
	}
	return exit
}

// plainText replaces control characters, so a row written by another package cannot forge lines or
// terminal escapes in the text output.
func plainText(s string) string {
	return strings.Map(func(r rune) rune {
		// C0 and C1 controls, and the bidirectional overrides that reorder a terminal line.
		if r < 0x20 || (r >= 0x7f && r < 0xa0) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return '?'
		}
		return r
	}, s)
}

func adminHasJSON(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "-json" || a == "--json=true" || a == "-json=true" {
			return true
		}
	}
	return false
}

// adminParse parses flags that may follow positional words; it returns the positionals.
func adminParse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, adminUsage("%s", err.Error())
		}
		args = fs.Args()
		if len(args) > 0 {
			pos, args = append(pos, args[0]), args[1:]
		}
	}
	return pos, nil
}

func adminWriteData(env adminEnv, command string, data any) int {
	if output.WriteData(env.Out, command, data, nil) != nil {
		return output.ExitInfra
	}
	return 0
}

const serveAdminHelp = `nself ci serve admin <group> ...  (coordinator host; role grants, scoped tokens, audit log)

  roles grant <principal> --role member|release|admin --project <id>... [--self] [--json]
  roles revoke <principal> [--json]
  roles list [--json]
  token issue <principal> --role member|release|admin --project <id>... [--ttl 720h] [--self] [--json]
  token revoke <id> [--json]
  token list [--json]
  audit [--since <rfc3339|duration>] [--verify] [--json]
  audit seal [--json]
`

func init() {
	registerV15("serve admin", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { return flag.NewFlagSet("serve admin", flag.ContinueOnError) },
		Handler: func(args []string) int {
			fmt.Fprint(os.Stdout, serveAdminHelp)
			if len(args) > 0 && args[0] != "--help" && args[0] != "-h" && args[0] != "help" {
				return output.ExitUser
			}
			return 0
		},
	})
}
