package store

// ledgerValues rejects signed amounts before they can reduce a budget's
// held total. Zero is valid (free usage); sums of positive int64 values
// saturate at MaxInt64 in the readers.
func ledgerValues(values ...int64) error {
	for _, v := range values {
		if v < 0 {
			return coded("E607", "ledger amounts and durations must be nonnegative")
		}
	}
	return nil
}

func (r RateSnap) validate() error {
	return ledgerValues(r.RateMicro, r.RateUnitMs, r.IncMs, r.MinMicro, r.ExtrasMicro)
}
