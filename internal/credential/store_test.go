package credential

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeBackend struct {
	value         string
	getErr        error
	setErr        error
	deleteErr     error
	service       string
	account       string
	stored        string
	deleteService string
	deleteAccount string
}

func (b *fakeBackend) Get(service, account string) (string, error) {
	b.service, b.account = service, account
	return b.value, b.getErr
}

func (b *fakeBackend) Set(service, account, secret string) error {
	b.service, b.account, b.stored = service, account, secret
	return b.setErr
}

func (b *fakeBackend) Delete(service, account string) error {
	b.deleteService, b.deleteAccount = service, account
	return b.deleteErr
}

func TestOSStoreUsesStableIdentity(t *testing.T) {
	backend := &fakeBackend{value: "secret"}
	store := osStore{backend: backend}

	if got, err := store.Get(context.Background()); err != nil || got != "secret" {
		t.Fatalf("Get() = %q, %v", got, err)
	}
	if backend.service != "mindrail" || backend.account != "typesafe-api-key" {
		t.Fatalf("identity = %q/%q", backend.service, backend.account)
	}

	if err := store.Set(context.Background(), "replacement"); err != nil {
		t.Fatal(err)
	}
	if backend.stored != "replacement" {
		t.Fatalf("stored = %q", backend.stored)
	}

	if err := store.Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
	if backend.deleteService != "mindrail" || backend.deleteAccount != "typesafe-api-key" {
		t.Fatalf("delete identity = %q/%q", backend.deleteService, backend.deleteAccount)
	}
}

func TestOSStoreMapsNotFound(t *testing.T) {
	store := osStore{backend: &fakeBackend{getErr: errBackendNotFound}}
	if _, err := store.Get(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v", err)
	}

	store = osStore{backend: &fakeBackend{deleteErr: errBackendNotFound}}
	if err := store.Delete(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestOSStoreSanitizesBackendErrors(t *testing.T) {
	sentinel := "SENTINEL_DO_NOT_DISCLOSE"
	backendErr := errors.New("backend failed with " + sentinel)
	tests := []struct {
		name string
		run  func(context.Context, osStore) error
	}{
		{name: "get", run: func(ctx context.Context, s osStore) error { _, err := s.Get(ctx); return err }},
		{name: "set", run: func(ctx context.Context, s osStore) error { return s.Set(ctx, sentinel) }},
		{name: "delete", run: func(ctx context.Context, s osStore) error { return s.Delete(ctx) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakeBackend{getErr: backendErr, setErr: backendErr, deleteErr: backendErr}
			err := test.run(context.Background(), osStore{backend: backend})
			if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), sentinel) {
				t.Fatalf("unsanitized error = %v", err)
			}
		})
	}
}

type blockingBackend struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingBackend) wait() {
	close(b.started)
	<-b.release
}

func (b *blockingBackend) Get(string, string) (string, error) {
	b.wait()
	return "late secret", nil
}

func (b *blockingBackend) Set(string, string, string) error {
	b.wait()
	return nil
}

func (b *blockingBackend) Delete(string, string) error {
	b.wait()
	return nil
}

func TestOSStoreReadIsBoundedByContext(t *testing.T) {
	backend := &blockingBackend{started: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := (osStore{backend: backend}).Get(ctx)
		done <- err
	}()
	<-backend.started

	select {
	case err := <-done:
		if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "late secret") {
			t.Fatalf("bounded read error = %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("credential read ignored context deadline")
	}
	close(backend.release)
}

type committingBackend struct {
	started chan struct{}
	release chan struct{}
	setDone chan struct{}
	delDone chan struct{}
}

func (*committingBackend) Get(string, string) (string, error) { return "", nil }

func (b *committingBackend) Set(string, string, string) error {
	close(b.started)
	<-b.release
	close(b.setDone)
	return nil
}

func (b *committingBackend) Delete(string, string) error {
	close(b.started)
	<-b.release
	close(b.delDone)
	return nil
}

func TestOSStoreMutationCancellationSemanticsPreventLateMutation(t *testing.T) {
	tests := []struct {
		name    string
		run     func(context.Context, osStore) error
		mutated func(*committingBackend) <-chan struct{}
	}{
		{
			name:    "set",
			run:     func(ctx context.Context, store osStore) error { return store.Set(ctx, "secret") },
			mutated: func(backend *committingBackend) <-chan struct{} { return backend.setDone },
		},
		{
			name:    "delete",
			run:     func(ctx context.Context, store osStore) error { return store.Delete(ctx) },
			mutated: func(backend *committingBackend) <-chan struct{} { return backend.delDone },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &committingBackend{
				started: make(chan struct{}), release: make(chan struct{}),
				setDone: make(chan struct{}), delDone: make(chan struct{}),
			}
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- test.run(ctx, osStore{backend: backend}) }()
			<-backend.started
			<-ctx.Done()

			select {
			case err := <-done:
				t.Fatalf("mutation returned after deadline but before outcome: %v", err)
			default:
			}
			select {
			case <-test.mutated(backend):
				t.Fatal("backend mutated before it was released")
			default:
			}

			close(backend.release)
			if err := <-done; err != nil {
				t.Fatalf("completed mutation error = %v", err)
			}
			select {
			case <-test.mutated(backend):
			default:
				t.Fatal("mutation result returned before backend commit")
			}
		})
	}
}

func TestOSStorePreCanceledMutationIsNeverDispatched(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	backend := &fakeBackend{}
	store := osStore{backend: backend}

	if err := store.Set(ctx, "must-not-store"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Set() error = %v, want ErrUnavailable", err)
	}
	if err := store.Delete(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Delete() error = %v, want ErrUnavailable", err)
	}
	if backend.stored != "" || backend.deleteService != "" || backend.deleteAccount != "" {
		t.Fatalf("pre-canceled mutation reached backend: %#v", backend)
	}
}

type unsupportedBackend struct{ fakeBackend }

func (*unsupportedBackend) persistenceSupport() error { return ErrUnsupportedPlatform }

func TestPersistenceSupportExposesUnsupportedPlatform(t *testing.T) {
	if err := PersistenceSupport(osStore{backend: &fakeBackend{}}); err != nil {
		t.Fatalf("supported store capability = %v", err)
	}
	if err := PersistenceSupport(osStore{backend: &unsupportedBackend{}}); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("unsupported store capability = %v, want ErrUnsupportedPlatform", err)
	}
	if err := PersistenceSupport(nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil store capability = %v, want ErrUnavailable", err)
	}
}

func TestNewOSStoreSatisfiesContract(t *testing.T) {
	if store := NewOSStore(); store == nil {
		t.Fatal("NewOSStore returned nil")
	}
}
