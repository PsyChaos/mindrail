// Package jevconnect implements the loopback-only browser flow used to save a
// TypeSafe API key. It is kept outside the CLI package so repository setup can
// retain its compile-time no-network boundary.
package jevconnect

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/credential"
)

const (
	connectPath    = "/connect"
	connectTimeout = 5 * time.Minute
	maxRequestBody = 8 * 1024
	maxAPIKey      = 4 * 1024
	storeTimeout   = 10 * time.Second
)

type dependencies struct {
	persistenceSupport func(credential.Store) error
	listen             func(network, address string) (net.Listener, error)
	openBrowser        func(context.Context, string) error
	random             io.Reader
	after              func(time.Duration) <-chan time.Time
	timeout            time.Duration
}

// Connect opens a private loopback form and stores the submitted key in store.
func Connect(ctx context.Context, store credential.Store) error {
	return connect(ctx, store, dependencies{
		persistenceSupport: credential.PersistenceSupport,
		listen:             net.Listen,
		openBrowser:        openDefaultBrowser,
		random:             rand.Reader,
		after:              time.After,
		timeout:            connectTimeout,
	})
}

func connect(ctx context.Context, store credential.Store, deps dependencies) error {
	persistenceSupport := deps.persistenceSupport
	if persistenceSupport == nil {
		persistenceSupport = credential.PersistenceSupport
	}
	if err := persistenceSupport(store); err != nil {
		if errors.Is(err, credential.ErrUnsupportedPlatform) {
			return persistenceUnsupportedError()
		}
		return credentialError("check")
	}
	listener, err := deps.listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return connectUnavailableError("Mindrail could not start the private local connection page.", "Check whether local loopback connections are allowed, then try again.")
	}
	defer listener.Close()

	host, ok := loopbackHost(listener.Addr())
	if !ok {
		return connectUnavailableError("The connection page did not bind to IPv4 loopback.", "Try again after checking the local network configuration.")
	}
	token, err := randomConnectToken(deps.random)
	if err != nil {
		return connectUnavailableError("Mindrail could not create a secure connection session.", "Try again; if this continues, check the operating system random source.")
	}

	result := make(chan error, 1)
	handler := newConnectHandler(ctx, host, token, store, result)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       15 * time.Second,
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()

	connectURL := "http://" + host + connectPath + "?token=" + url.QueryEscape(token)
	if err := deps.openBrowser(ctx, connectURL); err != nil {
		shutdownServer(server)
		if ctx.Err() != nil {
			return connectContextError(ctx)
		}
		return browserUnavailableError()
	}

	timeout := deps.timeout
	if timeout <= 0 {
		timeout = connectTimeout
	}
	verdict := awaitConnectVerdict(ctx, deps.after(timeout), served, result, handler)
	shutdownServer(server)
	return verdict
}

func awaitConnectVerdict(
	ctx context.Context,
	timedOut <-chan time.Time,
	served <-chan error,
	result <-chan error,
	handler *connectHandler,
) error {
	select {
	case err := <-result:
		return err
	case err := <-served:
		if !handler.stopIfReady() {
			// The credential mutation crossed its commit point. Native stores do
			// not offer transactional cancellation, so only its result can tell
			// the caller whether a key was saved.
			return <-result
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return connectUnavailableError("The private connection page stopped unexpectedly.", "Run mindrail jev connect and try again.")
		}
		return connectUnavailableError("The private connection page closed before a key was saved.", "Run mindrail jev connect and try again.")
	case <-timedOut:
		if !handler.stopIfReady() {
			return <-result
		}
		return connectTimeoutError()
	case <-ctx.Done():
		if !handler.stopIfReady() {
			return <-result
		}
		return connectContextError(ctx)
	}
}

func shutdownServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func loopbackHost(addr net.Addr) (string, bool) {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok || tcpAddr.Port <= 0 || !tcpAddr.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		return "", false
	}
	return net.JoinHostPort("127.0.0.1", fmt.Sprint(tcpAddr.Port)), true
}

func randomConnectToken(reader io.Reader) (string, error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(reader, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

type connectHandler struct {
	ctx    context.Context
	host   string
	token  string
	store  credential.Store
	result chan<- error
	mu     sync.Mutex
	state  connectState
}

type connectState uint8

const (
	connectReady connectState = iota
	connectProcessing
	connectTerminal
)

func newConnectHandler(ctx context.Context, host, token string, store credential.Store, result chan<- error) *connectHandler {
	return &connectHandler{ctx: ctx, host: host, token: token, store: store, result: result, state: connectReady}
}

func (h *connectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	secureHeaders(w.Header())
	if r.Host != h.host || r.URL.Path != connectPath || !exactConnectToken(r.URL.RawQuery, h.token) {
		http.Error(w, "Not found.", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.renderForm(w)
	case http.MethodPost:
		if !sameOriginFormPost(r, h.host) {
			http.Error(w, "Submit the connection form from this page.", http.StatusForbidden)
			return
		}
		h.acceptKey(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method not allowed.", http.StatusMethodNotAllowed)
	}
}

func sameOriginFormPost(r *http.Request, host string) bool {
	return r.Header.Get("Origin") == "http://"+host &&
		r.Header.Get("Sec-Fetch-Site") == "same-origin" &&
		r.Header.Get("Sec-Fetch-Mode") == "navigate" &&
		r.Header.Get("Sec-Fetch-Dest") == "document"
}

func secureHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store, max-age=0")
	header.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}

func exactConnectToken(rawQuery, token string) bool {
	values, err := url.ParseQuery(rawQuery)
	if err != nil || len(values) != 1 || len(values["token"]) != 1 {
		return false
	}
	got := values["token"][0]
	return len(got) == len(token) && subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

var connectPage = template.Must(template.New("jev-connect").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Connect JEV to Mindrail</title><style>body{font:16px system-ui;max-width:34rem;margin:4rem auto;padding:0 1rem}input,button{box-sizing:border-box;font:inherit;width:100%;padding:.75rem;margin:.5rem 0}small{color:#555}</style></head>
<body><h1>Connect JEV to Mindrail</h1><p>The key is saved directly in your operating-system credential store.</p>
<form method="post"><label for="api_key">TypeSafe API key</label><input id="api_key" name="api_key" type="password" required maxlength="4096" autocomplete="off" spellcheck="false"><button type="submit">Connect</button></form>
<small>You may close this tab after Mindrail confirms the connection.</small></body></html>`))

func (h *connectHandler) renderForm(w http.ResponseWriter) {
	h.mu.Lock()
	state := h.state
	h.mu.Unlock()
	if state != connectReady {
		http.Error(w, "This connection session is already complete.", http.StatusGone)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = connectPage.Execute(w, nil)
}

func (h *connectHandler) acceptKey(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		http.Error(w, "Submit the connection form from this page.", http.StatusUnsupportedMediaType)
		return
	}
	if r.ContentLength > maxRequestBody {
		http.Error(w, "The submitted value is too large.", http.StatusRequestEntityTooLarge)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		http.Error(w, "The submitted value is too large.", http.StatusRequestEntityTooLarge)
		return
	}
	values, err := url.ParseQuery(string(body))
	if err != nil || len(values) != 1 || len(values["api_key"]) != 1 {
		http.Error(w, "Submit exactly one API key.", http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(values["api_key"][0])
	if key == "" || len(key) > maxAPIKey {
		http.Error(w, "Enter a valid API key.", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	if h.state != connectReady {
		h.mu.Unlock()
		http.Error(w, "This connection session is already complete.", http.StatusGone)
		return
	}
	h.state = connectProcessing
	h.mu.Unlock()

	// Transitioning to processing is the commit point. Detach from parent
	// cancellation so a timeout cannot be reported while the native mutation
	// may still complete; Store.Set's actual return is authoritative.
	storeCtx, cancel := context.WithTimeout(context.WithoutCancel(h.ctx), storeTimeout)
	defer cancel()
	if err := h.store.Set(storeCtx, key); err != nil {
		h.finish()
		http.Error(w, "Mindrail could not save the key in the operating-system credential store.", http.StatusServiceUnavailable)
		if errors.Is(err, credential.ErrUnsupportedPlatform) {
			h.signal(persistenceUnsupportedError())
		} else {
			h.signal(credentialError("save"))
		}
		return
	}
	h.finish()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><html><body><h1>JEV connected</h1><p>The key was saved securely. You may close this tab.</p></body></html>")
	h.signal(nil)
}

func (h *connectHandler) finish() {
	h.mu.Lock()
	h.state = connectTerminal
	h.mu.Unlock()
}

// stopIfReady atomically closes a session only while no credential mutation
// has begun. Once processing starts, false means the caller must wait for the
// authoritative Store.Set result instead of reporting timeout or cancellation.
func (h *connectHandler) stopIfReady() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != connectReady {
		return false
	}
	h.state = connectTerminal
	return true
}

func (h *connectHandler) signal(err error) {
	select {
	case h.result <- err:
	default:
	}
}

func credentialError(action string) error {
	return app.NewError(app.CodeJEVCredentialUnavailable, app.KindUnavailable,
		"the operating-system credential store could not "+action+" the JEV key",
		"No key was exposed or written to repository files.",
		"Unlock or configure the operating-system credential service, then try again.")
}

func persistenceUnsupportedError() error {
	return app.NewError(app.CodeJEVPersistenceUnsupported, app.KindUnavailable,
		"Mindrail does not support secure persistent JEV credentials on this platform",
		"The browser connection page did not open and no JEV key was saved.",
		"Set TYPESAFE_API_KEY in the environment of the agent process when JEV routing is needed, or continue without JEV.")
}

func browserUnavailableError() error {
	return app.NewError(app.CodeJEVBrowserUnavailable, app.KindUnavailable,
		"Mindrail could not open the JEV connection page in the default browser",
		"No JEV key was saved.",
		"Configure a default browser, then run mindrail jev connect again.")
}

func connectUnavailableError(why, action string) error {
	return app.NewError(app.CodeJEVConnectUnavailable, app.KindUnavailable, why, "No JEV key was saved.", action)
}

func connectTimeoutError() error {
	return app.NewError(app.CodeJEVConnectTimeout, app.KindUnavailable,
		"The private JEV connection page timed out without saving a key",
		"No JEV key was saved.",
		"Run mindrail jev connect again when you are ready.")
}

func connectCancelledError() error {
	return app.NewError(app.CodeJEVConnectCancelled, app.KindUnavailable,
		"The JEV connection was cancelled before a key was saved",
		"No JEV key was saved.",
		"Run mindrail jev connect again when you are ready.")
}

func connectContextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return connectTimeoutError()
	}
	return connectCancelledError()
}
