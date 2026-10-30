package store

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

func TestNodeLabelsReplaceAudit(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	if err := s.NodeLabelsReplace(ctx, "n1", []NodeLabel{{"zone", "eu"}, {"gpu", ""}}, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := s.NodeLabelsReplace(ctx, "n1", []NodeLabel{{"zone", "us"}, {"ssd", "yes"}}, "bob"); err != nil {
		t.Fatal(err)
	}
	if err := s.NodeLabelsReplace(ctx, "n1", []NodeLabel{{"ssd", "yes"}, {"zone", "us"}}, "bob"); err != nil {
		t.Fatal(err)
	}
	if err := s.NodeLabelsReplace(ctx, "n2", []NodeLabel{{"pool", "a"}}, "alice"); err != nil {
		t.Fatal(err)
	}
	got, err := s.NodeLabels(ctx, "n1")
	if err != nil || !reflect.DeepEqual(got, []NodeLabel{{"ssd", "yes"}, {"zone", "us"}}) {
		t.Fatalf("labels %v %v", got, err)
	}
	all, err := s.NodeLabelsAll(ctx)
	if err != nil || len(all) != 2 || !reflect.DeepEqual(all["n2"], []NodeLabel{{"pool", "a"}}) {
		t.Fatalf("all %v %v", all, err)
	}
	audit, err := s.NodeLabelAudit(ctx, "n1")
	if err != nil {
		t.Fatal(err)
	}
	var acts []string
	for _, c := range audit {
		acts = append(acts, c.Actor+":"+c.Action+":"+c.Key+":"+c.OldValue+">"+c.Value)
	}
	want := []string{"alice:set:gpu:>", "alice:set:zone:>eu", "bob:remove:gpu:>", "bob:set:ssd:>yes", "bob:change:zone:eu>us"}
	if !reflect.DeepEqual(acts, want) {
		t.Fatalf("audit %v", acts)
	}
	if err = s.NodeLabelsReplace(ctx, "n1", nil, "carol"); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.NodeLabels(ctx, "n1"); len(got) != 0 {
		t.Fatalf("cleared labels %v", got)
	}
}

func TestNodeLabelsReplaceRejectsShape(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	for _, c := range []struct {
		node, actor string
		set         []NodeLabel
	}{
		{"", "a", nil},
		{"n", "", nil},
		{"n", "a", []NodeLabel{{"", "v"}}},
		{"n", "a", []NodeLabel{{"k", "1"}, {"k", "2"}}},
	} {
		err := s.NodeLabelsReplace(ctx, c.node, c.set, c.actor)
		if e, ok := err.(*Error); !ok || e.Code != "E607" {
			t.Fatalf("%v: %v", c, err)
		}
	}
}

func TestPipelineProjectsListsDistinctSeen(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	got, err := s.PipelineProjects(ctx)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty store: %v %v", got, err)
	}
	for i, p := range []string{"proj-b", "proj-a", "proj-b"} {
		id := string(rune('a' + i))
		err = s.write(ctx, func(tx *sql.Tx) error {
			_, e := tx.ExecContext(ctx, `INSERT INTO pipeline(id,project,revision,trigger,source_trust,privacy_zone,policy_digest,input_digest,selection_mode,selection_reason,selection_digest,status,created_at) VALUES(?,?,'r','push','t','z','d','i','m','r','s','passed',1)`, "p-"+id, p)
			return e
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err = s.PipelineProjects(ctx)
	if err != nil || !reflect.DeepEqual(got, []string{"proj-a", "proj-b"}) {
		t.Fatalf("projects %v %v", got, err)
	}
}
