package jevconnect

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/credential"
)

const testSecret = "ts-test-secret-never-print"

type memoryStore struct {
	mu     sync.Mutex
	key    string
	setErr error
	sets   int
}

func (s *memoryStore) Get(context.Context) (string, error) { return s.key, nil }
func (s *memoryStore) Delete(context.Context) error        { s.key = ""; return nil }
func (s *memoryStore) Set(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sets++
	if s.setErr == nil {
		s.key = key
	}
	return s.setErr
}

func TestConnectHandlerAcceptsOneSameOriginFormWithoutEchoingKey(t *testing.T) {
	store := &memoryStore{}
	results := make(chan error, 1)
	handler := newConnectHandler(t.Context(), "127.0.0.1:43123", "unguessable", store, results)

	req := trustedPostRequest(url.Values{"api_key": {testSecret}}.Encode())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || store.key != testSecret || store.sets != 1 {
		t.Fatalf("status = %d, key = %q, sets = %d", rec.Code, store.key, store.sets)
	}
	if strings.Contains(rec.Body.String(), testSecret) {
		t.Fatal("success page echoed the API key")
	}
	if err := <-results; err != nil {
		t.Fatalf("result = %v", err)
	}
	assertSecurityHeaders(t, rec.Header())

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, trustedPostRequest(url.Values{"api_key": {testSecret}}.Encode()))
	if second.Code != http.StatusGone || store.sets != 1 {
		t.Fatalf("duplicate status = %d, sets = %d", second.Code, store.sets)
	}
}

func TestConnectHandlerAcceptsNullOriginWithTrustedFetchMetadata(t *testing.T) {
	store := &memoryStore{}
	results := make(chan error, 1)
	handler := newConnectHandler(t.Context(), "127.0.0.1:43123", "unguessable", store, results)
	req := trustedPostRequest(url.Values{"api_key": {testSecret}}.Encode())
	req.Header.Set("Origin", "null")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || store.key != testSecret || store.sets != 1 {
		t.Fatalf("status = %d, key = %q, sets = %d", rec.Code, store.key, store.sets)
	}
	if err := <-results; err != nil {
		t.Fatalf("result = %v", err)
	}
}

func TestConnectHandlerRejectsHostileOriginAndFetchMetadata(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		site   string
		mode   string
		dest   string
	}{
		{name: "missing origin", site: "same-origin", mode: "navigate", dest: "document"},
		{name: "hostile origin", origin: "https://attacker.example", site: "cross-site", mode: "navigate", dest: "document"},
		{name: "null cross site", origin: "null", site: "cross-site", mode: "navigate", dest: "document"},
		{name: "cross site", origin: "http://127.0.0.1:43123", site: "cross-site", mode: "navigate", dest: "document"},
		{name: "cors mode", origin: "http://127.0.0.1:43123", site: "same-origin", mode: "cors", dest: "document"},
		{name: "empty destination", origin: "http://127.0.0.1:43123", site: "same-origin", mode: "navigate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &memoryStore{}
			handler := newConnectHandler(t.Context(), "127.0.0.1:43123", "unguessable", store, make(chan error, 1))
			req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43123/connect?token=unguessable", strings.NewReader("api_key=x"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Sec-Fetch-Site", tc.site)
			req.Header.Set("Sec-Fetch-Mode", tc.mode)
			req.Header.Set("Sec-Fetch-Dest", tc.dest)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden || store.sets != 0 {
				t.Fatalf("status = %d, sets = %d", rec.Code, store.sets)
			}
		})
	}
}

func TestConnectHandlerRejectsMalformedRequestsBeforeStorage(t *testing.T) {
	tests := []struct {
		name, target, contentType, body string
		wantStatus                      int
	}{
		{name: "wrong path", target: "http://127.0.0.1:43123/other?token=unguessable", contentType: "application/x-www-form-urlencoded", body: "api_key=x", wantStatus: http.StatusNotFound},
		{name: "wrong token", target: "http://127.0.0.1:43123/connect?token=wrong", contentType: "application/x-www-form-urlencoded", body: "api_key=x", wantStatus: http.StatusNotFound},
		{name: "wrong media", target: "http://127.0.0.1:43123/connect?token=unguessable", contentType: "application/json", body: `{}`, wantStatus: http.StatusUnsupportedMediaType},
		{name: "extra field", target: "http://127.0.0.1:43123/connect?token=unguessable", contentType: "application/x-www-form-urlencoded", body: "api_key=x&other=y", wantStatus: http.StatusBadRequest},
		{name: "blank", target: "http://127.0.0.1:43123/connect?token=unguessable", contentType: "application/x-www-form-urlencoded", body: "api_key=+", wantStatus: http.StatusBadRequest},
		{name: "oversized", target: "http://127.0.0.1:43123/connect?token=unguessable", contentType: "application/x-www-form-urlencoded", body: "api_key=" + strings.Repeat("x", maxRequestBody), wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &memoryStore{}
			handler := newConnectHandler(t.Context(), "127.0.0.1:43123", "unguessable", store, make(chan error, 1))
			req := httptest.NewRequest(http.MethodPost, tc.target, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			trustRequest(req)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus || store.sets != 0 {
				t.Fatalf("status = %d, sets = %d, body = %q", rec.Code, store.sets, rec.Body.String())
			}
		})
	}
}

func TestConnectRunsProductionShapedLoopbackRoundTrip(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("loopback unavailable: %v", err)
	}
	_ = listener.Close()

	store := &memoryStore{}
	var opened string
	deps := dependencies{
		listen: net.Listen,
		openBrowser: func(_ context.Context, target string) error {
			opened = target
			response, err := http.Get(target)
			if err != nil {
				return err
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(url.Values{"api_key": {testSecret}}.Encode()))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			parsed, _ := url.Parse(target)
			req.Header.Set("Origin", "http://"+parsed.Host)
			req.Header.Set("Sec-Fetch-Site", "same-origin")
			req.Header.Set("Sec-Fetch-Mode", "navigate")
			req.Header.Set("Sec-Fetch-Dest", "document")
			response, err = http.DefaultClient.Do(req)
			if err == nil {
				_ = response.Body.Close()
			}
			return err
		},
		random:  bytes.NewReader(bytes.Repeat([]byte{7}, 64)),
		after:   time.After,
		timeout: time.Second,
	}
	if err := connect(t.Context(), store, deps); err != nil {
		t.Fatalf("connect: %v", err)
	}
	parsed, err := url.Parse(opened)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.Path != connectPath {
		t.Fatalf("opened URL = %q (%v)", opened, err)
	}
	if store.key != testSecret {
		t.Fatalf("stored key = %q", store.key)
	}
}

func TestConnectTimeoutAfterStorageDispatchWaitsForAuthoritativeSuccess(t *testing.T) {
	store := &blockingSuccessStore{started: make(chan struct{}), release: make(chan struct{})}
	result := make(chan error, 1)
	handler := newConnectHandler(t.Context(), "127.0.0.1:43123", "unguessable", store, result)
	response := httptest.NewRecorder()
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		handler.ServeHTTP(response, trustedPostRequest("api_key=x"))
	}()
	<-store.started

	timedOut := make(chan time.Time)
	close(timedOut)
	served := make(chan error)
	done := make(chan error, 1)
	go func() { done <- awaitConnectVerdict(t.Context(), timedOut, served, result, handler) }()

	select {
	case err := <-done:
		t.Fatalf("connect returned before committed Set completed: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(store.release)
	<-requestDone
	if err := <-done; err != nil {
		t.Fatalf("connect returned authoritative Set success as failure: %v", err)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("response status = %d, want %d", response.Code, http.StatusOK)
	}
	if store.saved != "x" {
		t.Fatalf("saved = %q, want x", store.saved)
	}
}

type blockingSuccessStore struct {
	started chan struct{}
	release chan struct{}
	saved   string
}

func (*blockingSuccessStore) Get(context.Context) (string, error) { return "", nil }
func (*blockingSuccessStore) Delete(context.Context) error        { return nil }
func (s *blockingSuccessStore) Set(_ context.Context, key string) error {
	close(s.started)
	<-s.release
	s.saved = key
	return nil
}

func TestBrowserLaunchUsesTrustedCommandAndMinimalEnvironment(t *testing.T) {
	hostile := []string{
		"PATH=/attacker/bin", "BROWSER=/tmp/evil", "LD_PRELOAD=/tmp/evil.so",
		"DYLD_INSERT_LIBRARIES=/tmp/evil.dylib", "HTTPS_PROXY=http://attacker",
		"ALL_PROXY=socks5://attacker", "TYPESAFE_API_KEY=secret",
		"HOME=/home/test", "DISPLAY=:0", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1/bus",
	}
	var command string
	var environment []string
	err := openBrowser(t.Context(), "http://127.0.0.1:43123/connect", hostile,
		func(_ context.Context, gotCommand string, _ []string, gotEnvironment []string) error {
			command = gotCommand
			environment = gotEnvironment
			return nil
		})
	if err != nil {
		t.Skipf("no supported production browser launcher: %v", err)
	}
	if !strings.HasPrefix(command, "/usr/bin/") {
		t.Fatalf("browser command = %q, want trusted absolute system path", command)
	}
	joined := strings.Join(environment, "|")
	for _, forbidden := range []string{"/attacker", "BROWSER=", "LD_PRELOAD=", "DYLD_", "PROXY=", "TYPESAFE_API_KEY="} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("browser environment retained %q: %q", forbidden, joined)
		}
	}
	for _, required := range []string{"HOME=/home/test", "DISPLAY=:0", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1/bus", "PATH=/usr/bin:/bin"} {
		if !strings.Contains(joined, required) {
			t.Errorf("browser environment = %q, want %q", joined, required)
		}
	}
}

func TestStoreFailureIsGenericAndDoesNotLeak(t *testing.T) {
	store := &memoryStore{setErr: errors.New("backend rejected " + testSecret)}
	results := make(chan error, 1)
	handler := newConnectHandler(t.Context(), "127.0.0.1:43123", "unguessable", store, results)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, trustedPostRequest(url.Values{"api_key": {testSecret}}.Encode()))
	if rec.Code != http.StatusServiceUnavailable || strings.Contains(rec.Body.String(), testSecret) {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
	if err := <-results; err == nil || strings.Contains(err.Error(), testSecret) {
		t.Fatalf("result = %v", err)
	} else {
		assertErrorCode(t, err, app.CodeJEVCredentialUnavailable)
	}
}

func TestConcurrentFailureConsumesSessionBeforeQueuedSuccessCanSave(t *testing.T) {
	store := &failThenSucceedStore{started: make(chan struct{}), release: make(chan struct{})}
	results := make(chan error, 1)
	handler := newConnectHandler(t.Context(), "127.0.0.1:43123", "unguessable", store, results)

	first := httptest.NewRecorder()
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		handler.ServeHTTP(first, trustedPostRequest("api_key=first"))
	}()
	<-store.started

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, trustedPostRequest("api_key=second"))
	if second.Code != http.StatusGone {
		t.Fatalf("concurrent status = %d, want %d", second.Code, http.StatusGone)
	}

	close(store.release)
	<-firstDone
	if first.Code != http.StatusServiceUnavailable {
		t.Fatalf("first status = %d, want %d", first.Code, http.StatusServiceUnavailable)
	}
	if store.calls != 1 || store.saved != "" {
		t.Fatalf("store calls = %d, saved = %q; queued request reached storage", store.calls, store.saved)
	}

	afterFailure := httptest.NewRecorder()
	handler.ServeHTTP(afterFailure, trustedPostRequest("api_key=third"))
	if afterFailure.Code != http.StatusGone || store.calls != 1 {
		t.Fatalf("post-failure status = %d, calls = %d", afterFailure.Code, store.calls)
	}
}

type failThenSucceedStore struct {
	started chan struct{}
	release chan struct{}
	calls   int
	saved   string
}

func (*failThenSucceedStore) Get(context.Context) (string, error) { return "", nil }
func (*failThenSucceedStore) Delete(context.Context) error        { return nil }
func (s *failThenSucceedStore) Set(_ context.Context, key string) error {
	s.calls++
	if s.calls == 1 {
		close(s.started)
		<-s.release
		return credential.ErrUnavailable
	}
	s.saved = key
	return nil
}

func TestConnectPreflightsUnsupportedPersistenceBeforeNetworkOrBrowser(t *testing.T) {
	listenCalled := false
	browserCalled := false
	deps := dependencies{
		persistenceSupport: func(credential.Store) error { return credential.ErrUnsupportedPlatform },
		listen: func(string, string) (net.Listener, error) {
			listenCalled = true
			return nil, errors.New("must not listen")
		},
		openBrowser: func(context.Context, string) error {
			browserCalled = true
			return nil
		},
	}
	err := connect(t.Context(), &memoryStore{}, deps)
	assertErrorCode(t, err, app.CodeJEVPersistenceUnsupported)
	if listenCalled || browserCalled {
		t.Fatalf("listen called = %v, browser called = %v", listenCalled, browserCalled)
	}
	payload, _ := app.PayloadOf(err)
	if !strings.Contains(strings.Join(payload.NextAction, " "), "TYPESAFE_API_KEY") {
		t.Fatalf("unsupported-platform guidance = %#v", payload.NextAction)
	}
}

func TestConnectFailureModesUseSpecificCodes(t *testing.T) {
	tests := []struct {
		name string
		code app.Code
		run  func() error
	}{
		{
			name: "loopback unavailable", code: app.CodeJEVConnectUnavailable,
			run: func() error {
				return connect(t.Context(), &memoryStore{}, dependencies{listen: func(string, string) (net.Listener, error) { return nil, errors.New("no listener") }})
			},
		},
		{
			name: "browser unavailable", code: app.CodeJEVBrowserUnavailable,
			run: func() error {
				return connect(t.Context(), &memoryStore{}, blockingConnectDeps(func(context.Context, string) error { return errors.New("no browser") }))
			},
		},
		{
			name: "timeout", code: app.CodeJEVConnectTimeout,
			run: func() error {
				deps := blockingConnectDeps(func(context.Context, string) error { return nil })
				timedOut := make(chan time.Time)
				close(timedOut)
				deps.after = func(time.Duration) <-chan time.Time { return timedOut }
				return connect(t.Context(), &memoryStore{}, deps)
			},
		},
		{
			name: "cancelled", code: app.CodeJEVConnectCancelled,
			run: func() error {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return connect(ctx, &memoryStore{}, blockingConnectDeps(func(context.Context, string) error { return nil }))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { assertErrorCode(t, tc.run(), tc.code) })
	}
}

func TestConnectHandlerBoundsCredentialWrite(t *testing.T) {
	store := &deadlineStore{}
	results := make(chan error, 1)
	handler := newConnectHandler(t.Context(), "127.0.0.1:43123", "unguessable", store, results)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, trustedPostRequest("api_key=x"))
	if rec.Code != http.StatusOK || !store.sawDeadline {
		t.Fatalf("status = %d, saw deadline = %v", rec.Code, store.sawDeadline)
	}
}

type deadlineStore struct{ sawDeadline bool }

func (*deadlineStore) Get(context.Context) (string, error) { return "", nil }
func (*deadlineStore) Delete(context.Context) error        { return nil }
func (s *deadlineStore) Set(ctx context.Context, _ string) error {
	_, s.sawDeadline = ctx.Deadline()
	return nil
}

func trustedPostRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43123/connect?token=unguessable", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	trustRequest(req)
	return req
}

func trustRequest(req *http.Request) {
	req.Header.Set("Origin", "http://127.0.0.1:43123")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")
}

func assertSecurityHeaders(t *testing.T, header http.Header) {
	t.Helper()
	for name, want := range map[string]string{
		"Cache-Control": "no-store", "Content-Security-Policy": "frame-ancestors 'none'",
		"Referrer-Policy": "no-referrer", "X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY",
	} {
		if !strings.Contains(header.Get(name), want) {
			t.Errorf("%s = %q, want substring %q", name, header.Get(name), want)
		}
	}
}

func assertErrorCode(t *testing.T, err error, want app.Code) {
	t.Helper()
	payload, ok := app.PayloadOf(err)
	if !ok || payload.Code != want {
		t.Fatalf("error = %v, code = %q, want %q", err, payload.Code, want)
	}
}

func blockingConnectDeps(openBrowser func(context.Context, string) error) dependencies {
	return dependencies{
		listen:      func(string, string) (net.Listener, error) { return newBlockingListener(), nil },
		openBrowser: openBrowser,
		random:      bytes.NewReader(bytes.Repeat([]byte{7}, 64)),
		after:       time.After,
		timeout:     time.Second,
	}
}

type blockingListener struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingListener() *blockingListener {
	return &blockingListener{closed: make(chan struct{})}
}

func (l *blockingListener) Accept() (net.Conn, error) {
	<-l.closed
	return nil, net.ErrClosed
}

func (l *blockingListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (*blockingListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 43123}
}
