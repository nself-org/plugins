package exec

import "github.com/nself-org/plugins/free/ci/internal/model"

// Mode distinguishes operator work from reserved background and remote work.
type Mode string

const (
	Foreground Mode = "foreground"
	Background Mode = "background"
	Remote     Mode = "remote"
)

// AdmissionPolicy controls host reservations. Zero values select defaults.
type AdmissionPolicy struct {
	ReserveCPUs          int
	ReserveMemoryPercent int
	BatteryThreshold     int
	Force                bool
}

// Decision is a capacity verdict that a coordinator commits through store.AdmitAndLease.
type Decision struct {
	Allowed                                             bool
	Slots                                               int
	Reason                                              model.Reason
	Capacity                                            Capacity
	ReserveCPUs, ReserveMemoryPercent, BatteryThreshold int
	ReservationsDisabled                                bool
}

// Admit evaluates a snapshot without reserving capacity itself.
func Admit(job model.Job, mode Mode, capacity Capacity, policy AdmissionPolicy) Decision {
	if policy.ReserveCPUs <= 0 {
		policy.ReserveCPUs = 2
	}
	if policy.ReserveMemoryPercent <= 0 {
		policy.ReserveMemoryPercent = 25
	}
	if policy.BatteryThreshold <= 0 {
		policy.BatteryThreshold = 30
	}
	d := Decision{Capacity: capacity, ReserveCPUs: policy.ReserveCPUs, ReserveMemoryPercent: policy.ReserveMemoryPercent, BatteryThreshold: policy.BatteryThreshold}
	d.ReservationsDisabled = capacity.CI || !capacity.Interactive || mode == Foreground || policy.Force
	if d.ReservationsDisabled {
		d.ReserveCPUs = 0
		d.ReserveMemoryPercent = 0
	}
	if mode != Foreground && mode != Background && mode != Remote {
		d.Reason = "admission.reservation"
		return d
	}
	if mode != Foreground && !policy.Force && !capacity.CI && capacity.Interactive && job.Path == "deep" && (capacity.LowPower || (!capacity.PluggedIn && capacity.BatteryPercent >= 0 && capacity.BatteryPercent < policy.BatteryThreshold)) {
		d.Reason = "admission.battery"
		return d
	}
	available := capacity.CPUs - d.ReserveCPUs
	if mode == Foreground && available < 1 {
		available = 1
	}
	if mode == Foreground && capacity.CPUs <= 2 && available > 1 {
		available = 1
	}
	if available < 1 {
		d.Reason = "admission.reservation"
		return d
	}
	freeMB := capacity.FreeMemoryMB
	if freeMB <= 0 {
		freeMB = capacity.MemoryMB
	}
	freeMB -= capacity.MemoryMB * d.ReserveMemoryPercent / 100
	if freeMB < 256 && mode != Foreground {
		d.Reason = "admission.reservation"
		return d
	}
	need := 1
	if job.Requirements != nil && job.Requirements.CPU > need {
		need = job.Requirements.CPU
	}
	if need > available {
		d.Reason = "admission.reservation"
		return d
	}
	d.Slots = available / need
	if job.Requirements != nil && job.Requirements.MemMB > 0 && mode != Foreground {
		memorySlots := freeMB / job.Requirements.MemMB
		if memorySlots < d.Slots {
			d.Slots = memorySlots
		}
	}
	if d.Slots < 1 {
		d.Reason = "admission.reservation"
		return d
	}
	d.Allowed = true
	return d
}
