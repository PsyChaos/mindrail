package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/credential"
)

type routerCredentialStore struct {
	value string
	err   error
}

func (s routerCredentialStore) Get(context.Context) (string, error) { return s.value, s.err }
func (routerCredentialStore) Set(context.Context, string) error     { return nil }
func (routerCredentialStore) Delete(context.Context) error          { return nil }

func TestJEVRouterReturnsValidatedAdviceAndMetadata(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "router-secret")
	input := `{"goal":"choose","tools":[{"id":"rg","description":"search"},{"id":"git","description":"inspect history"}]}`
	router := NewRouterWithDependencies(RouterDependencies{
		FindPython: func() (string, error) { return "/trusted/python3", nil },
		RunPython: func(_ context.Context, python string, got []byte, key string) ([]byte, error) {
			if python != "/trusted/python3" || string(got) != input || key != "router-secret" {
				t.Fatalf("unexpected adapter input: python=%q input=%q key=%q", python, got, key)
			}
			return []byte(`{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"ok","reason":"advice_available","selections":{"tool":{"candidate":"rg","confidence":0.9,"accepted":true,"reason":"accepted"}}}`), nil
		},
	})

	result := router.Route(context.Background(), strings.NewReader(input))
	if result.Status != "ok" || result.Reason != "advice_available" || result.CredentialSource != "environment" {
		t.Fatalf("unexpected result metadata: %#v", result)
	}
	selection, ok := result.Selections["tool"]
	if !ok || selection.Candidate == nil || *selection.Candidate != "rg" || !selection.Accepted {
		t.Fatalf("unexpected selection: %#v", result.Selections)
	}
	if result.CandidateCounts["tool"] != 2 {
		t.Fatalf("candidate counts: %#v", result.CandidateCounts)
	}
	if strings.Contains(string(result.JSON), "router-secret") {
		t.Fatal("credential leaked to result")
	}
}

func TestJEVRouterMissingCredentialDoesNotReadInputOrLaunch(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	router := NewRouterWithDependencies(RouterDependencies{
		Store: routerCredentialStore{err: credential.ErrNotFound},
		FindPython: func() (string, error) {
			t.Fatal("python lookup called")
			return "", nil
		},
		RunPython: func(context.Context, string, []byte, string) ([]byte, error) {
			t.Fatal("adapter called")
			return nil, nil
		},
	})

	result := router.Route(context.Background(), panicRouteReader{})
	if result.Status != "disabled" || result.Reason != "api_key_missing" || result.CredentialSource != "none" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestJEVRouterUnavailableCredentialUsesNonSensitiveNoneSource(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	router := NewRouterWithDependencies(RouterDependencies{
		Store: routerCredentialStore{err: errors.New("keyring backend unavailable")},
		FindPython: func() (string, error) {
			t.Fatal("python lookup called")
			return "", nil
		},
	})
	result := router.Route(context.Background(), panicRouteReader{})
	if result.Status != "fallback" || result.Reason != "credential_unavailable" || result.CredentialSource != "none" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestJEVRouterRejectsSelectionOutsideCallerCandidates(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "router-secret")
	router := NewRouterWithDependencies(RouterDependencies{
		FindPython: func() (string, error) { return "/trusted/python3", nil },
		RunPython: func(context.Context, string, []byte, string) ([]byte, error) {
			return []byte(`{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"ok","reason":"advice_available","selections":{"tool":{"candidate":"shell","confidence":0.9,"accepted":true,"reason":"accepted"}}}`), nil
		},
	})

	result := router.Route(context.Background(), strings.NewReader(`{"goal":"choose","tools":[{"id":"rg","description":"search"}]}`))
	if result.Status != "fallback" || result.Reason != "adapter_failure" || len(result.Selections) != 0 {
		t.Fatalf("untrusted selection escaped validation: %#v", result)
	}
}

func TestJEVRouterReadFailureIsSanitized(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "router-secret")
	router := NewRouterWithDependencies(RouterDependencies{
		FindPython: func() (string, error) { return "/trusted/python3", nil },
		RunPython: func(context.Context, string, []byte, string) ([]byte, error) {
			t.Fatal("adapter called after read failure")
			return nil, nil
		},
	})

	result := router.Route(context.Background(), errorRouteReader{err: errors.New("sensitive read detail")})
	if result.Status != "fallback" || result.Reason != "adapter_failure" || bytes.Contains(result.JSON, []byte("sensitive")) {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestJEVRouterProviderTimeoutFailsOpenWithinBound(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "router-secret")
	router := NewRouterWithDependencies(RouterDependencies{
		FindPython:      func() (string, error) { return "/trusted/python3", nil },
		ProviderTimeout: 20 * time.Millisecond,
		RunPython: func(ctx context.Context, _ string, _ []byte, _ string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})

	started := time.Now()
	result := router.Route(context.Background(), strings.NewReader(`{"goal":"choose","tools":[{"id":"rg","description":"search"}]}`))
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("route exceeded bounded provider period: %s", elapsed)
	}
	if result.Status != "fallback" || result.Reason != "provider_timeout" || len(result.Selections) != 0 {
		t.Fatalf("unexpected timeout fallback: %#v", result)
	}
}

func TestJEVRouterRejectsValidAdviceReturnedAfterDeadline(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "router-secret")
	router := NewRouterWithDependencies(RouterDependencies{
		FindPython:      func() (string, error) { return "/trusted/python3", nil },
		ProviderTimeout: 10 * time.Millisecond,
		RunPython: func(ctx context.Context, _ string, _ []byte, _ string) ([]byte, error) {
			<-ctx.Done()
			return []byte(`{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"ok","reason":"advice_available","selections":{"tool":{"candidate":"rg","confidence":0.9,"accepted":true,"reason":"accepted"}}}`), nil
		},
	})

	result := router.Route(context.Background(), strings.NewReader(`{"goal":"choose","tools":[{"id":"rg","description":"search"}]}`))
	if result.Status != "fallback" || result.Reason != "provider_timeout" || len(result.Selections) != 0 {
		t.Fatalf("expired advice escaped: %#v", result)
	}
}

func TestJEVChildEnvironmentContainsOnlyResolvedKey(t *testing.T) {
	const resolved = "RESOLVED_SENTINEL_SECRET"
	for name, value := range map[string]string{
		"HTTPS_PROXY": "http://attacker.invalid:8080", "ALL_PROXY": "socks5://attacker.invalid:1080",
		"SSL_CERT_FILE": "/attacker/ca.pem", "LD_PRELOAD": "/attacker/inject.so",
		"PYTHONPATH": "/attacker/python", "TYPESAFE_API_KEY": "INHERITED_SENTINEL_SECRET",
	} {
		t.Setenv(name, value)
	}
	if got, want := childEnvironment(resolved), []string{"TYPESAFE_API_KEY=" + resolved}; !slices.Equal(got, want) {
		t.Fatalf("child environment = %#v want %#v", got, want)
	}
}

func TestJEVRouterBoundedBufferRejectsOverflow(t *testing.T) {
	var buffer boundedBuffer
	n, err := buffer.Write(bytes.Repeat([]byte("x"), maxRouteOutput+17))
	if n != maxRouteOutput || !errors.Is(err, io.ErrShortWrite) || buffer.Len() != maxRouteOutput {
		t.Fatalf("Write()=(%d,%v), len=%d", n, err, buffer.Len())
	}
}

type panicRouteReader struct{}

func (panicRouteReader) Read([]byte) (int, error) { panic("input must not be read") }

type errorRouteReader struct{ err error }

func (r errorRouteReader) Read([]byte) (int, error) { return 0, r.err }
