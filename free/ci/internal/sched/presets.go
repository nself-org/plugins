package sched

type Preset struct {
	Weights        [9]int64
	MsPerCostMicro int64
	ClassBiasMs    [5]int64
}

var presetTable = map[string]Preset{
	"balanced":              {[9]int64{1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000}, 1, [5]int64{}},
	"prefer-local":          {[9]int64{1000, 1000, 1000, 2000, 1000, 500, 1000, 1000, 1000}, 1, [5]int64{0, 10000, 30000, 60000, 60000}},
	"prefer-included":       {[9]int64{1000, 1000, 1000, 1000, 1000, 2000, 1000, 0, 1000}, 1, [5]int64{40000, 40000, 0, 60000, 60000}},
	"preserve-hosted-quota": {[9]int64{1000, 1000, 1000, 1000, 1000, 1000, 1000, 4000, 1000}, 1, [5]int64{0, 0, 60000, 120000, 120000}},
	"fastest":               {[9]int64{1000, 1000, 1000, 1000, 1000, 250, 0, 0, 1000}, 0, [5]int64{}},
	"cheapest":              {[9]int64{250, 250, 250, 250, 250, 250, 4000, 2000, 250}, 10, [5]int64{}},
}

// Presets returns independent preset values.
func Presets() map[string]Preset {
	out := make(map[string]Preset, len(presetTable))
	for k, v := range presetTable {
		out[k] = v
	}
	return out
}
func preset(name string) Preset {
	p, ok := presetTable[name]
	if !ok {
		return presetTable["balanced"]
	}
	return p
}
func classIndex(class string) int {
	switch class {
	case "owned":
		return 1
	case "included":
		return 2
	case "metered":
		return 3
	case "ephemeral":
		return 4
	}
	return 0
}
