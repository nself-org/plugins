package transport

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

type testDriver struct {
	kind  string
	calls *[]string
}

func (d testDriver) Kind() string { return d.kind }
func (d testDriver) Run(_ context.Context, _ model.Capability, _ protocol.Lease, s Sink) error {
	*d.calls = append(*d.calls, "driver")
	w, err := s.Blob(context.Background(), BlobHeader{Stream: "cache", SHA256: "sha", Size: 1})
	if err != nil {
		return err
	}
	_, err = w.Write([]byte("a"))
	if err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return s.CacheRef(context.Background(), protocol.CacheRef{LeaseID: "l", Spec: "s", Key: "k"})
}

type writer struct{ calls *[]string }

func (w writer) Write(p []byte) (int, error) {
	*w.calls = append(*w.calls, "write")
	return len(p), nil
}
func (w writer) Close() error  { *w.calls = append(*w.calls, "close"); return nil }
func (w writer) Offset() int64 { return 0 }

var _ io.WriteCloser = writer{}

type sink struct{ calls *[]string }

func (s sink) Log(context.Context, protocol.Log) error                 { return nil }
func (s sink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return nil }
func (s sink) Result(context.Context, protocol.Result) error           { return nil }
func (s sink) Blob(_ context.Context, h BlobHeader) (BlobWriter, error) {
	if h.SHA256 == "" {
		return nil, errors.New("digest missing")
	}
	*s.calls = append(*s.calls, "blob")
	return writer(s), nil
}
func (s sink) CacheRef(context.Context, protocol.CacheRef) error {
	*s.calls = append(*s.calls, "cache-ref")
	return nil
}

func TestRegistryAndDelivery(t *testing.T) {
	var calls []string
	kind := "test-registry-and-delivery"
	if _, ok := Lookup(kind); ok {
		t.Fatal("unknown kind found")
	}
	if err := Register(testDriver{kind, &calls}); err != nil {
		t.Fatal(err)
	}
	if err := Register(testDriver{kind, &calls}); err == nil {
		t.Fatal("duplicate accepted")
	}
	for _, n := range []string{"first", "second"} {
		name := n
		if err := RegisterPreparer(func(_ context.Context, _ model.Capability, _ *protocol.Lease, _ model.Job) error {
			calls = append(calls, name)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	node := model.Capability{}
	node.Identity.Transport = kind
	if err := Deliver(context.Background(), node, protocol.Lease{}, sink{&calls}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"first", "second", "driver", "blob", "write", "close", "cache-ref"}) {
		t.Fatal(calls)
	}
	if err := RegisterPreparer(func(context.Context, model.Capability, *protocol.Lease, model.Job) error { return errors.New("stop") }); err != nil {
		t.Fatal(err)
	}
	calls = nil
	if err := Deliver(context.Background(), node, protocol.Lease{}, sink{&calls}); err == nil {
		t.Fatal("preparer error ignored")
	}
	if !reflect.DeepEqual(calls, []string{"first", "second"}) {
		t.Fatal(calls)
	}
}
