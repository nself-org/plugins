package sched

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

type presetFixture struct {
	Runners []struct {
		ID         string `json:"id"`
		Class      string `json:"class"`
		Location   string `json:"location"`
		Wait       int64  `json:"wait_ms"`
		Runtime    int64  `json:"runtime_ms"`
		Cold       int64  `json:"cold_start_ms"`
		Transfer   int64  `json:"transfer_ms"`
		Contention int64  `json:"contention_ms"`
		Cost       int64  `json:"cost_micro"`
		Scarcity   int64  `json:"scarcity_ms"`
	} `json:"runners"`
}

func TestPresetFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/presets/world.json")
	if err != nil {
		t.Fatal(err)
	}
	var f presetFixture
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		scores map[string]int64
		order  string
	}{
		"balanced":              {map[string]int64{"L": 90000, "N": 57000, "H": 118000, "E": 108000}, "NLEH"},
		"prefer-local":          {map[string]int64{"L": 75000, "N": 72000, "H": 156000, "E": 176000}, "NLHE"},
		"prefer-included":       {map[string]int64{"L": 160000, "N": 97000, "H": 93000, "E": 168000}, "HNLE"},
		"preserve-hosted-quota": {map[string]int64{"L": 90000, "N": 57000, "H": 253000, "E": 228000}, "NLEH"},
		"fastest":               {map[string]int64{"L": 67500, "N": 57000, "H": 93000, "E": 98000}, "NLHE"},
		"cheapest":              {map[string]int64{"L": 22500, "N": 14250, "H": 73250, "E": 424500}, "NLHE"},
	}
	if len(want) != len(Presets()) {
		t.Fatal("preset count")
	}
	for name, expected := range want {
		w := World{Policy: Policy{Preset: name}, Estimates: map[string]map[string]Sample{}}
		j := fixtureJob("j")
		scores := map[string]int64{}
		ids := []string{}
		for _, row := range f.Runners {
			r := fixtureRunner(row.ID, row.Location)
			r.Class = row.Class
			w.Estimates[row.ID] = map[string]Sample{"j": {WaitMs: ptr(row.Wait), RuntimeMs: ptr(row.Runtime), ColdStartMs: ptr(row.Cold), TransferMs: ptr(row.Transfer), ContentionMs: ptr(row.Contention), CostMicro: ptr(row.Cost), ScarcityMs: ptr(row.Scarcity), ReliabilityMs: ptr(int64(0)), CacheMs: ptr(int64(0)), Confidence: "known"}}
			v, _ := score(w, j, r)
			scores[row.ID] = v
			ids = append(ids, row.ID)
		}
		sort.Slice(ids, func(i, k int) bool {
			if scores[ids[i]] == scores[ids[k]] {
				return ids[i] < ids[k]
			}
			return scores[ids[i]] < scores[ids[k]]
		})
		order := ""
		for _, id := range ids {
			order += id
		}
		if order != expected.order {
			t.Errorf("%s order %s want %s", name, order, expected.order)
		}
		for id, expectedScore := range expected.scores {
			if scores[id] != expectedScore {
				t.Errorf("%s %s score %d want %d", name, id, scores[id], expectedScore)
			}
		}
	}
}
