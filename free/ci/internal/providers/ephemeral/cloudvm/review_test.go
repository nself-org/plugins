package cloudvm_test

// Purpose: TestCloudVMReview*: the pins added after the adversarial review (the failure paths a mutation run
// left uncovered): a failed create withdraws its offer, a machine that cannot be used is destroyed with a
// retry, the certificate lives as long as the machine may, one intent holds one offer.
// Inputs:  the rig (fake nself over a fake cloud). Outputs: pass/fail.
// Constraints: no sleeps; the destroy wait is off in the rig.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/ephemeral/cloudvm"
)

func TestCloudVMReviewFailedCreateWithdrawsItsOffer(t *testing.T) {
	r := newRig(t)
	r.serve()
	r.br.set(func(k *knobs) { k.dieOnProvision = true })
	before := r.auditCount("pairing.withdraw")
	if _, err := r.prov.Provision(context.Background(), intentOf("01J0000000000000000000WDR01")); err == nil {
		t.Fatal("a lost response must fail")
	}
	if r.auditCount("pairing.withdraw") != before+1 {
		t.Fatal("the offer of a failed create was not withdrawn: its code would stay live for the offer's lifetime")
	}
	eventually(t, 2*time.Second, "the listener closes with the last offer", func() bool { return !isOpen(r.enrollAdr) })
}

func TestCloudVMReviewUnusableMachineIsDestroyedWithRetry(t *testing.T) {
	r := newRig(t)
	r.br.set(func(k *knobs) { k.stripLabels = true; k.failDestroys = 2 })
	_, err := r.prov.Provision(context.Background(), intentOf("01J0000000000000000000RTY01"))
	if cloudvm.ReasonOf(err) != cloudvm.ReasonAnswer {
		t.Fatalf("want an unusable answer, got %v", err)
	}
	if n := len(r.br.callsOf("destroy")); n != 3 {
		t.Fatalf("%d destroy attempts, want 3", n)
	}
	if len(r.cloud.All()) != 0 {
		t.Fatalf("the unusable machine was left running after a transient destroy failure: %+v", r.cloud.All())
	}
}

func TestCloudVMReviewCertificateLivesAsLongAsTheMachine(t *testing.T) {
	r := newRig(t)
	r.serve()
	res := r.provision("job-1")
	code, fp := r.userDataOf(res.Token)
	joined, err := r.join(code, fp, cloudvm.NodeName(res.Token))
	if err != nil {
		t.Fatal(err)
	}
	want := r.cfg.MaxLifetime + r.cfg.Grace
	if got := joined.NotAfter.Sub(r.clk.Now()); got < want-time.Minute || got > want+time.Minute {
		t.Fatalf("the certificate lives %v, want MaxLifetime+Grace = %v", got, want)
	}
}

func TestCloudVMReviewAnIntentHoldsOneOffer(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	in := intentOf("01J0000000000000000000ONE01")
	if _, err := r.prov.Provision(ctx, in); err != nil {
		t.Fatal(err)
	}
	// the same intent again finds its machine and does not offer a second code
	before := r.auditCount("pairing.offer")
	if _, err := r.prov.Provision(ctx, in); err != nil {
		t.Fatal(err)
	}
	if r.auditCount("pairing.offer") != before {
		t.Fatal("a second offer was made for an intent that already holds one")
	}
}

func TestCloudVMReviewErrorCodeIsNotCopiedUnchecked(t *testing.T) {
	r := newRig(t)
	if _, err := r.prov.Provision(context.Background(), intentOf("01J0000000000000000000COD00")); err != nil { // the probe is cached after the first call
		t.Fatal(err)
	}
	r.br.set(func(k *knobs) { k.failCode = "E744\nSECRET"; k.failCause = "HTTP 503" })
	_, err := r.prov.Provision(context.Background(), intentOf("01J0000000000000000000COD01"))
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("an unchecked code reached the error: %v", err)
	}
}
