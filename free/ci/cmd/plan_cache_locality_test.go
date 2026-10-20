package main

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestPlanBindsCacheLocalitySource(t *testing.T) {
	repo, _ := placementRepo(t)
	pd, err := loadPlanned(t.Context(), planSpec{repo: repo})
	if err != nil {
		t.Fatal(err)
	}
	if len(pd.pl.Jobs) == 0 {
		t.Fatal("fixture has no jobs")
	}
	digests := map[bool]string{}
	for _, disabled := range []bool{false, true} {
		for i := range pd.pl.Jobs {
			pd.pl.Jobs[i].Job.Cache = &model.JobCacheConfig{Disable: disabled}
		}
		doc, err := placementDoc(t.Context(), pd)
		if err != nil {
			t.Fatal(err)
		}
		digests[disabled] = doc.WorldDigest
	}
	if digests[false] == digests[true] {
		t.Fatal("plan's world has no cache facts for enabled jobs")
	}
}
