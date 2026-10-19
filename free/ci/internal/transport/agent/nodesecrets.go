package agent

// Purpose: node-side secret resolution for carrier B (X8, P7-TRUST-02): the lease carries secret references
//          only; this wrapper resolves them on the node and hands the values to the session as a local `secrets`
//          frame, so no value ever travels between the coordinator and the agent.
// Inputs:  the inner carrier; a SecretResolver (the node's own `nself config secrets resolve`, run under its
//          age identity); the leases the coordinator sends.
// Outputs: the lease first, then a synthetic secrets message with the resolved values; nothing when the
//          lease declares no secrets or resolution fails (the session then skips the job as secrets_missing).
// Constraints: resolution runs off the Recv path so frames keep flowing; a value is never logged and never
//          sent upstream; a failure is logged as its code only; a `secrets` frame that arrives from the
//          coordinator is dropped (carrier B refuses values, and the codec already rejects them).

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
)

// Frames is the frame carrier the session drives (session.Carrier).
type Frames interface {
	Send(protocol.Message) error
	Recv() (protocol.Message, error)
	Close() error
}

// SecretResolver resolves the references of one attempt to values, on this node.
type SecretResolver func(ctx context.Context, attemptID string, refs []string) (map[string]string, error)

type nodeSecrets struct {
	inner   Frames
	resolve SecretResolver
	logf    func(string, ...any)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	local  chan protocol.Message
	in     chan recvItem
	once   sync.Once
}

type recvItem struct {
	m   protocol.Message
	err error
}

// WithNodeSecrets wraps inner so leases that declare secrets get them resolved on this node.
func WithNodeSecrets(inner Frames, resolve SecretResolver, logf func(string, ...any)) Frames {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	n := &nodeSecrets{inner: inner, resolve: resolve, logf: logf, ctx: ctx, cancel: cancel, local: make(chan protocol.Message, 16), in: make(chan recvItem)}
	go n.pump()
	return n
}

// pump reads the inner carrier so Recv can also serve the local frames.
func (n *nodeSecrets) pump() {
	for {
		m, err := n.inner.Recv()
		select {
		case n.in <- recvItem{m, err}:
		case <-n.ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}

func (n *nodeSecrets) Send(m protocol.Message) error { return n.inner.Send(m) }

func (n *nodeSecrets) Recv() (protocol.Message, error) {
	for {
		select {
		case m := <-n.local:
			return m, nil
		default:
		}
		select {
		case m := <-n.local:
			return m, nil
		case it := <-n.in:
			if it.err != nil {
				return it.m, it.err
			}
			switch b := it.m.Body.(type) {
			case *protocol.Secrets:
				continue // a value-bearing frame from the coordinator is never trusted
			case *protocol.Lease:
				if len(b.Secrets) > 0 {
					n.start(*b)
				}
			}
			return it.m, nil
		case <-n.ctx.Done():
			return protocol.Message{}, io.EOF
		}
	}
}

// start resolves the lease's references in the background and queues the local secrets frame.
func (n *nodeSecrets) start(l protocol.Lease) {
	refs := make([]string, len(l.Secrets))
	for i, r := range l.Secrets {
		refs[i] = r.Ref
	}
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		vals, err := n.resolve(n.ctx, l.AttemptID, refs)
		if err != nil {
			n.logf("pull: secrets of lease %s could not be resolved on this node: %s", l.LeaseID, codeOf(err))
			return
		}
		body := &protocol.Secrets{LeaseID: l.LeaseID, Values: make([]protocol.SecretValue, 0, len(l.Secrets))}
		for _, r := range l.Secrets {
			ref, perr := secrets.Parse(r.Ref)
			if perr != nil {
				continue
			}
			if v, ok := vals[ref.Name]; ok { // the resolver keys values by variable name
				body.Values = append(body.Values, protocol.SecretValue{Ref: r.Ref, Value: v})
			}
		}
		epoch := l.Epoch
		select {
		case n.local <- protocol.Message{V: 1, Type: "secrets", LeaseID: l.LeaseID, Epoch: &epoch, Body: body}:
		case <-n.ctx.Done():
		}
	}()
}

func (n *nodeSecrets) Close() error {
	var err error
	n.once.Do(func() {
		n.cancel()
		err = n.inner.Close()
		n.wg.Wait()
	})
	return err
}

// codeOf is the error code of a resolver failure (E674..E677); the text may name refs, never values.
func codeOf(err error) string {
	var se *secrets.Error
	if errors.As(err, &se) {
		return se.Code
	}
	return "E676"
}
