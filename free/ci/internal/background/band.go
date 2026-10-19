package background

// Purpose: which kinds of work run in the background band (Epic D9, D14).
// Inputs:  a work kind name.
// Outputs: whether the kind is background, and the band name to declare.
// Constraints: pure; the fair queue's strict band order (fairq) then keeps these
// jobs behind every interactive and normal job. Unknown kinds are not background,
// so a typo never demotes real work.

// BandName is the band a background job declares (fairq.Background's name).
const BandName = "background"

var kinds = map[string]bool{
	"schedule":           true, // scheduled pipelines
	"drill":              true,
	"flaky-confirmation": true,
	"nightly":            true, // nightly legs
}

// IsBackgroundKind reports whether work of this kind runs in the background band.
func IsBackgroundKind(kind string) bool { return kinds[kind] }

// DeclaredBand returns the band name work of this kind declares, or "" when the
// kind leaves the band to the trigger rules of fairq.For.
func DeclaredBand(kind string) string {
	if IsBackgroundKind(kind) {
		return BandName
	}
	return ""
}
