//go:build darwin

package credential

import (
	"context"
	"errors"
	"testing"
)

func TestDarwinBackendFailsClosedWithoutShellKeychainAdapter(t *testing.T) {
	backend := newSystemBackend()
	if _, ok := backend.(unavailableBackend); !ok {
		t.Fatalf("macOS backend type = %T, want unavailableBackend", backend)
	}
	store := NewOSStore()
	if err := PersistenceSupport(store); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("PersistenceSupport() error = %v, want ErrUnsupportedPlatform", err)
	}
	if _, err := store.Get(context.Background()); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Get() error = %v, want ErrUnsupportedPlatform", err)
	}
	if err := store.Set(context.Background(), "secret"); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Set() error = %v, want ErrUnsupportedPlatform", err)
	}
	if err := store.Delete(context.Background()); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Delete() error = %v, want ErrUnsupportedPlatform", err)
	}
}
