package run

func CreateAttempt() error {
	// two references where the allowlist holds one: a NEW call in a listed function trips the count
	_ = st.Transition(ctx, id, "queued", "cancelled", d)
	_ = st.Transition(ctx, id, "queued", "cancelled", d)
	if err := sess.Store.AdmitAndLease(ctx, host, demand); err != nil {
		return err
	}
	return LocalAdmit()
}

func LocalAdmit() error { return sess.Store.AdmitAndLease(ctx, host, demand) }
