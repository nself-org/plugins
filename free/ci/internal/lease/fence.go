package lease

import (
	"sync"
	"time"
)

// AgentFence uses both clocks so suspend and wall jumps cannot extend a lease.
type AgentFence struct {
	mu         sync.Mutex
	Wall       func() time.Time
	Mono       func() time.Duration
	TTL        time.Duration
	OnFence    func()
	lastWall   time.Time
	lastMono   time.Duration
	selfFenced bool
}

// NewAgentFence sets the self-fence deadline two heartbeat intervals before expiry.
func NewAgentFence(wall func() time.Time, mono func() time.Duration, coordinatorTTL, heartbeat time.Duration) *AgentFence {
	if wall == nil {
		wall = time.Now
	}
	if mono == nil {
		start := time.Now()
		mono = func() time.Duration { return time.Since(start) }
	}
	f := &AgentFence{Wall: wall, Mono: mono, TTL: coordinatorTTL - 2*heartbeat}
	if f.TTL <= 0 {
		f.TTL = time.Nanosecond
	}
	f.Ack()
	return f
}

// Ack records a coordinator heartbeat acknowledgement.
func (f *AgentFence) Ack() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.selfFenced {
		return
	}
	f.lastWall, f.lastMono = f.Wall(), f.Mono()
}

// Check revalidates on every wake and latches a self-fence before job work resumes.
func (f *AgentFence) Check() bool {
	f.mu.Lock()
	wallDelta := f.Wall().Sub(f.lastWall)
	monoDelta := f.Mono() - f.lastMono
	first := !f.selfFenced && (wallDelta >= f.TTL || monoDelta >= f.TTL)
	if first {
		f.selfFenced = true
	}
	fenced, callback := f.selfFenced, f.OnFence
	f.mu.Unlock()
	if first && callback != nil {
		callback()
	}
	return fenced
}
