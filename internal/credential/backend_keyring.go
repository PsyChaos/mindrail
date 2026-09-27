//go:build !darwin

package credential

import (
	"errors"

	keyring "github.com/zalando/go-keyring"
)

type systemKeyring struct{}

func newSystemBackend() keyringBackend { return systemKeyring{} }

func (systemKeyring) persistenceSupport() error { return nil }

func (systemKeyring) Get(service, account string) (string, error) {
	secret, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", errBackendNotFound
	}
	return secret, err
}

func (systemKeyring) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}

func (systemKeyring) Delete(service, account string) error {
	err := keyring.Delete(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return errBackendNotFound
	}
	return err
}
