package dashboard

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/status"
	"github.com/PsyChaos/mindrail/internal/validation"
	"github.com/PsyChaos/mindrail/internal/workspace"
)

const (
	maxSnapshotBytes = 256 << 10
	streamInterval   = 1500 * time.Millisecond
	maxStreams       = 8
)

type Server struct {
	collector *Collector
	listener  net.Listener
	token     string
	host      string
	http      *http.Server
	streams   chan struct{}
}

type RunOptions struct {
	DB             *sql.DB
	ProjectID      string
	ProjectName    string
	WorkspaceID    string
	WorktreeRoot   string
	LinkedWorktree bool
	Profiles       map[string]config.ValidationProfile
	Readiness      status.Report
	JEV            JEVState
	SecretNames    []string
	Port           int
	Output         io.Writer
}

// Run constructs and serves one dashboard from already-bootstrapped read-only
// state. It is called through the cmd/mindrail injection seam so net/http never
// enters the ordinary CLI and init dependency graph.
func Run(ctx context.Context, opts RunOptions) error {
	secretNames := append([]string(nil), opts.SecretNames...)
	secretNames = append(secretNames, "TYPESAFE_API_KEY")
	redactor, err := validation.NewRedactor(secretNames)
	if err != nil {
		return err
	}
	collector, err := NewCollector(CollectorOptions{
		DB: opts.DB, ProjectID: opts.ProjectID, ProjectName: opts.ProjectName,
		Workspace:    workspace.Workspace{ID: opts.WorkspaceID, ProjectID: opts.ProjectID, IsLinkedWorktree: opts.LinkedWorktree},
		WorktreeRoot: opts.WorktreeRoot, Profiles: opts.Profiles, Readiness: opts.Readiness, JEV: opts.JEV,
		StartedAt: time.Now().UTC(), Redact: redactor.Redact,
	})
	if err != nil {
		return err
	}
	server, err := NewServer(collector, opts.Port)
	if err != nil {
		return err
	}
	if opts.Output == nil {
		return errors.New("dashboard: output writer is required")
	}
	if _, err := fmt.Fprintf(opts.Output, "Mindrail dashboard: %s\nRead-only · loopback only · press Ctrl-C to stop\n", server.URL()); err != nil {
		return err
	}
	return server.Serve(ctx)
}

func NewServer(collector *Collector, port int) (*Server, error) {
	if collector == nil {
		return nil, errors.New("dashboard: collector is required")
	}
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("dashboard: port must be between 0 and 65535")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, fmt.Errorf("dashboard: listen on loopback: %w", err)
	}
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("dashboard: create access token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	s := &Server{collector: collector, listener: listener, token: token, host: listener.Addr().String(), streams: make(chan struct{}, maxStreams)}
	s.http = &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	return s, nil
}

func (s *Server) URL() string { return "http://" + s.host + "/t/" + s.token + "/" }

func (s *Server) Serve(ctx context.Context) error {
	s.http.BaseContext = func(net.Listener) context.Context { return ctx }
	done := make(chan error, 1)
	go func() { done <- s.http.Serve(s.listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.http.Shutdown(shutdownCtx); err != nil {
			if closeErr := s.http.Close(); closeErr != nil {
				return errors.Join(err, closeErr)
			}
		}
		return nil
	}
}

func (s *Server) handler() http.Handler {
	assetFS, _ := fs.Sub(assets, "assets")
	files := http.FileServer(http.FS(assetFS))
	prefix := "/t/" + s.token + "/"
	mux := http.NewServeMux()
	mux.Handle(prefix+"assets/", http.StripPrefix(prefix+"assets/", files))
	mux.HandleFunc(prefix+"api/snapshot", s.snapshot)
	mux.HandleFunc(prefix+"events", s.events)
	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != prefix {
			http.NotFound(w, r)
			return
		}
		http.ServeFileFS(w, r, assetFS, "index.html")
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != s.host {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" && origin != "http://"+s.host {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		securityHeaders(w.Header())
		mux.ServeHTTP(w, r)
	})
}

func securityHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store, max-age=0")
	header.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	header.Set("Cross-Origin-Opener-Policy", "same-origin")
	header.Set("Cross-Origin-Resource-Policy", "same-origin")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}

func (s *Server) snapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload, err := s.payload(r.Context())
	if err != nil {
		http.Error(w, "snapshot unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(payload)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	select {
	case s.streams <- struct{}{}:
		defer func() { <-s.streams }()
	default:
		http.Error(w, "too many streams", http.StatusTooManyRequests)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func() bool {
		payload, err := s.payload(r.Context())
		if err != nil {
			if _, writeErr := fmt.Fprintf(w, "event: error\ndata: {\"error\":\"snapshot unavailable\"}\n\n"); writeErr != nil {
				return false
			}
			flusher.Flush()
			return true
		}
		if _, err := fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send() {
		return
	}
	ticker := time.NewTicker(streamInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}

func (s *Server) payload(ctx context.Context) ([]byte, error) {
	snapshot, err := s.collector.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	for {
		payload, marshalErr := json.Marshal(snapshot)
		if marshalErr != nil {
			return nil, marshalErr
		}
		if len(payload) <= maxSnapshotBytes {
			return payload, nil
		}
		kind := largestDetail(snapshot)
		if kind == "" {
			return nil, fmt.Errorf("dashboard: fixed snapshot metadata exceeds %d bytes", maxSnapshotBytes)
		}
		if snapshot.Truncated == nil {
			snapshot.Truncated = map[string]bool{}
		}
		snapshot.Truncated[kind] = true
		switch kind {
		case "tasks":
			snapshot.Tasks = snapshot.Tasks[:len(snapshot.Tasks)/2]
		case "sessions":
			snapshot.Sessions = snapshot.Sessions[:len(snapshot.Sessions)/2]
		case "leases":
			snapshot.Leases = snapshot.Leases[:len(snapshot.Leases)/2]
		case "checkpoints":
			snapshot.Checkpoints = snapshot.Checkpoints[:len(snapshot.Checkpoints)/2]
		case "evidence":
			snapshot.Evidence = snapshot.Evidence[:len(snapshot.Evidence)/2]
		}
	}
}

func largestDetail(snapshot Snapshot) string {
	kind, size := "", 0
	for _, detail := range []struct {
		name   string
		length int
	}{{"tasks", len(snapshot.Tasks)}, {"sessions", len(snapshot.Sessions)}, {"leases", len(snapshot.Leases)}, {"checkpoints", len(snapshot.Checkpoints)}, {"evidence", len(snapshot.Evidence)}} {
		if detail.length > size {
			kind, size = detail.name, detail.length
		}
	}
	return kind
}
