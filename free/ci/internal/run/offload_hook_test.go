package run

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const snapSHA = "0123456789abcdef0123456789abcdef01234567"

func TestDirtyRunNoStatus(t *testing.T) {
	released := 0
	var names []string
	h := SnapshotOffload{
		Build: func(_ context.Context, root string) (*OffloadSnapshot, error) {
			if root != "/r" {
				t.Errorf("root %q", root)
			}
			return &OffloadSnapshot{SHA: snapSHA, Excluded: []string{".env.local"}, Release: func(context.Context) error { released++; return nil }}, nil
		},
		Excluded: func(n []string) { names = n },
	}
	var _ OffloadHook = h
	res, err := h.Offload(context.Background(), OffloadRequest{Opts: Options{Root: "/r"}, Rev: Revision{SHA: strings.Repeat("a", 40), Dirty: true}})
	if err != nil || res.Rev.SHA != snapSHA || !res.Rev.Dirty || StatusOfRecord(res.Rev) {
		t.Fatalf("result %+v %v", res, err)
	}
	res.Release()
	if released != 1 || len(names) != 1 {
		t.Fatalf("released %d excluded %v", released, names)
	}
}

func TestOffloadPassThroughAndRefusal(t *testing.T) {
	clean := Revision{SHA: strings.Repeat("b", 40)}
	boom := SnapshotOffload{Build: func(context.Context, string) (*OffloadSnapshot, error) { panic("not for a clean tree") }}
	if res, err := boom.Offload(context.Background(), OffloadRequest{Rev: clean}); err != nil || res.Rev.SHA != "" || !StatusOfRecord(clean) {
		t.Fatalf("clean: %+v %v", res, err)
	}
	refuse := SnapshotOffload{Build: func(context.Context, string) (*OffloadSnapshot, error) {
		return nil, errors.New("E807: tracked file refused")
	}}
	if _, err := refuse.Offload(context.Background(), OffloadRequest{Rev: Revision{SHA: clean.SHA, Dirty: true}}); CodeOf(err) != "E807" {
		t.Fatalf("err = %v", err)
	}
	if res, err := (SnapshotOffload{}).Offload(context.Background(), OffloadRequest{Rev: Revision{SHA: clean.SHA, Dirty: true}}); err != nil || res.Rev.SHA != "" {
		t.Fatalf("nil build: %+v %v", res, err)
	}
	if _, err := boom.Offload(context.Background(), OffloadRequest{Rev: Revision{SHA: clean.SHA, Dirty: true}}); err == nil {
		t.Fatal("panic must refuse the run")
	}
}

func TestDevSummary(t *testing.T) {
	if got := DevSummary("unit", 3, "boom\n"); !strings.Contains(got, "exit 3") || !strings.HasSuffix(got, "boom") {
		t.Fatalf("summary %q", got)
	}
}
