package watch

import "github.com/nself-org/plugins/free/ci/internal/model"

// The codes of this Ticket's share of the AUTO block (model.CodeRanges: E804-E805, and the spare E821
// for the isolated-runner skip, because E625 is P7-CI-37's).
const (
	CodeConfig   = "E804"
	CodeQuota    = "E805"
	CodeIsolated = "E821"
)

func init() {
	for _, code := range []model.Code{
		{ID: CodeConfig, Class: "usage", Summary: "a watch.* setting is invalid", Fix: "Fix the operator value (`nself ci policy show` names the key) and restart the watcher"},
		{ID: CodeQuota, Class: "quota", Summary: "the GitHub quota stopped the watch pass", Fix: "Nothing to do: the watcher makes no request before the recorded reset time; raise providers.github.poll_interval if it recurs"},
		{ID: CodeIsolated, Class: "policy", Summary: "skipped: needs isolated runner", Fix: "A collaborator head runs only on an isolated runner; add one (the isolation section of `nself ci doctor` shows what this machine proved) or let the coordinator run it"},
	} {
		model.Register(code)
	}
}

// Error carries one of the registered watch codes. Its text never holds a token or a path.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }
