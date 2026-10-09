package protocol

import "github.com/nself-org/plugins/free/ci/internal/model"

const (
	CodeVersion = "E652"
	CodeInvalid = "E653"
)

func init() {
	model.Register(model.Code{ID: CodeVersion, Class: "usage", Summary: "runner protocol version incompatible", Fix: "Upgrade the agent to min_agent_version"})
	model.Register(model.Code{ID: CodeInvalid, Class: "infra", Summary: "runner protocol message invalid", Fix: "Check the carrier and agent protocol stream"})
}
