package store

// The definer: it defines its admission function; it does not call it from a
// wrapper (see wrapper.go).
func (s *Store) AdmitAndLease() error { return nil }
