package secrets

// platformSet stores value under account with an access control list that
// trusts only the running serenity binary (SEC-L08). Another process
// running as the same user is prompted by macOS before it can read it.
func platformSet(account, value string) error {
	exe, err := trustedExecutable()
	if err != nil {
		return err
	}
	return addGenericPassword("", Service, account, value, exe)
}
