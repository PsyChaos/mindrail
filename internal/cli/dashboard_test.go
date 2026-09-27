package cli

import (
	"context"
	"testing"

	"github.com/PsyChaos/mindrail/internal/credential"
)

func TestResolveDashboardJEVUsesEnvironmentWithoutReadingKeyring(t *testing.T) {
	store := &fakeCredentialStore{value: "keyring-secret"}
	state := resolveDashboardJEV(context.Background(), []string{
		"UNRELATED=must-not-cross-dashboard-boundary",
		"TYPESAFE_API_KEY=environment-secret",
	}, store)
	if !state.Configured || state.Source != "environment" {
		t.Fatalf("state = %#v", state)
	}
	if store.getCalls != 0 {
		t.Fatalf("keyring reads = %d, want 0", store.getCalls)
	}
}

func TestResolveDashboardJEVReportsKeyringMissingAndUnavailable(t *testing.T) {
	tests := []struct {
		name       string
		store      *fakeCredentialStore
		configured bool
		source     string
	}{
		{name: "keyring", store: &fakeCredentialStore{value: "keyring-secret"}, configured: true, source: "keyring"},
		{name: "missing", store: &fakeCredentialStore{getErr: credential.ErrNotFound}, source: "none"},
		{name: "blank", store: &fakeCredentialStore{value: " \t"}, source: "none"},
		{name: "unavailable", store: &fakeCredentialStore{getErr: credential.ErrUnavailable}, source: "unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := resolveDashboardJEV(context.Background(), []string{"TYPESAFE_API_KEY= "}, test.store)
			if state.Configured != test.configured || state.Source != test.source {
				t.Fatalf("state = %#v", state)
			}
		})
	}
}

func TestResolveDashboardJEVCancelledReadFailsOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	state := resolveDashboardJEV(ctx, nil, &fakeCredentialStore{value: "must-not-be-used"})
	if state.Configured || state.Source != "unavailable" {
		t.Fatalf("state = %#v", state)
	}
}
