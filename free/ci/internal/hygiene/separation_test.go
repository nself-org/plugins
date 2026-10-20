package hygiene

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

const (
	agentUID = 1001
	coordUID = 1002
)

func baseFacts() Facts {
	return Facts{Isolation: "process", Trust: "internal", Coordinator: trust.Team, Ownership: "operator", Slots: 4, AgentUID: agentUID, CoordinatorUID: coordUID}
}

func TestSeparationModes(t *testing.T) {
	system := baseFacts()
	system.JobShim, system.JobUIDs = true, []int{2000, 2001, 2002, 2003}
	userns := baseFacts()
	userns.Userns, userns.SubUIDStart, userns.SubUIDCount = true, 100000, 65536
	cont := baseFacts()
	cont.Isolation = "container"
	cases := []struct {
		name string
		f    Facts
		slot int
		mode Mode
		uid  int
		user string
	}{
		{"system install", system, 2, JobShim, 2002, "nself-ci-job-3"},
		{"userns", userns, 3, Userns, 100003, ""},
		{"container", cont, 0, ContainerMapped, 65532, ""},
		{"container skips the agent uid", withAgent(cont, 65532), 0, ContainerMapped, 65533, ""},
		{"sandboxed container", withLevel(cont, "sandboxed-container"), 1, ContainerMapped, 65533, ""},
	}
	for _, c := range cases {
		p, err := Separation(c.f, c.slot)
		if err != nil || p.Mode != c.mode || p.UID != c.uid || p.User != c.user || p.Serial {
			t.Errorf("%s: %+v %v", c.name, p, err)
		}
	}
}

func withAgent(f Facts, uid int) Facts  { f.AgentUID = uid; return f }
func withLevel(f Facts, l string) Facts { f.Isolation = model.Isolation(l); return f }

func TestSeparationRefusesTeamWithoutShim(t *testing.T) {
	f := baseFacts()
	f.JobShim = false
	_, err := Separation(f, 0)
	if codeOf(err) != CodeNoJobUID {
		t.Fatalf("want E682 for a TEAM lease without the shim rule, got %v", err)
	}
	f.JobShim, f.JobUIDs = true, []int{2000}
	f.Slots = 1
	if p, err := Separation(f, 0); err != nil || p.Mode != JobShim {
		t.Fatalf("with the rule: %+v %v", p, err)
	}
}

// TestSeparationOwnerExceptionTable runs the full product of the exception inputs without any
// separation and expects the exception for exactly one cell.
func TestSeparationOwnerExceptionTable(t *testing.T) {
	trusts := []model.TrustClass{"untrusted", "collaborator", "internal", "owner"}
	levels := []string{"process", "container"}
	modes := []trust.CoordinatorMode{trust.Personal, trust.Team}
	owners := []string{"operator", "other"}
	nodes := []bool{true, false}
	cells := 0
	for _, tr := range trusts {
		for _, lvl := range levels {
			for _, mode := range modes {
				for _, own := range owners {
					for _, node := range nodes {
						f := baseFacts()
						f.Trust, f.Isolation, f.Coordinator, f.Ownership, f.OperatorNode = tr, model.Isolation(lvl), mode, own, node
						p, err := Separation(f, 0)
						want := lvl == "process" && tr == "owner" && mode == trust.Personal && own == "operator" && node
						switch {
						case lvl == "container":
							if err != nil || p.Mode != ContainerMapped {
								t.Errorf("%v: container must map, got %+v %v", f, p, err)
							}
						case want:
							cells++
							if err != nil || p.Mode != OwnerException || p.UID != agentUID {
								t.Errorf("exception cell %+v: %+v %v", f, p, err)
							}
						default:
							if codeOf(err) != CodeNoJobUID {
								t.Errorf("must refuse %s/%s/%s/%s/%v, got %+v %v", tr, lvl, mode, own, node, p, err)
							}
						}
					}
				}
			}
		}
	}
	if cells != 1 {
		t.Fatalf("exception cells = %d", cells)
	}
}

func TestSeparationNeverSharesAUIDAcrossSlots(t *testing.T) {
	for _, mode := range []string{"job-shim", "userns", "container"} {
		for slots := 1; slots <= 8; slots++ {
			f := baseFacts()
			f.Slots = slots
			switch mode {
			case "job-shim":
				f.JobShim = true
				for i := 0; i < slots; i++ {
					f.JobUIDs = append(f.JobUIDs, 2000+i)
				}
			case "userns":
				f.Userns, f.SubUIDStart, f.SubUIDCount = true, 100000, 65536
			default:
				f.Isolation = "container"
			}
			seen := map[int]int{}
			for slot := 0; slot < slots; slot++ {
				p, err := Separation(f, slot)
				if err != nil {
					t.Fatalf("%s slots=%d slot=%d: %v", mode, slots, slot, err)
				}
				if prev, dup := seen[p.UID]; dup {
					t.Fatalf("%s: slots %d and %d share uid %d", mode, prev, slot, p.UID)
				}
				if p.UID == agentUID || p.UID == coordUID {
					t.Fatalf("%s: slot %d got a forbidden uid", mode, slot)
				}
				seen[p.UID] = slot
			}
		}
	}
}

func TestSeparationFewerJobUsersThanSlotsRunsOneAtATime(t *testing.T) {
	f := baseFacts()
	f.JobShim, f.JobUIDs, f.Slots = true, []int{2000}, 4
	p, err := Separation(f, 0)
	if err != nil || !p.Serial || p.Reason != trust.IsolationSlotUIDShared || p.UID != 2000 {
		t.Fatalf("slot 0: %+v %v", p, err)
	}
	if _, err := Separation(f, 1); codeOf(err) != CodeNoJobUID {
		t.Fatalf("slot 1 has no job user and must be refused, got %v", err)
	}
}

func TestSeparationRejectsBadProvisioning(t *testing.T) {
	f := baseFacts()
	f.JobShim, f.JobUIDs = true, []int{2000, agentUID, 2002, 2003}
	if _, err := Separation(f, 0); codeOf(err) != CodeNoJobUID {
		t.Errorf("job user equal to the agent uid accepted: %v", err)
	}
	f.JobUIDs = []int{2000, 2000, 2002, 2003}
	if _, err := Separation(f, 0); codeOf(err) != CodeNoJobUID {
		t.Errorf("duplicate job uids accepted: %v", err)
	}
	f = baseFacts()
	f.Userns, f.SubUIDStart, f.SubUIDCount = true, 100000, 2
	if _, err := Separation(f, 2); codeOf(err) != CodeNoJobUID {
		t.Errorf("slot beyond the subuid range accepted: %v", err)
	}
	if _, err := Separation(baseFacts(), 9); codeOf(err) != CodeNoJobUID {
		t.Errorf("slot beyond the node accepted: %v", err)
	}
	if _, err := Separation(baseFacts(), -1); codeOf(err) != CodeNoJobUID {
		t.Errorf("negative slot accepted: %v", err)
	}
}

func TestParseSubuid(t *testing.T) {
	in := "root:100000:65536\nci-agent:200000:65536\n1001:300000:1000\nbroken\nx:0:5\n"
	cases := []struct {
		user  string
		uid   int
		start int
		ok    bool
	}{{"ci-agent", 5, 200000, true}, {"nobody", 1001, 300000, true}, {"x", 7, 0, false}, {"zzz", 9, 0, false}}
	for _, c := range cases {
		start, count, ok := ParseSubuid(strings.NewReader(in), c.user, c.uid)
		if ok != c.ok || start != c.start || (ok && count == 0) {
			t.Errorf("%s: %d %d %v", c.user, start, count, ok)
		}
	}
	_ = fmt.Sprint
}

func TestSeparationRefusesUnknownAgentUID(t *testing.T) {
	f := baseFacts()
	f.AgentUID = 0
	f.Isolation = "container"
	if _, err := Separation(f, 0); codeOf(err) != CodeNoJobUID {
		t.Fatalf("an unset agent uid must not pass: %v", err)
	}
	f = baseFacts()
	f.CoordinatorUID = 0 // unset and not declared absent: refused, never guessed
	f.Isolation = "container"
	if _, err := Separation(f, 0); codeOf(err) != CodeNoJobUID {
		t.Fatalf("an unset coordinator uid must not pass: %v", err)
	}
	f.NoCoordinator = true
	f.AgentUID = 65532
	if p, err := Separation(f, 0); err != nil || p.UID == 65532 {
		t.Fatalf("agent uid handed out: %+v %v", p, err)
	}
}

func TestSeparationProcessUIDNeverSharesAContainerSlotUID(t *testing.T) {
	f := baseFacts()
	f.Slots = 4
	f.JobShim, f.JobUIDs = true, []int{2000, 65533, 2002, 2003}
	if _, err := Separation(f, 0); codeOf(err) != CodeNoJobUID {
		t.Fatalf("a job user inside the container slot range must be refused: %v", err)
	}
	f = baseFacts()
	f.Slots = 4
	f.Userns, f.SubUIDStart, f.SubUIDCount = true, 65530, 65536
	if _, err := Separation(f, 2); codeOf(err) != CodeNoJobUID {
		t.Fatalf("a subordinate uid inside the container slot range must be refused: %v", err)
	}
}

func TestUsernsPlanCarriesTheVerifiedWrapper(t *testing.T) {
	f := baseFacts()
	f.Userns, f.SubUIDStart, f.SubUIDCount = true, 100000, 65536
	p, err := Separation(f, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"unshare", "--user", "--map-users=0:100002:1", "--map-groups=0:100002:1", "--setuid=0", "--setgid=0", "--"}
	if strings.Join(p.Argv, " ") != strings.Join(want, " ") {
		t.Fatalf("wrapper %v", p.Argv)
	}
}
