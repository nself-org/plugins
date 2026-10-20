package sched

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

type Pipeline struct {
	ID         string
	Support    []string
	Band       string
	Foreground bool
	Release    bool
	// Record marks a commit-CI run of record; false for dev runs (P7-AUTO-27).
	Record bool `json:",omitempty"`
}
type Job struct {
	Key            string
	ID             string
	Name           string
	MatrixKey      string
	Deps           []string
	Pipeline       Pipeline
	Requirements   model.PlacementRequirements
	Accelerators   []string
	Trust          model.TrustClass
	SecretClasses  []model.SecretClass
	Network        model.NetworkScope
	TimeoutMs      int64
	Attempt        int
	Priority       string
	RemoteOnly     bool
	PersonalOwnRun bool
	Project        string
	User           string
	Team           string
	// QueuedMs is how long the job has waited, in ms; dispatch fills it (P7-AUTO-27).
	QueuedMs int64 `json:",omitempty"`
}
type Avail struct {
	Slots int
	CPU   int64
	MemMB int64
}
type Runner struct {
	ID              string
	Capability      model.Capability
	Decl            trust.Declaration
	Class           string
	Location        string
	Provider        string
	Ephemeral       bool
	Used            bool
	Avail           Avail
	CoordinatorHost bool
	CoordinatorMode trust.CoordinatorMode
	ProviderDown    bool
	QueuedAhead     int64
	Active          bool
}
type Sample struct {
	WaitMs               *int64
	RuntimeMs            *int64
	ColdStartMs          *int64
	TransferMs           *int64
	CacheMs              *int64
	ContentionMs         *int64
	ScarcityMs           *int64
	ReliabilityMs        *int64
	InfraFailurePermille *int64
	CostMicro            *int64
	CheckoutBytes        *int64
	LocalCPU             *int64
	Confidence           string
}
type Cost struct {
	Class              string
	RateMicro          int64
	BillingIncrementMs int64
	// DivMs makes RateMicro a price per DivMs milliseconds (0 or 1: per millisecond), so a
	// declared hourly or per-minute rate keeps its precision instead of rounding up per millisecond.
	DivMs              int64 `json:",omitempty"`
	MinimumChargeMicro int64
	ExtrasMicro        int64
	Confidence         string
	OwnerCoordinator   string
}
type Allowance struct {
	RemainingEstimated int64
	MarginMicro        int64
	Known              bool
	CeilingKnown       bool
}
type Budget struct {
	Remaining int64
	Set       bool
}
type Budgets struct {
	Job, Pipeline, Day, Month Budget
	Providers                 map[string]Budget
}
type CacheFact struct {
	MissBytes  *int64
	Confidence string
}
type Pending struct {
	Provider string
	Platform string
	Slots    int
}
type Limits struct {
	Pipeline, Job, Runner, Provider, Project, User, Team, Global int
}
type Counts struct {
	Pipeline, Job, Project, User, Team, Global int
	Runner, Provider                           int // Legacy aggregate snapshot counts; scoped counts below control admission.
	Runners                                    map[string]int
	Providers                                  map[string]int
}
type Overrides struct {
	LocalOnly, LANOnly, PrivateOnly bool
	ProvidersAllow, ProvidersDeny   []string
	Runner, OS, Arch                string
	Labels                          []string
	MaxCostMicro                    int64
	MaxQueueMs                      int64
	MinIsolation                    model.Isolation
}
type Policy struct {
	Preset                 string
	Trust                  trust.Effective
	PaidEnabled            bool
	CoordinatorID          string
	MaxPaidTimeoutMs       int64
	ApprovalThresholdMicro int64
	FastPathMarginMs       int64
	Overrides              Overrides
	Limits                 Limits
}
type World struct {
	NowMs             int64
	Runners           []Runner
	Hosted            []Runner
	Policy            Policy
	Estimates         map[string]map[string]Sample
	Costs             map[string]Cost
	Allowances        map[string]Allowance
	Budgets           Budgets
	Pending           []Pending
	Cache             map[string]map[string]CacheFact
	Previous          map[string]string
	PreviousIsolation map[string]model.Isolation
	Approvals         map[string]bool
	Counts            Counts
	OpenRegistry      bool
	InteractiveQueued map[string]bool
	// Exclusions are extra per-runner reasons appended to eligibility() output. The
	// only contributor is internal/autoworld (P7-AUTO-27); empty means no effect.
	Exclusions map[string][]model.PlacementReasonDetail `json:",omitempty"`
	// Bias is a per-runner score addition in ms, shown as the tier_rank component.
	// Its contributor is P7-AUTO-05; empty means no effect.
	Bias map[string]int64 `json:",omitempty"`
	// Network holds, per runner id, the scopes a probe proved the runner enforces (network.enforced_scopes).
	// Its contributor is P7-TRUST-08; a runner without an entry has no proof, so its NoLANRoute stays false.
	Network map[string]NetworkFact `json:",omitempty"`
}

// NetworkFact is what a runner proved about its network scopes.
type NetworkFact struct {
	Enforced []model.NetworkScope `json:",omitempty"`
}
type Placement struct {
	JobKey     string
	RunnerID   string
	CostMicro  int64
	CPU, MemMB int64
}
type Explanation struct {
	JobKey       string                        `json:"job_key"`
	Status       string                        `json:"status"`
	Reasons      []model.PlacementReasonDetail `json:"reasons"`
	Alternatives []model.Alternative           `json:"alternatives"`
}
type Result struct {
	Placements   []Placement    `json:"placements"`
	Explanations []Explanation  `json:"explanations"`
	FastPath     model.FastPath `json:"fast_path"`
	ErrorCode    string         `json:"error_code,omitempty"`
}

// Apply reserves a placement against this world snapshot after admission.
func (w *World) Apply(p Placement) {
	for i := range w.Runners {
		if w.Runners[i].ID == p.RunnerID {
			w.reservePlacement(&w.Runners[i], p)
			return
		}
	}
	for i := range w.Hosted {
		if w.Hosted[i].ID == p.RunnerID {
			w.reservePlacement(&w.Hosted[i], p)
			return
		}
	}
}

func (w *World) reservePlacement(r *Runner, p Placement) {
	reserve(r, p)
	w.Counts.Pipeline++
	w.Counts.Job++
	w.Counts.Runner++
	w.Counts.Provider++
	if w.Counts.Runners == nil {
		w.Counts.Runners = make(map[string]int)
	}
	if w.Counts.Providers == nil {
		w.Counts.Providers = make(map[string]int)
	}
	w.Counts.Runners[r.ID]++
	w.Counts.Providers[r.Provider]++
	w.Counts.Project++
	w.Counts.User++
	w.Counts.Team++
	w.Counts.Global++

	class := runnerClass(*w, *r)
	if class == "included" && !labelMatches(r.Capability.Identity.Labels, "nself.visibility=public") {
		if a, ok := w.Allowances[r.Provider]; ok {
			a.RemainingEstimated, _ = Add(a.RemainingEstimated, -p.CostMicro)
			w.Allowances[r.Provider] = a
		}
	}
	if class != "local" && class != "owned" && class != "included" {
		debitBudget(&w.Budgets.Job, p.CostMicro)
		debitBudget(&w.Budgets.Pipeline, p.CostMicro)
		debitBudget(&w.Budgets.Day, p.CostMicro)
		debitBudget(&w.Budgets.Month, p.CostMicro)
		if b, ok := w.Budgets.Providers[r.Provider]; ok {
			debitBudget(&b, p.CostMicro)
			w.Budgets.Providers[r.Provider] = b
		}
	}
}

func debitBudget(b *Budget, cost int64) {
	if b.Set {
		b.Remaining, _ = Add(b.Remaining, -cost)
	}
}
func reserve(r *Runner, p Placement) {
	r.Avail.Slots--
	r.Avail.CPU -= p.CPU
	r.Avail.MemMB -= p.MemMB
	if r.Ephemeral {
		r.Used = true
	}
}
