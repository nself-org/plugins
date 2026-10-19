package store

// The definer: it may call its own admission functions.
func (s *Store) AdmitAndLease() error { return s.inner() }
func (s *Store) inner() error         { return s.AdmitAndLease() }
