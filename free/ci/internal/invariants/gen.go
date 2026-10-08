package invariants

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"math/rand"
	"strconv"
	"syscall"
)

const DefaultSeed int64 = 1701

func Seed() int64 {
	raw, _ := syscall.Getenv("NSELF_CI_INVARIANT_SEED")
	if s, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return s
	}
	return DefaultSeed
}
func Generate(seed int64, n int, visit func(World)) {
	r := rand.New(rand.NewSource(seed))
	t := model.EnumValues("TrustClass")
	i := model.EnumValues("Isolation")
	nw := model.EnumValues("NetworkScope")
	p := model.EnumValues("PrivacyZone")
	s := model.EnumValues("SecretClass")
	for j := 0; j < n; j++ {
		classes := []model.SecretClass{}
		mask := r.Intn(1 << len(s))
		for bit, c := range s {
			if mask&(1<<bit) != 0 {
				classes = append(classes, model.SecretClass(c))
			}
		}
		mode := "personal"
		if r.Intn(2) == 1 {
			mode = "team"
		}
		owner := "operator"
		if r.Intn(2) == 1 {
			owner = "other"
		}
		visit(World{model.TrustClass(t[r.Intn(len(t))]), model.Isolation(i[r.Intn(len(i))]), model.NetworkScope(nw[r.Intn(len(nw))]), classes, model.PrivacyZone(p[r.Intn(len(p))]), mode, owner})
	}
}
