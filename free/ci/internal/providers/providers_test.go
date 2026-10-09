package providers_test

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/fake"
)

// TestRefetchFacts proves absent providers preserve facts and API facts overwrite only their fields.
func TestRefetchFacts(t *testing.T) {
	original := model.TriggerFacts{Source: "not-registered", Repo: "org/repo", Actor: model.Fact[string]{Value: ptr("payload-actor"), Source: "payload"}}
	got, err := providers.RefetchFacts(context.Background(), original)
	if err != nil || got.Actor.Value == nil || *got.Actor.Value != "payload-actor" {
		t.Fatalf("absent provider changed facts: %+v %v", got, err)
	}
	fresh := model.TriggerFacts{Actor: model.Fact[string]{Value: ptr("api-actor"), Source: "api"}, Fork: model.Fact[bool]{Value: boolPtr(true), Source: "api"}, Visibility: model.Fact[string]{Value: ptr("bad-payload"), Source: "payload"}}
	if err := providers.Register(fake.New(fake.Options{Facts: map[string]model.TriggerFacts{"org/repo": fresh}})); err != nil {
		t.Fatal(err)
	}
	original.Source = "fake"
	original.Visibility = model.Fact[string]{Value: ptr("private"), Source: "caller"}
	got, err = providers.RefetchFacts(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if got.Actor.Value == nil || *got.Actor.Value != "api-actor" || got.Fork.Value == nil || !*got.Fork.Value || *got.Visibility.Value != "private" {
		t.Fatalf("wrong refetch merge: %+v", got)
	}
}
func ptr(s string) *string { return &s }
func boolPtr(v bool) *bool { return &v }

func TestRegistry(t *testing.T) {
	if providers.DemandHandlerFor("missing") != nil {
		t.Fatal("unknown demand handler")
	}
	if err := providers.RegisterDemandHandler("fake", fake.New(fake.Options{})); err != nil {
		t.Fatal(err)
	}
	if err := providers.RegisterDemandHandler("fake", fake.New(fake.Options{})); err == nil {
		t.Fatal("duplicate handler accepted")
	}
}
