package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/status"
	"github.com/PsyChaos/mindrail/internal/workspace"
)

func TestServerRequiresTokenHostAndSameOriginAndSetsSecurityHeaders(t *testing.T) {
	collector, err := NewCollector(CollectorOptions{DB: dashboardDB(t), ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo", Readiness: status.Report{}})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{collector: collector, token: "test-token", host: "127.0.0.1:49152"}
	handler := server.handler()
	prefix := "/t/" + server.token + "/"

	t.Run("valid snapshot", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://"+server.host+prefix+"api/snapshot", nil)
		req.Host = server.host
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		for name := range map[string]bool{"Content-Security-Policy": true, "Cache-Control": true, "X-Content-Type-Options": true, "Referrer-Policy": true} {
			if response.Header().Get(name) == "" {
				t.Errorf("missing %s", name)
			}
		}
		if response.Body.Len() > maxSnapshotBytes {
			t.Fatalf("snapshot=%d", response.Body.Len())
		}
	})
	t.Run("token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://"+server.host+"/api/snapshot", nil)
		req.Host = server.host
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status=%d", response.Code)
		}
	})
	t.Run("host", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://evil.test"+prefix, nil)
		req.Host = "evil.test"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status=%d", response.Code)
		}
	})
	t.Run("origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://"+server.host+prefix, nil)
		req.Host = server.host
		req.Header.Set("Origin", "https://evil.test")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status=%d", response.Code)
		}
	})
}

func TestServerSnapshotNeverExposesLegacyRawClientMetadata(t *testing.T) {
	db := dashboardDB(t)
	markers := []string{"legacy-name-keyring-marker", "legacy-title-typesafe-marker", "legacy-version-secret-marker"}
	t.Setenv("TYPESAFE_API_KEY", markers[0])
	mustExec(t, db, `INSERT INTO agent_runtimes VALUES ('RUN-LEGACY','PRJ-1','WS-1','TSK-1','SES-1',?,?,?,
		'2026-09-28T11:00:00Z','2026-09-28T11:00:00Z','2026-09-28T11:00:00Z',1,NULL,NULL)`, markers[0], markers[1], markers[2])
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{collector: collector, token: "test-token", host: "127.0.0.1:49152"}
	req := httptest.NewRequest(http.MethodGet, "http://"+server.host+"/t/test-token/api/snapshot", nil)
	req.Host = server.host
	response := httptest.NewRecorder()
	server.handler().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	payload := response.Body.String()
	if hits := markerCount(payload, markers); hits != 0 {
		t.Fatalf("REST snapshot contains %d/3 legacy markers: %s", hits, payload)
	}
	for _, retired := range []string{`"client_title"`, `"client_version"`} {
		if strings.Contains(payload, retired) {
			t.Fatalf("REST snapshot includes retired key %s", retired)
		}
	}
}

func TestServerCancelsOpenEventStreamOnShutdown(t *testing.T) {
	collector, err := NewCollector(CollectorOptions{DB: dashboardDB(t), ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo", Readiness: status.Report{}})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{collector: collector, streams: make(chan struct{}, maxStreams)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	response := &flushRecorder{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{}, 1)}
	request := httptest.NewRequest(http.MethodGet, "http://localhost/events", nil).WithContext(ctx)
	go func() { server.events(response, request); done <- nil }()
	<-response.flushed
	cancel()
	<-done
}

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
}

func (r *flushRecorder) Flush() {
	r.ResponseRecorder.Flush()
	select {
	case r.flushed <- struct{}{}:
	default:
	}
}

func TestServerRejectsEventStreamsAboveLimit(t *testing.T) {
	collector, err := NewCollector(CollectorOptions{DB: dashboardDB(t), ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo", Readiness: status.Report{}})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{collector: collector, token: "test-token", host: "127.0.0.1:49152", streams: make(chan struct{}, maxStreams)}
	for range maxStreams {
		server.streams <- struct{}{}
	}
	req := httptest.NewRequest(http.MethodGet, "http://"+server.host+"/t/test-token/events", nil)
	req.Host = server.host
	response := httptest.NewRecorder()
	server.handler().ServeHTTP(response, req)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestEmbeddedClientAvoidsDynamicHTMLInjection(t *testing.T) {
	body, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "innerHTML") || strings.Contains(string(body), "insertAdjacentHTML") {
		t.Fatal("client uses an HTML injection sink")
	}
	if !strings.Contains(string(body), "textContent") {
		t.Fatal("client does not use text-only rendering")
	}
}
