package authz

// Purpose: the size limits of an extraction and the checks that use them.
// Inputs:  Limits (zero fields take the defaults), Entry sizes.
// Outputs: Limits.norm, checkSize, ratioCheck (all used by ArtifactPath and Extractor).
// Constraints: pure. An unknown packed size counts as one byte, so a large
// entry that cannot show its compression ratio is refused, not trusted.

// Limits bound an extraction. A zero field takes the default.
type Limits struct {
	MaxEntries    int
	MaxFileBytes  int64
	MaxTotalBytes int64
	MaxRatio      int64 // uncompressed:compressed, per entry and overall
	RatioFloor    int64 // entries and totals at or below this size skip the ratio test
	MaxNameBytes  int
	MaxComponent  int
	MaxDepth      int
}

// DefaultLimits are the values a zero Limits field takes.
func DefaultLimits() Limits {
	return Limits{MaxEntries: 1 << 20, MaxFileBytes: 2 << 30, MaxTotalBytes: 20 << 30, MaxRatio: 100, RatioFloor: 1 << 20,
		MaxNameBytes: 4096, MaxComponent: 255, MaxDepth: 128}
}

func (l Limits) norm() Limits {
	d := DefaultLimits()
	fill := func(v *int64, def int64) {
		if *v <= 0 {
			*v = def
		}
	}
	fillInt := func(v *int, def int) {
		if *v <= 0 {
			*v = def
		}
	}
	fillInt(&l.MaxEntries, d.MaxEntries)
	fill(&l.MaxFileBytes, d.MaxFileBytes)
	fill(&l.MaxTotalBytes, d.MaxTotalBytes)
	fill(&l.MaxRatio, d.MaxRatio)
	fill(&l.RatioFloor, d.RatioFloor)
	fillInt(&l.MaxNameBytes, d.MaxNameBytes)
	fillInt(&l.MaxComponent, d.MaxComponent)
	fillInt(&l.MaxDepth, d.MaxDepth)
	return l
}

func checkSize(e Entry, lim Limits) error {
	if e.Size < 0 {
		return reject(SizeNegative, e.Name)
	}
	if e.Mode.Type() != 0 { // directories and links hold no bytes
		return nil
	}
	if e.Size > lim.MaxFileBytes {
		return reject(SizeFile, e.Name)
	}
	return ratioCheck(e.Size, e.Packed, lim, e.Name)
}

// ratioCheck refuses size > ratio * packed above the floor. An unknown packed
// size counts as one byte, so a large entry with no packed size is refused.
func ratioCheck(size, packed int64, lim Limits, name string) error {
	if size <= lim.RatioFloor {
		return nil
	}
	if packed < 1 {
		packed = 1
	}
	if size/packed > lim.MaxRatio {
		return reject(SizeRatio, name)
	}
	return nil
}
