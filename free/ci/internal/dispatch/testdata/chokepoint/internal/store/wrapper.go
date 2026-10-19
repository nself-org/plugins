package store

// A wrapper in a definer package launders the call to every other package.
func (s *Store) Take() error { return s.AdmitAndLease(ctx, "host", demand) }
