package pairing

// Purpose: the default Approver, the interactive admin prompt of `nodes pair`.
// Inputs:  the admin's input stream and the output stream of the pairing process.
// Outputs: a Decision. The admin approves by typing the exact CSR fingerprint the node printed, then
//          sets the trust fields (Enter keeps the default shown in brackets).
// Constraints: fingerprint typed wrong or empty is a denial (nothing issued); the prompt reads whole
//          lines from the injected reader, so scripted tests need no terminal; one prompt at a time
//          (the lock), so two simultaneous redemptions cannot interleave their questions.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// PromptApprover asks the admin on In/Out.
type PromptApprover struct {
	In  *bufio.Reader
	Out io.Writer
	// turn admits one redemption at a time; it is a channel so a waiting redemption can give up with ctx.
	once sync.Once
	turn chan struct{}
}

// Kind is "admin".
func (p *PromptApprover) Kind() string { return "admin" }

// ask prints q and reads one line; it returns early with ctx.Err() when ctx ends (the blocked read keeps
// running in its goroutine until the stream yields a line or closes, which only ends with the process).
func (p *PromptApprover) ask(ctx context.Context, q string) (string, error) {
	fmt.Fprint(p.Out, q)
	type answer struct {
		line string
		err  error
	}
	ch := make(chan answer, 1)
	go func() {
		line, err := p.In.ReadString('\n')
		ch <- answer{line, err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case a := <-ch:
		if a.err != nil && (a.err != io.EOF || a.line == "") {
			return "", a.err
		}
		return strings.TrimSpace(a.line), nil
	}
}

// printable drops control characters and cuts s, so a node cannot send terminal escape sequences to the
// admin through its name, os, arch or the label.
func printable(s string) string {
	return cut(s, 80)
}

func list(v string, def []string) []string {
	if v == "" {
		return def
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// Approve shows the node, requires the CSR fingerprint, then asks for the trust fields.
func (p *PromptApprover) Approve(ctx context.Context, r ApprovalRequest) (Decision, error) {
	p.once.Do(func() { p.turn = make(chan struct{}, 1) })
	select {
	case p.turn <- struct{}{}:
		defer func() { <-p.turn }()
	case <-ctx.Done():
		return Decision{}, ctx.Err()
	}
	fmt.Fprintf(p.Out, "\nA node asks to join.\n  name:            %s\n  os/arch:         %s/%s\n  from:            %s\n  CSR key:         %s\n  node key id:     %s\n  certificate:     %s\n",
		printable(r.NodeName), printable(r.OS), printable(r.Arch), printable(r.SourceAddr), r.CSRFingerprint, r.NodeKeyID, r.CertTTL)
	if r.Label != "" {
		fmt.Fprintf(p.Out, "  label:           %s\n", printable(r.Label))
	}
	got, err := p.ask(ctx, "Compare with what the node printed. Type the CSR key fingerprint to approve, or press Enter to deny: ")
	if err != nil {
		return Decision{}, err
	}
	if got == "" || !pki.Equal(got, r.CSRFingerprint) {
		return Decision{Reason: "fingerprint not confirmed by the admin"}, nil
	}
	if r.Existing {
		fmt.Fprintln(p.Out, "This node is already registered: it keeps its trust fields and its pinned node key.")
		return Decision{Approve: true, Reason: "approved by the admin (existing node)"}, nil
	}
	def := DefaultTrust()
	accepts, err := p.ask(ctx, fmt.Sprintf("Trust classes the node accepts [%s]: ", strings.Join(def.Accepts, ",")))
	if err != nil {
		return Decision{}, err
	}
	secrets, err := p.ask(ctx, fmt.Sprintf("Secret classes the node may hold [%s]: ", strings.Join(def.SecretClasses, ",")))
	if err != nil {
		return Decision{}, err
	}
	zone, err := p.ask(ctx, "Privacy zone []: ")
	if err != nil {
		return Decision{}, err
	}
	projects, err := p.ask(ctx, "Projects to authorize (comma list) []: ")
	if err != nil {
		return Decision{}, err
	}
	return Decision{Approve: true, Reason: "approved by the admin", Trust: TrustFields{
		Accepts: list(accepts, def.Accepts), SecretClasses: list(secrets, def.SecretClasses), PrivacyZone: zone, Projects: list(projects, nil),
	}}, nil
}
