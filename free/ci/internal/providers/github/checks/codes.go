package checks

import "github.com/nself-org/plugins/free/ci/internal/model"

// E816 is this Ticket's share of the AUTO block (model.CodeRanges).
func init() {
	model.Register(model.Code{ID: "E816", Class: "infra", Summary: "the GitHub App check run was not published (the status was)", Fix: "The commit status is the result of record and was posted; only the optional `nself-ci/gates` check run failed. Check providers.github.app.id, providers.github.app.installation_id and the PEM in the variable named by providers.github.app.key_env, and that the App has checks:write on the repository"})
}

// Error carries E816. Its text never holds a key, a JWT or an installation token.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func coded(message string) error { return &Error{Code: "E816", Message: message} }
