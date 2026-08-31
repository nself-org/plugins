package router_test

import (
	"testing"

	"github.com/nself-org/nself-cdc/router"
)

// TestTopicMapper_DefaultPrefix verifies default prefix topic generation.
func TestTopicMapper_DefaultPrefix(t *testing.T) {
	tr := router.New("nself")
	cases := []struct {
		table     string
		op        string
		wantTopic string
	}{
		{"np_users", "INSERT", "nself.np_users.insert"},
		{"np_orders", "DELETE", "nself.np_orders.delete"},
		{"np_products", "UPDATE", "nself.np_products.update"},
	}
	for _, tc := range cases {
		got := tr.Topic(tc.table, tc.op)
		if got != tc.wantTopic {
			t.Errorf("Topic(%q, %q) = %q; want %q", tc.table, tc.op, got, tc.wantTopic)
		}
	}
}

// TestTopicMapper_CustomPrefix verifies that CDC_TOPIC_PREFIX is honoured.
func TestTopicMapper_CustomPrefix(t *testing.T) {
	tr := router.New("myapp")
	got := tr.Topic("np_events", "INSERT")
	if got[:5] != "myapp" {
		t.Errorf("expected topic to start with 'myapp', got %q", got)
	}
}

// TestTopicMapper_EmptyPrefixDefaultsToNself checks that empty prefix falls back.
func TestTopicMapper_EmptyPrefixDefaultsToNself(t *testing.T) {
	tr := router.New("")
	got := tr.Topic("np_logs", "DELETE")
	want := "nself.np_logs.delete"
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}
