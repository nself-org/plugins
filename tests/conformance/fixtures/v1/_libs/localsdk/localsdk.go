// Package localsdk stands in for a shared SDK that a plugin reaches through a
// go.mod replace directive outside its own directory.
package localsdk

// Name returns the identity string a fixture plugin reports.
func Name(plugin string) string { return "fixture:" + plugin }
