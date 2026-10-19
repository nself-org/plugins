package secrets

// Purpose: change who can decrypt one store file by calling `nself config secrets recipients` (the only
// fabric caller of that command, contract:cli.secret-resolve companion).
// Inputs:  the project root, the store file, the node's age recipient and the verb.
// Outputs: nil, or E676 when the cli refused or could not run.
// Constraints: the call runs as the operator, never on the node, from an operator-trusted checkout with
// an absolute --project; release and deploy files add --allow-release-class (the caller has already
// checked the node is dedicated-trusted). A recipient is a public key, but it is still validated here
// so it cannot become a flag. Removal does not revoke plaintext a node already read: rotate afterwards.

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var recipientPattern = regexp.MustCompile(`^age1[a-z0-9]{50,100}$`)

// ValidRecipient reports whether s looks like an age public key.
func ValidRecipient(s string) bool { return recipientPattern.MatchString(s) }

// Recipients runs `config secrets recipients <add|remove> --file <file> --project <root> [--allow-release-class] <recipient>`.
func Recipients(ctx context.Context, r Runner, binary, project, verb, file, recipient string, class model.SecretClass) error {
	if verb != "add" && verb != "remove" {
		return errRef("unknown recipients verb")
	}
	if !ValidRecipient(recipient) {
		return errRef("the node's age recipient is not a valid age public key")
	}
	if binary == "" {
		binary = "nself"
	}
	argv := []string{binary, "config", "secrets", "recipients", verb, "--file", file, "--project", project}
	if class == "release" || class == "deploy" {
		argv = append(argv, "--allow-release-class")
	}
	argv = append(argv, recipient)
	out, err := r.Run(ctx, Call{Argv: argv, Timeout: CallTimeout})
	defer zero(out.Bytes)
	if err != nil {
		return errResolve("the nself cli could not be run")
	}
	if out.ExitCode != 0 {
		code := "E???"
		var env envelope
		if i := bytes.IndexByte(out.Bytes, '{'); i >= 0 {
			_ = json.NewDecoder(bytes.NewReader(out.Bytes[i:])).Decode(&env)
		}
		if env.Error != nil {
			code = safeCode(env.Error.Code)
		}
		e := newErr("E676", "the nself cli refused `config secrets recipients %s` (%s)", verb, code)
		e.Cause = code
		return e
	}
	return nil
}
