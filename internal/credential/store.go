// Package credential provides access to Mindrail-owned credentials stored in
// the operating system's credential service.
package credential

import (
	"context"
	"errors"
)

const (
	serviceName = "mindrail"
	accountName = "typesafe-api-key"
)

var (
	// ErrNotFound indicates that no TypeSafe credential has been stored.
	ErrNotFound = errors.New("credential not found")
	// ErrUnavailable indicates that the operating-system credential store could
	// not safely complete an operation. Callers should continue without JEV or
	// tell the user to use the TYPESAFE_API_KEY automation override.
	ErrUnavailable = errors.New("credential store unavailable")
	// ErrUnsupportedPlatform indicates that Mindrail intentionally refuses
	// persistent credential storage on this platform because no backend meets
	// the application's security contract.
	ErrUnsupportedPlatform = errors.New("credential persistence unsupported on this platform")
	errBackendNotFound     = errors.New("backend credential not found")
)

// Store persists the TypeSafe API key outside repositories and Mindrail
// configuration. Reads must honor context cancellation. Mutations must reject
// a context canceled before dispatch; after dispatch, their returned result
// must describe the completed mutation rather than a speculative timeout.
// Implementations must not expose the value in returned errors.
type Store interface {
	Get(context.Context) (string, error)
	Set(context.Context, string) error
	Delete(context.Context) error
}

// keyringBackend is deliberately private: platform adapters may expose
// different error types, but callers only see the stable, sanitized Store
// contract.
type keyringBackend interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
	Delete(service, account string) error
}

type osStore struct {
	backend keyringBackend
}

// PersistenceCapability is implemented by stores that can explicitly report
// whether interactive persistent storage is safe on the current platform.
type PersistenceCapability interface {
	PersistenceSupport() error
}

type backendPersistenceSupporter interface {
	persistenceSupport() error
}

// NewOSStore returns the operating-system credential store. Supported
// platforms use their native credential service. A platform whose available
// implementation cannot meet Mindrail's security contract returns sanitized
// unavailability errors instead of falling back to a weaker mechanism.
func NewOSStore() Store {
	return osStore{backend: newSystemBackend()}
}

// PersistenceSupport reports whether store can safely persist a credential.
// Custom Store implementations are assumed to support persistence unless they
// implement PersistenceCapability; NewOSStore always implements it.
// Callers should check this before starting an interactive credential flow.
func PersistenceSupport(store Store) error {
	if store == nil {
		return ErrUnavailable
	}
	if supporter, ok := store.(PersistenceCapability); ok {
		return supporter.PersistenceSupport()
	}
	return nil
}

func (s osStore) PersistenceSupport() error {
	if supporter, ok := s.backend.(backendPersistenceSupporter); ok {
		return supporter.persistenceSupport()
	}
	return nil
}

type credentialResult struct {
	secret string
	err    error
}

// awaitBackendRead keeps non-mutating reads bounded even when an
// operating-system API or test double does not itself support cancellation.
// The result channel is buffered so a late completion can exit its goroutine.
// Callers are still required to supply a bounded context.
func awaitBackendRead(ctx context.Context, operation func() credentialResult) credentialResult {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return credentialResult{err: ErrUnavailable}
	default:
	}

	result := make(chan credentialResult, 1)
	go func() { result <- operation() }()
	select {
	case value := <-result:
		return value
	case <-ctx.Done():
		return credentialResult{err: ErrUnavailable}
	}
}

func (s osStore) Get(ctx context.Context) (string, error) {
	result := awaitBackendRead(ctx, func() credentialResult {
		secret, err := s.backend.Get(serviceName, accountName)
		return credentialResult{secret: secret, err: err}
	})
	if errors.Is(result.err, errBackendNotFound) {
		return "", ErrNotFound
	}
	if errors.Is(result.err, ErrUnsupportedPlatform) {
		return "", ErrUnsupportedPlatform
	}
	if result.err != nil {
		return "", ErrUnavailable
	}
	return result.secret, nil
}

func (s osStore) Set(ctx context.Context, secret string) error {
	if mutationCanceledBeforeDispatch(ctx) {
		return ErrUnavailable
	}
	// Dispatch is the commit point. Native credential APIs do not provide a
	// transactional cancellation primitive, so once dispatched we wait for the
	// actual outcome. This prevents reporting a timeout and then mutating later.
	err := s.backend.Set(serviceName, accountName, secret)
	if errors.Is(err, ErrUnsupportedPlatform) {
		return ErrUnsupportedPlatform
	}
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

func (s osStore) Delete(ctx context.Context) error {
	if mutationCanceledBeforeDispatch(ctx) {
		return ErrUnavailable
	}
	// As with Set, completion after dispatch is authoritative even if ctx is
	// canceled while the native call is in progress.
	err := s.backend.Delete(serviceName, accountName)
	if errors.Is(err, errBackendNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, ErrUnsupportedPlatform) {
		return ErrUnsupportedPlatform
	}
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

func mutationCanceledBeforeDispatch(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
