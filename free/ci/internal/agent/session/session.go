// Package session is the carrier-neutral agent session (contract:ci.runner-protocol v1).
//
// Purpose: the agent's side of one coordinator connection: hello, welcome,
// lease handling, the heartbeat, log and result emission, cancel and drain.
// Inputs:  a Carrier, the node facts (key, age recipient, slot table), a
// Runner that runs one job, and a Clock.
// Outputs: protocol frames on the carrier; <state>/coordinators.json on welcome.
// Constraints: one goroutine owns the lease table and every decision; job
// goroutines only send frames (serialised by a mutex) and report back on a
// channel. The slot table caps leases across every coordinator and carrier; a
// lease beyond it is refused with E654 (a log line plus a heartbeat naming the
// lease state "refused", because v1 has no agent-side reject frame). A stale
// epoch is ignored. Nothing here decides trust: a lease is run as the carrier
// delivered it, and a result is signed only by the registered Signer.
package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// minHeartbeat is the shortest heartbeat interval the agent will keep.
const minHeartbeat = time.Second

// NodeKey is the public half of the node signing key (nodekey.Key).
type NodeKey interface {
	KeyID() string
	PublicString() string
}

// Facts are the node's static answers sent in hello.
type Facts struct {
	AgentVersion        string
	MachineID           string
	HostKeyFingerprints []string
	CapabilityDigest    string
	CompatModes         []protocol.CompatMode
	// AgeRecipient is the node's age1... recipient; nil when it has none.
	AgeRecipient *string
}

// Status is what a heartbeat reports about the host.
type Status struct {
	// OnBattery is nil when the host has no battery.
	OnBattery   *bool
	Interactive bool
}

// Resume starts a session already past the handshake (carrier B reconnect, or a
// transcript replay): no hello is sent and Seq continues after SendSeq.
type Resume struct {
	SendSeq uint64
	Welcome protocol.Welcome
}

// Config wires a Session.
type Config struct {
	Carrier     Carrier
	Facts       Facts
	Key         NodeKey
	Slots       *slots.Table
	Runner      Runner
	Coordinator Coordinator
	// StateDir holds jobs/ and coordinators.json.
	StateDir string
	Clock    Clock
	// Hooks overrides the registered job hooks when non-nil.
	Hooks []JobHook
	// Signer overrides the one set with SetResultSigner when non-nil.
	Signer Signer
	// Status reports battery and interactivity for the heartbeat; nil reports neither.
	Status func() Status
	Resume *Resume
}

// RejectError is the coordinator's reject of hello.
type RejectError struct{ protocol.Reject }

func (e *RejectError) Error() string {
	return fmt.Sprintf("%s: coordinator rejected hello: %s (min_agent_version %s)", e.Code, e.Reason, e.MinAgentVersion)
}

// Session is one agent connection.
type Session struct {
	cfg Config

	sendMu  sync.Mutex
	sendSeq uint64

	// Owned by the Run goroutine.
	active     map[string]*activeLease
	seenEpoch  map[string]uint64
	seenOrder  []string
	draining   bool
	drainBy    time.Time
	heartbeat  time.Duration
	welcome    protocol.Welcome
	wg         sync.WaitGroup
	finished   chan *activeLease
	jobHooks   []JobHook
	runCtx     context.Context
	ticker     Ticker
	closing    chan struct{}
	sendFailed chan error
}

// New checks cfg and returns a Session.
func New(cfg Config) (*Session, error) {
	switch {
	case cfg.Carrier == nil, cfg.Key == nil, cfg.Slots == nil, cfg.Runner == nil, cfg.StateDir == "":
		return nil, errors.New("session: Carrier, Key, Slots, Runner and StateDir are required")
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	if len(cfg.Facts.CompatModes) == 0 {
		cfg.Facts.CompatModes = []protocol.CompatMode{protocol.Compat14, protocol.Compat15}
	}
	s := &Session{cfg: cfg, active: map[string]*activeLease{}, seenEpoch: map[string]uint64{}, finished: make(chan *activeLease, 64), closing: make(chan struct{}), sendFailed: make(chan error, 1)}
	s.jobHooks = cfg.Hooks
	if s.jobHooks == nil {
		s.jobHooks = registeredHooks()
	}
	return s, nil
}

// Run drives the session until the carrier ends, a drain completes, the
// coordinator rejects hello, or ctx is cancelled. A clean end returns nil.
// Running jobs are cancelled and their results sent before it returns.
func (s *Session) Run(ctx context.Context) error {
	s.runCtx = ctx
	type item struct {
		m   protocol.Message
		err error
	}
	recv := make(chan item)
	stop := make(chan struct{})
	go func() {
		for {
			m, err := s.cfg.Carrier.Recv()
			select {
			case recv <- item{m, err}:
			case <-stop:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	defer func() {
		close(stop)
		_ = s.cfg.Carrier.Close()
	}()
	defer s.shutdown()

	if s.cfg.Resume != nil {
		if s.cfg.Resume.Welcome.Version != 1 {
			return &protocol.Error{Code: protocol.CodeVersion, Reason: "welcome_version"}
		}
		s.sendSeq, s.welcome = s.cfg.Resume.SendSeq, s.cfg.Resume.Welcome
	} else {
		if err := s.sendHello(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case it := <-recv:
			if it.err != nil {
				return s.endErr(it.err)
			}
			if err := s.onHandshake(it.m); err != nil {
				return err
			}
		}
	}
	s.heartbeat = time.Duration(s.welcome.HeartbeatMS) * time.Millisecond
	if s.heartbeat <= 0 {
		s.heartbeat = 10 * time.Second
	}
	if s.heartbeat < minHeartbeat {
		s.heartbeat = minHeartbeat // a welcome cannot make the agent spin
	}
	s.ticker = s.cfg.Clock.NewTicker(s.heartbeat)
	defer s.ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-s.sendFailed:
			return fmt.Errorf("carrier send: %w", err)
		case <-s.ticker.C():
			s.beat(nil)
		case al := <-s.finished:
			if s.active[al.lease.LeaseID] == al {
				delete(s.active, al.lease.LeaseID)
			}
			if s.draining && len(s.active) == 0 {
				return nil
			}
		case it := <-recv:
			if it.err != nil {
				return s.endErr(it.err)
			}
			done, err := s.onMessage(it.m)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
	}
}

func (s *Session) endErr(err error) error {
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func (s *Session) send(kind string, body any, l *activeLease) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	m := protocol.Message{V: 1, Type: kind, Seq: s.sendSeq + 1, Body: body}
	if l != nil {
		e := l.lease.Epoch
		m.Epoch, m.LeaseID = &e, l.lease.LeaseID
	}
	if err := s.cfg.Carrier.Send(m); err != nil {
		select { // the Run loop ends the session: a deaf agent must not keep running jobs
		case s.sendFailed <- err:
		default:
		}
		return err
	}
	s.sendSeq++
	return nil
}

func (s *Session) sendHello() error {
	f := s.cfg.Facts
	used, err := s.cfg.Slots.Used()
	if err != nil {
		return err
	}
	fps := f.HostKeyFingerprints
	if fps == nil {
		fps = []string{}
	}
	return s.send("hello", protocol.Hello{
		Versions: []int{1}, AgentVersion: f.AgentVersion, AgeRecipient: f.AgeRecipient,
		NodeKey:             protocol.NodeKey{Alg: "ed25519", KeyID: s.cfg.Key.KeyID(), Public: s.cfg.Key.PublicString()},
		CapabilityDigest:    f.CapabilityDigest,
		Identity:            protocol.Identity{MachineID: f.MachineID, HostKeyFingerprints: fps},
		CompatModeSupported: f.CompatModes,
		Slots:               protocol.Slots{Total: s.cfg.Slots.Total(), Used: used},
	}, nil)
}

func (s *Session) onHandshake(m protocol.Message) error {
	switch b := m.Body.(type) {
	case *protocol.Welcome:
		if b.Version != 1 {
			return &protocol.Error{Code: protocol.CodeVersion, Reason: "welcome_version"}
		}
		s.welcome = *b
		return RecordWelcome(s.cfg.StateDir, s.cfg.Coordinator, *b, s.cfg.Clock.Now())
	case *protocol.Reject:
		return &RejectError{*b}
	}
	return &protocol.Error{Code: protocol.CodeInvalid, Reason: "expected_welcome"}
}
