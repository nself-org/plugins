package handlers

import "net/http"

// MigrationsHealth supplies the "migrations" member of /health (applied and expected counts). main sets it after
// the boot migrations ran; it stays nil in unit tests that build handlers without a pool.
var MigrationsHealth func(w http.ResponseWriter) map[string]int

// bootMigrationsHealth returns the migrations member for /health, or nil when main did not wire it.
func bootMigrationsHealth(w http.ResponseWriter) map[string]int {
	if MigrationsHealth == nil {
		return nil
	}
	return MigrationsHealth(w)
}
