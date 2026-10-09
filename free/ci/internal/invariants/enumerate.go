package invariants

import "github.com/nself-org/plugins/free/ci/internal/model"

type World struct {
	Trust     model.TrustClass
	Isolation model.Isolation
	Network   model.NetworkScope
	Secrets   []model.SecretClass
	Privacy   model.PrivacyZone
	Mode      string
	Ownership string
}

func Enumerate(visit func(World)) int {
	count := 0
	for _, t := range model.EnumValues("TrustClass") {
		for _, i := range model.EnumValues("Isolation") {
			for _, n := range model.EnumValues("NetworkScope") {
				for mask := 0; mask < 64; mask++ {
					for _, p := range model.EnumValues("PrivacyZone") {
						for _, mode := range []string{"personal", "team"} {
							for _, owner := range []string{"operator", "other"} {
								secrets := []model.SecretClass{}
								for bit, c := range model.EnumValues("SecretClass") {
									if mask&(1<<bit) != 0 {
										secrets = append(secrets, model.SecretClass(c))
									}
								}
								visit(World{model.TrustClass(t), model.Isolation(i), model.NetworkScope(n), secrets, model.PrivacyZone(p), mode, owner})
								count++
							}
						}
					}
				}
			}
		}
	}
	return count
}
