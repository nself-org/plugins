package probe

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestRemoteCommandsStatic proves every remote command is a source literal: each cmd* constant
// is one basic string literal (no concatenation, no identifier, no call), and the status list
// holds only those literals.
func TestRemoteCommandsStatic(t *testing.T) {
	fset := token.NewFileSet()
	literals := map[string]bool{}
	for _, file := range []string{"commands.go", "demand_read.go"} {
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if !strings.HasPrefix(name.Name, "cmd") {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						t.Errorf("%s: %s is not one string literal", file, name.Name)
						continue
					}
					literals[name.Name] = true
				}
			}
		}
	}
	for _, name := range []string{"cmdDemand", "cmdReadiness"} {
		if !literals[name] {
			t.Errorf("%s is not a literal constant", name)
		}
	}
	got := StatusCommands()
	if len(got) != 2 || got[0] != cmdDemand || got[1] != cmdReadiness {
		t.Fatalf("StatusCommands = %q", got)
	}
	for _, c := range got {
		if strings.Contains(c, "%") || !strings.HasPrefix(c, "cat ") {
			t.Errorf("status command %q is not a plain read", c)
		}
	}
	got[0] = "rm -rf /"
	if StatusCommands()[0] != cmdDemand {
		t.Error("StatusCommands exposes its backing array")
	}
}

func TestReadStatusSendsOnlyLiterals(t *testing.T) {
	var sent []string
	run := func(_ context.Context, command string) (string, error) {
		sent = append(sent, command)
		if command == cmdDemand {
			return `{"ts":"2026-10-27T12:00:00Z","ready":true}`, nil
		}
		return "", nil
	}
	docs, err := ReadStatus(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) != 2 || sent[0] != cmdDemand || sent[1] != cmdReadiness {
		t.Fatalf("sent = %q", sent)
	}
	if !json.Valid(docs.Demand) || docs.Readiness != nil {
		t.Fatalf("docs = %+v", docs)
	}
	boom := errors.New("connection refused")
	if _, err := ReadStatus(context.Background(), func(context.Context, string) (string, error) { return "", boom }); !errors.Is(err, boom) {
		t.Fatalf("transport error = %v", err)
	}
	if _, err := ReadStatus(context.Background(), nil); err == nil {
		t.Fatal("nil runner accepted")
	}
}

func TestSSHStatusRunnerRefusesOtherCommands(t *testing.T) {
	n := registry.Node{Record: store.NodeRecord{ID: "n1"}}
	if _, err := SSHStatusRunner(n, "/pins", "", remote.Version{Major: 9, Minor: 6}); err == nil {
		t.Fatal("node without SSH identity accepted")
	}
	n.Capability.Identity.SSH = &model.CapabilitySSH{Hostname: "builder.lan", User: "ci", Port: 2222}
	if _, err := SSHStatusRunner(n, "", "", remote.Version{Major: 9, Minor: 6}); err == nil {
		t.Fatal("missing pin file accepted")
	}
	run, err := SSHStatusRunner(n, "/pins", "", remote.Version{Major: 9, Minor: 6})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "cat /etc/shadow"); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("non-literal command ran: %v", err)
	}
}
