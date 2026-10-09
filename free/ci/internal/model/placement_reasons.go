package model

// PlacementReason is a scheduler reason with a stable public code.
type PlacementReason string

const (
	NodeRevoked                 PlacementReason = "node.revoked"
	NodeDiscovered              PlacementReason = "node.discovered"
	NodeDeployHost              PlacementReason = "node.deploy_host"
	NodeProtocol                PlacementReason = "node.protocol"
	NodeOffline                 PlacementReason = "node.offline"
	NodeDraining                PlacementReason = "node.draining"
	NodeMaintenance             PlacementReason = "node.maintenance"
	NodeOnBattery               PlacementReason = "node.on_battery"
	NodeInteractive             PlacementReason = "node.interactive"
	NodeReservation             PlacementReason = "node.reservation"
	PlatformMismatch            PlacementReason = "platform.mismatch"
	ToolsMissing                PlacementReason = "tools.missing"
	ResourcesInsufficient       PlacementReason = "resources.insufficient"
	LabelsMismatch              PlacementReason = "labels.mismatch"
	CapacityBusy                PlacementReason = "capacity.busy"
	RetryAntiAffinity           PlacementReason = "retry.anti_affinity"
	OverrideExcluded            PlacementReason = "override.excluded"
	OverrideMinIsolation        PlacementReason = "override.min_isolation"
	OverrideMaxCost             PlacementReason = "override.max_cost"
	OverrideMaxQueue            PlacementReason = "override.max_queue"
	CostUnknownRate             PlacementReason = "cost.unknown_rate"
	CostUnbounded               PlacementReason = "cost.unbounded"
	PaidDisabled                PlacementReason = "paid.disabled"
	BudgetUnset                 PlacementReason = "budget.unset"
	ProviderOwnedElsewhere      PlacementReason = "provider.owned_elsewhere"
	BurstTeamOnly               PlacementReason = "burst.team_only"
	AllowanceUnknown            PlacementReason = "allowance.unknown"
	AllowanceBelowMargin        PlacementReason = "allowance.below_margin"
	BudgetExceeded              PlacementReason = "budget.exceeded"
	ApprovalPending             PlacementReason = "approval.pending"
	ProviderDown                PlacementReason = "provider.down"
	RunnerRemoteOnly            PlacementReason = "runner.remote_only"
	LimitPipeline               PlacementReason = "limit.pipeline"
	LimitJob                    PlacementReason = "limit.job"
	LimitRunner                 PlacementReason = "limit.runner"
	LimitProvider               PlacementReason = "limit.provider"
	LimitProject                PlacementReason = "limit.project"
	LimitUser                   PlacementReason = "limit.user"
	LimitTeam                   PlacementReason = "limit.team"
	LimitGlobal                 PlacementReason = "limit.global"
	BackgroundInteractiveQueued PlacementReason = "background.interactive_queued"
	BackgroundContention        PlacementReason = "background.contention"
	EphemeralUsed               PlacementReason = "ephemeral.used"
	PlatformNoneAvailable       PlacementReason = "platform.none_available"
	QueueTimeout                PlacementReason = "queue.timeout"
	BackpressureQueueFull       PlacementReason = "backpressure.queue_full"
)

var placementReasons = map[PlacementReason]bool{
	NodeRevoked: true, NodeDiscovered: true, NodeDeployHost: true, NodeProtocol: true, NodeOffline: true, NodeDraining: true, NodeMaintenance: true, NodeOnBattery: true, NodeInteractive: true, NodeReservation: true,
	PlatformMismatch: true, ToolsMissing: true, ResourcesInsufficient: true, LabelsMismatch: true, CapacityBusy: true, RetryAntiAffinity: true,
	OverrideExcluded: true, OverrideMinIsolation: true, OverrideMaxCost: true, OverrideMaxQueue: true,
	CostUnknownRate: true, CostUnbounded: true, PaidDisabled: true, BudgetUnset: true, ProviderOwnedElsewhere: true, BurstTeamOnly: true, AllowanceUnknown: true, AllowanceBelowMargin: true, BudgetExceeded: true, ApprovalPending: true,
	ProviderDown: true, RunnerRemoteOnly: true, LimitPipeline: true, LimitJob: true, LimitRunner: true, LimitProvider: true, LimitProject: true, LimitUser: true, LimitTeam: true, LimitGlobal: true, BackgroundInteractiveQueued: true, BackgroundContention: true, EphemeralUsed: true,
	PlatformNoneAvailable: true, QueueTimeout: true, BackpressureQueueFull: true,
}

func (r PlacementReason) Valid() bool { return placementReasons[r] }

// Static reports whether a runner cannot become eligible without a configuration change.
func (r PlacementReason) Static() bool {
	switch r {
	case NodeRevoked, NodeDiscovered, NodeDeployHost, NodeProtocol, PlatformMismatch, ToolsMissing, ResourcesInsufficient, LabelsMismatch, OverrideExcluded, OverrideMinIsolation, OverrideMaxCost, CostUnknownRate, CostUnbounded, PaidDisabled, BudgetUnset, ProviderOwnedElsewhere, BurstTeamOnly, AllowanceUnknown, RunnerRemoteOnly:
		return true
	}
	return false
}
