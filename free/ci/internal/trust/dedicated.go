package trust

func IsDedicatedTrusted(d Declaration) bool {
	if d.Source == "hello" || d.Source == "probe" || d.Ownership != "operator" || d.Shared || d.Projects != 1 {
		return false
	}
	if len(d.Accepts) != 1 || d.Accepts[0] != "owner" {
		return false
	}
	return d.Dedicated || d.PersonalOwnNode && d.PersonalOwnRun && d.ReleaseJob
}
