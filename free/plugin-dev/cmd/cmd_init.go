package main

// Purpose: `init <name>` (and its deprecated alias `new`): scaffold a plugin
// project. Port of core cmd/commands/plugin_new.go.
// Inputs:  the scaffold flags and one plugin name.
// Outputs: the generated tree (internal/scaffold) and the same status lines as core.
// Constraints: the tenancy prompt reads stdin only when no --tenancy value is
// given, --no-interactive is unset and stdin is a character device, as in core.

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-plugin-dev/internal/clui"
	"github.com/nself-org/nself-plugin-dev/internal/scaffold"
)

// initData is the --json data of `init` and `new`.
type initData struct {
	Name     string   `json:"name"`
	Template string   `json:"template"`
	Tier     string   `json:"tier"`
	Tenancy  string   `json:"tenancy"`
	Dir      string   `json:"dir"`
	Files    []string `json:"files"`
}

func runInit(p *parsed) (any, error) {
	name := p.args[0]
	tmplType := p.str("template")
	tier := p.str("tier")

	switch tmplType {
	case "go", "rust", "node", "static":
	default:
		return nil, usageErr(fmt.Errorf("unknown template %q: must be go, rust, node, or static", tmplType),
			"unknown template: must be go, rust, node, or static")
	}

	tenancy, err := resolveTenancy(p.str("tenancy"), p.boolean("no-interactive"))
	if err != nil {
		return nil, err
	}

	clui.Infof("Scaffolding %s plugin %q (tier: %s, tenancy: %s)...", tmplType, name, tier, tenancy)

	result, err := scaffold.Run(scaffold.Options{
		Name:        name,
		Tier:        tier,
		Bundle:      p.str("bundle"),
		Description: p.str("description"),
		Author:      p.str("author"),
		Category:    p.str("category"),
		Language:    tmplType,
		MinCLI:      p.str("min-cli"),
		MinSDK:      p.str("min-sdk"),
		Port:        p.integer("port"),
		OutDir:      p.str("out"),
		Force:       p.boolean("force"),
		Tenancy:     tenancy,
	})
	if err != nil {
		return nil, scaffoldErr(fmt.Errorf("scaffold failed: %w", err))
	}

	clui.Successf("Plugin %q scaffolded in %s/", name, result.Dir)
	clui.Dimmedf("Files created: %s", strings.Join(result.Files, ", "))
	clui.Dimmed("")
	clui.Dimmed("Next steps:")
	clui.Dimmedf("  cd %s", result.Dir)
	switch tmplType {
	case "go":
		clui.Dimmed("  go mod tidy && go test ./...")
		clui.Dimmedf("  nself plugin link %s    # link for development", result.Dir)
		clui.Dimmedf("  nself plugin dev %s     # start hot-reload watcher", name)
	case "rust":
		clui.Dimmed("  cargo build")
	case "node":
		clui.Dimmed("  pnpm install && pnpm build")
	case "static":
		clui.Dimmed("  # Edit static/ directory with your content")
	}
	files := append([]string{}, result.Files...)
	sort.Strings(files)
	return initData{Name: name, Template: tmplType, Tier: tier, Tenancy: string(tenancy), Dir: result.Dir, Files: files}, nil
}

// scaffoldErr gives a scaffold.Run failure its envelope code: the validation and
// destination errors are the caller's (user), anything else is a render or write
// failure (infra). The envelope text is fixed; the human text is err.
func scaffoldErr(err error) error {
	text := err.Error()
	switch {
	case strings.Contains(text, "is not empty"):
		return coded(err, CodeDestNotEmpty, output.ClassUser, "the destination directory is not empty",
			"pick another --out or pass --force to overwrite")
	case strings.Contains(text, "invalid plugin name"):
		return usageErr(err, "invalid plugin name")
	case strings.Contains(text, "--tier must be"):
		return usageErr(err, "--tier must be free or pro")
	case strings.Contains(text, "--tenancy must be"):
		return usageErr(err, "--tenancy must be none, app-isolation, cloud-tenant, or both")
	}
	return coded(err, CodeScaffoldFailed, output.ClassInfra, "the scaffold could not render or write the plugin files",
		"check that the output directory is writable")
}

// resolveTenancy returns the tenancy mode: explicit flag, else the prompt unless
// --no-interactive or stdin is not a terminal, else none.
func resolveTenancy(flagVal string, noInteractive bool) (scaffold.TenancyMode, error) {
	if flagVal != "" {
		switch scaffold.TenancyMode(flagVal) {
		case scaffold.TenancyNone, scaffold.TenancyAppIsolation, scaffold.TenancyCloudTenant, scaffold.TenancyBoth:
			return scaffold.TenancyMode(flagVal), nil
		default:
			return "", usageErr(fmt.Errorf("--tenancy must be one of: none, app-isolation, cloud-tenant, both; got %q", flagVal),
				"--tenancy must be one of: none, app-isolation, cloud-tenant, both")
		}
	}
	if noInteractive || !stdinIsTerminal() {
		return scaffold.TenancyNone, nil
	}
	return promptTenancy()
}

// stdinIsTerminal reports whether stdin is a character device (core's test).
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// promptTenancy runs the interactive multi-tenancy selection on stdin.
func promptTenancy() (scaffold.TenancyMode, error) {
	r := bufio.NewReader(os.Stdin)

	fmt.Fprint(os.Stderr, "? Does this plugin store per-user or per-app data in Postgres tables? (y/N) ")
	ans, err := r.ReadString('\n')
	if err != nil {
		return scaffold.TenancyNone, usageErr(fmt.Errorf("reading input: %w", err),
			"could not read the tenancy answer: pass --tenancy or --no-interactive")
	}
	ans = strings.TrimSpace(strings.ToLower(ans))
	if ans != "y" && ans != "yes" {
		return scaffold.TenancyNone, nil
	}

	fmt.Fprintln(os.Stderr, "? What kind of isolation do you need?")
	fmt.Fprintln(os.Stderr, "  [a] Per-app isolation within one nSelf deploy  (source_account_id TEXT NOT NULL DEFAULT 'primary')")
	fmt.Fprintln(os.Stderr, "  [b] Cloud multi-tenant (separate paying customers)  (tenant_id UUID + Hasura row filter)")
	fmt.Fprintln(os.Stderr, "  [c] Not sure — add both, I'll decide later")
	fmt.Fprintln(os.Stderr, "  [d] Neither — skip tenancy scaffold")
	fmt.Fprint(os.Stderr, "Choice [a/b/c/d]: ")

	choice, err := r.ReadString('\n')
	if err != nil {
		return scaffold.TenancyNone, usageErr(fmt.Errorf("reading input: %w", err),
			"could not read the tenancy answer: pass --tenancy or --no-interactive")
	}
	choice = strings.TrimSpace(strings.ToLower(choice))
	switch choice {
	case "a":
		return scaffold.TenancyAppIsolation, nil
	case "b":
		return scaffold.TenancyCloudTenant, nil
	case "c":
		return scaffold.TenancyBoth, nil
	default:
		return scaffold.TenancyNone, nil
	}
}
