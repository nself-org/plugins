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
type Counts struct{ Pipeline, Job, Runner, Provider, Project, User, Team, Global int }
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
			reserve(&w.Runners[i], p)
			return
		}
	}
	for i := range w.Hosted {
		if w.Hosted[i].ID == p.RunnerID {
			reserve(&w.Hosted[i], p)
			return
		}
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
