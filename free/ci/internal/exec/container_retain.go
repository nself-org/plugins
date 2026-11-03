package exec

import "strings"

// retainContainer removes automatic removal from validated runtime flags so the
// executor can collect its final host sample before explicitly removing the job.
// Flag values and job arguments (including a literal --rm) are preserved.
func retainContainer(args []string, image string) []string {
	out := []string{args[0]}
	for i := 1; i < len(args); i++ {
		if args[i] == image {
			return append(out, args[i:]...)
		}
		name, _, inline := strings.Cut(args[i], "=")
		if name != "--rm" {
			out = append(out, args[i])
		}
		if plannedFlags[name] && !inline {
			i++
			out = append(out, args[i])
		}
	}
	return out
}
