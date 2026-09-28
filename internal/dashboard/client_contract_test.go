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
		"Dashboard SSE health does not prove agent liveness",
		"Exact client, model, and process liveness are not recorded",
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
		"SOURCE: SELF-REPORTED CLIENTINFO NAME",
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
