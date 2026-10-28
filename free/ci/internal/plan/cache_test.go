package plan

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestResolvedCachePolicy(t *testing.T) {
	for _, cache := range []*model.CacheConfig{nil, {}, {Disable: true}} {
		p, err := Build(Input{Config: model.PipelineConfig{Version: 1, Cache: cache}})
		if err != nil {
			t.Fatal(err)
		}
		if p.Cache == nil || p.Cache.Disable != (cache != nil && cache.Disable) {
			t.Fatalf("cache %+v from %+v", p.Cache, cache)
		}
		if cache != nil {
			cache.Disable = !cache.Disable
			if p.Cache.Disable == cache.Disable {
				t.Fatal("plan aliases config cache")
			}
		}
	}
}
