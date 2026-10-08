package trust

func IsDedicatedTrusted(d Declaration) bool {
	if d.Source == "hello" || d.Source == "probe" || d.Ownership != "operator" || d.Shared {
		return false
	}
	if d.PersonalOwnNode && d.PersonalOwnRun && d.ReleaseJob {
		if !containsTrust(d.Accepts, "owner") {
			return false
		}
		for _, accepted := range d.Accepts {
			if accepted != "owner" && accepted != "internal" {
				return false
			}
		}
		return true
	}
	return d.Dedicated && d.Projects == 1 && len(d.Accepts) == 1 && d.Accepts[0] == "owner"
}
