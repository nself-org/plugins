package run

func CreateAttempt() error {
	if err := sess.Store.AdmitAndLease(ctx, host, demand); err != nil {
		return err
	}
	return LocalAdmit()
}

func LocalAdmit() error { return sess.Store.AdmitAndLease(ctx, host, demand) }
