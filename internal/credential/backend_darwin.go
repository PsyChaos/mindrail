//go:build darwin

package credential

// unavailableBackend is intentional on macOS. The currently available
// go-keyring adapter shells out to /usr/bin/security, which can make the ACL
// authorize that shared executable rather than the Mindrail binary. Until a
// native, application-bound Keychain adapter is available, failing closed is
// safer than persisting a credential readable by unrelated local processes.
type unavailableBackend struct{}

func newSystemBackend() keyringBackend { return unavailableBackend{} }

func (unavailableBackend) persistenceSupport() error { return ErrUnsupportedPlatform }

func (unavailableBackend) Get(string, string) (string, error) {
	return "", ErrUnsupportedPlatform
}

func (unavailableBackend) Set(string, string, string) error {
	return ErrUnsupportedPlatform
}

func (unavailableBackend) Delete(string, string) error {
	return ErrUnsupportedPlatform
}
