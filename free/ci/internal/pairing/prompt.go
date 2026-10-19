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
	mu  sync.Mutex
}

// Kind is "admin".
func (p *PromptApprover) Kind() string { return "admin" }

func (p *PromptApprover) ask(q string) (string, error) {
	fmt.Fprint(p.Out, q)
	line, err := p.In.ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		return "", err
	}
	return strings.TrimSpace(line), nil
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
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.Out, "\nA node asks to join.\n  name:            %s\n  os/arch:         %s/%s\n  from:            %s\n  CSR key:         %s\n  node key id:     %s\n  certificate:     %s\n",
		r.NodeName, r.OS, r.Arch, r.SourceAddr, r.CSRFingerprint, r.NodeKeyID, r.CertTTL)
	if r.Label != "" {
		fmt.Fprintf(p.Out, "  label:           %s\n", r.Label)
	}
	got, err := p.ask("Compare with what the node printed. Type the CSR key fingerprint to approve, or press Enter to deny: ")
	if err != nil {
		return Decision{}, err
	}
	if got == "" || !pki.Equal(got, r.CSRFingerprint) {
		return Decision{Reason: "fingerprint not confirmed by the admin"}, nil
	}
	def := DefaultTrust()
	accepts, err := p.ask(fmt.Sprintf("Trust classes the node accepts [%s]: ", strings.Join(def.Accepts, ",")))
	if err != nil {
		return Decision{}, err
	}
	secrets, err := p.ask(fmt.Sprintf("Secret classes the node may hold [%s]: ", strings.Join(def.SecretClasses, ",")))
	if err != nil {
		return Decision{}, err
	}
	zone, err := p.ask("Privacy zone []: ")
	if err != nil {
		return Decision{}, err
	}
	projects, err := p.ask("Projects to authorize (comma list) []: ")
	if err != nil {
		return Decision{}, err
	}
	return Decision{Approve: true, Reason: "approved by the admin", Trust: TrustFields{
		Accepts: list(accepts, def.Accepts), SecretClasses: list(secrets, def.SecretClasses), PrivacyZone: zone, Projects: list(projects, nil),
	}}, nil
}
