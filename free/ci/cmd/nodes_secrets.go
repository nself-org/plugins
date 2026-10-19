package main

// Purpose: `nself ci nodes secrets grant|revoke <node> --class <class>[/<env>]`: add or remove a node's
// age recipient on one secret store file, through the cli's `config secrets recipients` (P7-TRUST-02).
// Inputs:  a node id from the registry, a class (project, environment/<env>, release, deploy/<env>),
// --project (default: the current directory, made absolute).
// Outputs: one line on success; "Error: E677 ..." on a refusal. Exit 0, or 1 for any failure.
// Constraints: grant refuses a node whose registry entry has `secrets_eligible: false` (E677,
// secrets.node_not_eligible, naming the node) BEFORE any cli call and before any audit row; release and
// deploy files only go to a dedicated-trusted node authorized for that file's project (D-1071); one audit row per change, written after the cli call
// succeeded; revoke is never refused for eligibility (taking access away must always work). The call
// runs as the operator through the exec funnel and never on the node.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// nodesSecretsDeps are the seams the tests replace.
type nodesSecretsDeps struct {
	OpenStore func() (*store.Store, error)
	Runner    secrets.Runner
	Binary    string
	Actor     string
	Getwd     func() (string, error)
	ProjectID func(ctx context.Context, root string) string
	Stdout    io.Writer
	Stderr    io.Writer
}

func defaultNodesSecretsDeps() nodesSecretsDeps {
	actor := os.Getenv("USER")
	if actor == "" {
		actor = "operator"
	}
	return nodesSecretsDeps{
		OpenStore: func() (*store.Store, error) { return cirun.OpenStore(runsStorePath()) },
		Runner:    &secrets.ExecRunner{CoordinatorID: "secrets-grant"},
		Actor:     actor, Getwd: os.Getwd, Stdout: os.Stdout, Stderr: os.Stderr,
		ProjectID: func(ctx context.Context, root string) string {
			return policy.ProjectIDGit(ctx, policy.Runner(report.Capture), root)
		},
	}
}

func nodesSecretsFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("nodes secrets", flag.ContinueOnError)
	fs.String("class", "", "secret class: project, release, environment/<env> or deploy/<env>")
	fs.String("project", "", "project directory (default: the current directory)")
	return fs
}

func init() {
	for _, verb := range []string{"grant", "revoke"} {
		verb := verb
		registerV15("nodes secrets "+verb, subcommandEntry{V15Only: true, Flags: nodesSecretsFlags,
			Handler: func(args []string) int { return runNodesSecrets(verb, args, defaultNodesSecretsDeps()) }})
	}
}

// dedicatedNode applies the Epic D4 definition to a registry entry: operator-owned, accepts exactly
// owner, one authorized project.
func dedicatedNode(c model.Capability) bool {
	d := trust.Declaration{Source: "assigned", Dedicated: true, Projects: len(c.Lifecycle.AuthorizedProjects)}
	// only operator-assigned facts count: a value the node reported about itself never makes it dedicated
	if v := c.Identity.Ownership.Value; v != nil && c.Identity.Ownership.Source == "assigned" {
		d.Ownership = *v
	}
	if v := c.Trust.Accepts.Value; v != nil && c.Trust.Accepts.Source == "assigned" {
		d.Accepts = append([]model.TrustClass(nil), (*v)...)
	}
	return trust.IsDedicatedTrusted(d)
}

// dedicatedGrantRefusal is the E677 refusal for a release or deploy grant, or nil. The file goes only to
// a dedicated node whose single authorized project is the project the file belongs to (projectID is
// policy.ProjectIDGit of --project, the form the authorization is written in); other classes are not bound.
func dedicatedGrantRefusal(nodeID string, class model.SecretClass, c model.Capability, projectID string) *secrets.Error {
	if class != "release" && class != "deploy" {
		return nil
	}
	if !dedicatedNode(c) {
		return &secrets.Error{Code: "E677", Reasons: []string{string(trust.TrustReleaseNeedsDedicated)},
			Message: fmt.Sprintf("node %s is not dedicated-trusted: %s files go only to a dedicated node (%s)", nodeID, class, trust.TrustReleaseNeedsDedicated)}
	}
	if p := c.Lifecycle.AuthorizedProjects; len(p) != 1 || p[0] != projectID {
		return &secrets.Error{Code: "E677", Reasons: []string{secrets.ReasonNodeOtherProject},
			Message: fmt.Sprintf("node %s is dedicated to another project (%s): %s files go only to the node dedicated to %s", nodeID, secrets.ReasonNodeOtherProject, class, projectID)}
	}
	return nil
}

func runNodesSecrets(verb string, args []string, d nodesSecretsDeps) int {
	fail := func(err error) int { fmt.Fprintf(d.Stderr, "Error: %v\n", err); return 1 }
	var nodeID string
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		nodeID, args = args[0], args[1:]
	}
	fs := nodesSecretsFlags()
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || nodeID == "" {
		fmt.Fprintf(d.Stderr, "usage: nself ci nodes secrets %s <node> --class <class>[/<env>] [--project <dir>]\n", verb)
		return 1
	}
	class, env, err := secrets.ParseClass(fs.Lookup("class").Value.String())
	if err != nil {
		return fail(err)
	}
	project := fs.Lookup("project").Value.String()
	if project == "" {
		if project, err = d.Getwd(); err != nil {
			return fail(err)
		}
	}
	if project, err = filepath.Abs(project); err != nil {
		return fail(err)
	}
	ctx := context.Background()
	st, err := d.OpenStore()
	if err != nil {
		return fail(err)
	}
	defer func() { _ = st.Close() }()
	node, err := registry.New(st).Get(ctx, nodeID)
	if err != nil {
		return fail(fmt.Errorf("node %q: %w", nodeID, err))
	}
	c := node.Capability
	if verb == "grant" {
		if !c.SecretsEligible {
			return fail(&secrets.Error{Code: "E677", Reasons: []string{secrets.ReasonNodeNotEligible},
				Message: fmt.Sprintf("node %s is not secrets-eligible (%s): its jobs could read the node's age key, so it gets no recipient", nodeID, secrets.ReasonNodeNotEligible)})
		}
		if refusal := dedicatedGrantRefusal(nodeID, class, c, d.ProjectID(ctx, project)); refusal != nil {
			return fail(refusal)
		}
	}
	recipient := ""
	if v := c.Identity.AgeRecipient.Value; v != nil {
		recipient = *v
	}
	if recipient == "" {
		return fail(&secrets.Error{Code: "E677", Reasons: []string{secrets.ReasonNodeNoRecipient},
			Message: fmt.Sprintf("node %s has no age recipient yet (%s): the agent creates its identity on first start", nodeID, secrets.ReasonNodeNoRecipient)})
	}
	file := secrets.StoreFileOf(class, env)
	cliVerb := map[string]string{"grant": "add", "revoke": "remove"}[verb]
	if err = secrets.Recipients(ctx, d.Runner, d.Binary, project, cliVerb, file, recipient, class); err != nil {
		return fail(err)
	}
	if err = st.AppendAudit(ctx, d.Actor, "node.secrets."+verb, nodeID, "class="+string(class)+";file="+file); err != nil {
		return fail(fmt.Errorf("the %s took effect on store file %s but its audit row could not be written: %w", verb, file, err))
	}
	fmt.Fprintf(d.Stdout, "%s: node %s, store file %s\n", verb, nodeID, file)
	if verb == "revoke" {
		fmt.Fprintln(d.Stdout, "Removal does not revoke plaintext the node already read: rotate the values in that file.")
	}
	return 0
}
