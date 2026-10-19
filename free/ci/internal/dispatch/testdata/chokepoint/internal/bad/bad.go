package bad

// Planted violations: neither function is in internal/store, internal/lease,
// internal/dispatch or run.CreateAttempt.
func Sneak(s *store.Store) error { return s.AdmitAndLease(ctx, "host", demand) }

func Grantor(e *lease.Engine) { _, _ = e.Grant(ctx, "host", demand, 1) }

// Closure: the call belongs to the enclosing function, still a violation.
func Closure(s *store.Store) func() error { return func() error { return s.AdmitPlacement(ctx) } }

// A method value hands the function to code the scan never sees calling it.
func ValueAlias(s *store.Store) any { f := s.AdmitPlacement; return f }

// A method expression names the function without a call.
func MethodExpr() any { return (*lease.Engine).Grant }

func Reflect(s *store.Store) { reflect.ValueOf(s).MethodByName("AdmitAndLease") }

// Reflection with a spelling no literal scan matches.
func Reflect2(s *store.Store) { reflect.ValueOf(s).MethodByName("Admit" + "Placement") }
