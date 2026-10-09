package sched

import (
	"encoding/json"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestXcodeRequiresPlacement(t *testing.T) {
	type node struct {
		ID         string            `json:"id"`
		Platform   string            `json:"platform"`
		Toolchains map[string]string `json:"toolchains"`
	}
	root := "../model/testdata/placement/xcode"
	load := func(name string) Runner {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		var n node
		if err = json.Unmarshal(raw, &n); err != nil {
			t.Fatal(err)
		}
		r := fixtureRunner(n.ID, "lan")
		parts := strings.Split(n.Platform, "/")
		r.Capability.Platform.OS.Value = &parts[0]
		r.Capability.Platform.Arch.Value = &parts[1]
		r.Capability.Tools.Toolchains = map[string]model.Fact[string]{}
		for k, v := range n.Toolchains {
			value := v
			r.Capability.Tools.Toolchains[k] = model.Fact[string]{Value: &value}
		}
		return r
	}
	yes, no := load("with-xcode.json"), load("no-xcode.json")
	j := fixtureJob("xcode-test")
	j.Requirements.Platform = "darwin/arm64"
	j.Pipeline.Support = []string{"darwin/arm64"}
	j.Requirements.Tools = []string{"xcode"}
	got := Place(fixtureWorld(no, yes), []Job{j})
	if len(got.Placements) != 1 || got.Placements[0].RunnerID != yes.ID {
		t.Fatalf("xcode node: %+v", got)
	}
	got = Place(fixtureWorld(no), []Job{j})
	if got.Explanations[0].Status != "unplaceable" || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.ToolsMissing)) {
		t.Fatalf("missing xcode: %+v", got)
	}
}
