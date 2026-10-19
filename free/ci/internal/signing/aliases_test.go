package signing_test

import (
	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
)

func (f *fixture) mutateRecord(name string, fn func(*records.Record)) {
	for i := range f.src.recs {
		if f.src.recs[i].Name == name {
			fn(&f.src.recs[i])
		}
	}
}

// setEvidence replaces the stored run evidence (and so its digest).
func (f *fixture) setEvidence(body string) {
	f.evRaw = []byte(body)
	f.src.evidence[1].Body = f.evRaw
}
