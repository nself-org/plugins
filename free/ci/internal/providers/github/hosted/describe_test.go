package hosted_test

// Purpose: the Describe acceptance test of P7-HOST-03.
// Inputs:  fakeGH with public, private and failing visibility lookups.
// Outputs: pass or fail.
// Constraints: no live network.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

func hasLabel(c model.Capability, l string) bool {
	for _, have := range c.Identity.Labels {
		if have == l {
			return true
		}
	}
	return false
}

// TestDescribe proves the Epic D5 acceptance: a public fixture repository gets runners only for the
// labels in labels.yaml with class included, isolation hosted-disposable, secret_classes [none] and
// label nself.visibility=public; a private repository carries no public label, so SCHED's allowance
// rule explains allowance.unknown until the owner declares a ceiling; a repository whose visibility
// lookup fails is described as private. Every capability validates against the public schema.
func TestDescribe(t *testing.T) {
	v15(t)
	ctx := context.Background()
	names, err := githubtmpl.LabelNames()
	if err != nil {
		t.Fatal(err)
	}
	q := providers.Query{Project: "widgets", Repo: "acme/widgets"}
	check := func(t *testing.T, caps []model.Capability, public bool) {
		t.Helper()
		if len(caps) != len(names) {
			t.Fatalf("%d runners for %d labels", len(caps), len(names))
		}
		for i, c := range caps {
			if err := model.ValidateCapability(c); err != nil {
				t.Fatalf("%s: %v", c.Identity.ID, err)
			}
			label := names[i]
			if !hasLabel(c, hosted.LabelRunner+label) || strings.Contains(c.Identity.ID, "large") || c.Identity.ID != "github-hosted:acme/widgets:"+label {
				t.Errorf("runner %q is not for label %q", c.Identity.ID, label)
			}
			if got := hasLabel(c, hosted.LabelPublic); got != public {
				t.Errorf("%s: public label = %v, want %v", label, got, public)
			}
			if *c.Economics.Class.Value != "included" || *c.Trust.Isolation.Value != "hosted-disposable" || c.Identity.Provider != hosted.ProviderID ||
				len(*c.Trust.SecretClasses.Value) != 1 || (*c.Trust.SecretClasses.Value)[0] != "none" || (*c.Trust.Network.Value)[0] != "internet" || c.SecretsEligible {
				t.Errorf("%s: facts %+v", label, c.Trust)
			}
			if !hasLabel(c, hosted.LabelProject+"widgets") || !hasLabel(c, hosted.LabelRepo+"acme/widgets") {
				t.Errorf("%s: labels %v", label, c.Identity.Labels)
			}
		}
	}
	t.Run("public", func(t *testing.T) {
		f := newFakeGH(t)
		caps, err := f.provider(options{}).Describe(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		check(t, caps, true)
		if n := len(f.Reqs); n != 1 {
			t.Fatalf("%d requests; the visibility lookup is the only one", n)
		}
	})
	t.Run("private", func(t *testing.T) {
		f := newFakeGH(t)
		f.Private, f.Visibility = true, "private"
		caps, err := f.provider(options{}).Describe(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		check(t, caps, false)
	})
	t.Run("the query cannot claim a visibility", func(t *testing.T) {
		f := newFakeGH(t)
		f.Private, f.Visibility = true, "private"
		claim := q
		claim.Visibility = "public"
		caps, err := f.provider(options{}).Describe(ctx, claim)
		if err != nil {
			t.Fatal(err)
		}
		check(t, caps, false)
	})
	t.Run("lookup fails: private", func(t *testing.T) {
		f := newFakeGH(t)
		f.RepoStatus = 500
		caps, err := f.provider(options{}).Describe(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		check(t, caps, false)
	})
	t.Run("an internal repository is not public", func(t *testing.T) {
		f := newFakeGH(t)
		f.Visibility = "internal"
		caps, _ := f.provider(options{}).Describe(ctx, q)
		check(t, caps, false)
	})
	t.Run("nothing to describe without a repository or without v1.5", func(t *testing.T) {
		f := newFakeGH(t)
		if caps, err := f.provider(options{}).Describe(ctx, providers.Query{}); err != nil || caps != nil || len(f.Reqs) != 0 {
			t.Fatalf("empty query: %v %v %d", caps, err, len(f.Reqs))
		}
		t.Setenv("NSELF_V15", "")
		if caps, err := f.provider(options{}).Describe(ctx, q); err != nil || caps != nil || len(f.Reqs) != 0 {
			t.Fatalf("v1.4: %v %v %d", caps, err, len(f.Reqs))
		}
	})
	t.Run("a missing credential reaches the caller", func(t *testing.T) {
		f := newFakeGH(t)
		p := hosted.New(hosted.Config{Client: func(context.Context) (*rest.Client, error) {
			c := f.client()
			c.Auth = func(context.Context) (string, error) {
				return "", providers.NewError(providers.Config, "E700", "token_env", 0, time.Time{})
			}
			return c, nil
		}})
		_, err := p.Describe(ctx, q)
		wantCode(t, err, "E700", "")
	})
	t.Run("a malformed repository is refused", func(t *testing.T) {
		f := newFakeGH(t)
		_, err := f.provider(options{}).Describe(ctx, providers.Query{Repo: "../etc"})
		wantCode(t, err, "E702", "repo")
	})
}
