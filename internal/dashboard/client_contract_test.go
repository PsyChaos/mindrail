package dashboard

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEmbeddedClientExplainsLiveDashboardSignalsConservatively(t *testing.T) {
	htmlBytes, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	jsBytes, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	viewBytes, err := assets.ReadFile("assets/view.js")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlBytes)
	js := string(viewBytes) + string(jsBytes)

	for _, want := range []string{
		"DASHBOARD START",
		"UPTIME",
		"LAST SNAPSHOT",
		"Connection history, not model telemetry",
		"Heartbeat shows when MCP presence was last recorded",
		"STARTUP SNAPSHOT · RESTART DASHBOARD TO REFRESH",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("embedded HTML is missing %q", want)
		}
	}
	viewScript := strings.Index(html, `src="assets/view.js"`)
	appScript := strings.Index(html, `src="assets/app.js"`)
	if viewScript < 0 || appScript < 0 || viewScript >= appScript {
		t.Error("pure view helpers must load before the DOM client")
	}
	if got := strings.Count(html, `class="metric"`); got != 8 {
		t.Errorf("loading shell metrics = %d, want 8 to prevent layout shift", got)
	}

	for _, want := range []string{
		"DASHBOARD LIVE / SSE",
		"dashboard.started_at",
		"dashboard.uptime_seconds",
		"current_task_count",
		"current_tasks_truncated",
		"latest_activity_at",
		"latest_lease_renewed_at",
		"next_lease_expires_at",
		"ACTIVE SIGNAL",
		"startup snapshot · restart after change",
		"restart to refresh",
		"unavailable",
		"JEV CONFIGURED",
		"JEV ACTUAL USE",
		"CANONICAL CLIENT FAMILY",
		"CONNECTED FOR",
		"LAST HEARTBEAT",
		"LAST MCP ACTIVITY",
		"runtimeCard",
		"agent_runtimes",
		"jev_route_events",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("embedded JavaScript is missing %q", want)
		}
	}
}

func TestEmbeddedDashboardPinsTheAppShellAndScrollsPanels(t *testing.T) {
	cssBytes, err := assets.ReadFile("assets/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssBytes)
	for _, want := range []string{
		"html, body { margin: 0; height: 100%; overflow: hidden",
		"grid-template-rows: auto auto minmax(0,1fr)",
		".board-wrap { height: 100%; min-height: 0; overflow: auto",
		".agent-grid { min-height:0; flex:1 1 auto",
		".data-list { min-height:0; flex:1 1 0; overflow-y:auto",
		".timeline { min-height:0; flex:1 1 auto; overflow-y:auto",
		"overscroll-behavior:contain",
		"@media (max-height: 700px)",
		"@media (min-width: 681px) and (max-width: 1100px) and (max-height: 700px)",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("embedded CSS is missing viewport/panel contract %q", want)
		}
	}
}

func TestEmbeddedClientNeverPromisesOrProjectsRawClientTitleVersion(t *testing.T) {
	viewBytes, err := assets.ReadFile("assets/view.js")
	if err != nil {
		t.Fatal(err)
	}
	appBytes, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	client := string(viewBytes) + string(appBytes)
	for _, forbidden := range []string{"client_title", "client_version", "SELF-REPORTED CLIENT ·", "name/version"} {
		if strings.Contains(client, forbidden) {
			t.Errorf("embedded client retained raw identity promise %q", forbidden)
		}
	}
}

func TestOperatorDocsDescribeCanonicalClientFamilySecurityAmendment(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate dashboard package")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	for _, relative := range []string{"README.md", "docs/usage-tr.md", "docs/engineering/automatic-jev-and-agent-presence-2026-09-28.md"} {
		body, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, required := range []string{"canonical client family", "unknown-client", "raw title/version"} {
			if !strings.Contains(strings.ToLower(text), strings.ToLower(required)) {
				t.Errorf("%s missing canonical identity disclosure %q", relative, required)
			}
		}
	}
}

func TestEmbeddedClientKeepsOperationalTextOutOfHTMLInjectionSinks(t *testing.T) {
	for _, asset := range []string{"assets/view.js", "assets/app.js"} {
		body, err := assets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		client := string(body)
		for _, sink := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write"} {
			if strings.Contains(client, sink) {
				t.Errorf("%s uses HTML injection sink %q", asset, sink)
			}
		}
	}
}

func TestEmbeddedClientBehaviorWithFixedClock(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is an optional developer dependency; dashboard runtime remains Node-free")
	}
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate client behavior test")
	}
	directory := filepath.Dir(filename)
	command := exec.Command(node, filepath.Join(directory, "testdata", "client_behavior_test.cjs"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("client behavior test failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "client behavior assertions passed") {
		t.Fatalf("client behavior test did not report completion: %s", output)
	}
}
