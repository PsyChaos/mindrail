"use strict";

const assert = require("node:assert/strict");
const view = require("../assets/view.js");

const now = Date.parse("2026-09-28T12:00:00Z");

assert.equal(view.duration(0), "0s");
assert.equal(view.duration(65), "1m 5s");
assert.equal(view.duration(3661), "1h 1m");
assert.equal(view.duration(90061), "1d 1h");
assert.equal(view.duration(Number.NaN), "—");
assert.equal(view.elapsed("2026-09-28T11:58:55Z", now), "1m 5s");
assert.equal(view.elapsed("not-a-date", now), "—");

const runtime = view.runtime({
  generated_at: "2026-09-28T11:59:48Z",
  dashboard: { started_at: "2026-09-28T11:00:00Z", uptime_seconds: 3600 }
}, now - 5000, now);
assert.deepEqual(runtime, {
  startedAt: "2026-09-28T11:00:00Z",
  uptime: "1h 0m",
  snapshotAge: "12s"
});
assert.equal(view.runtime({ dashboard: { uptime_seconds: 3600 } }, now - 65000, now).uptime, "1h 1m");

assert.equal(view.remaining("2026-09-28T12:01:30Z", now), "1m 30s remaining");
assert.equal(view.remaining("2026-09-28T11:59:59Z", now), "expired");

assert.deepEqual(view.connection("connecting"), { mode: "", text: "DASHBOARD CONNECTING" });
assert.deepEqual(view.connection("live"), { mode: "live", text: "DASHBOARD LIVE / SSE" });
assert.deepEqual(view.connection("reconnecting"), { mode: "error", text: "DASHBOARD RECONNECTING" });
assert.deepEqual(view.connection("snapshot_error"), { mode: "error", text: "DASHBOARD SNAPSHOT ERROR" });
assert.deepEqual(view.connection("frame_error", "agents: broken"), { mode: "error", text: "FRAME AGENTS: BROKEN" });

const agent = view.sessionCard({
  id: "SES-123",
  label: "automatic: implement dashboard",
  started_at: "2026-09-28T10:30:00Z",
  activity_status: "active_signal",
  current_task_count: 2,
  lease_count: 1,
  current_tasks: [
    { id: "TSK-1", title: "Render activity", state: "IN_PROGRESS", updated_at: "2026-09-28T11:59:30Z" }
  ],
  current_tasks_truncated: true,
  latest_activity_at: "2026-09-28T11:59:30Z",
  latest_lease_renewed_at: "2026-09-28T11:59:00Z",
  next_lease_expires_at: "2026-09-28T12:02:00Z"
}, now);
assert.deepEqual(agent, {
  id: "SES-123",
  label: "automatic: implement dashboard",
  status: "active_signal",
  statusLabel: "ACTIVE SIGNAL",
  tone: "mint",
  sessionAge: "1h 30m",
  currentTaskCount: 2,
  activeLeaseCount: 1,
  tasks: [
    { id: "TSK-1", title: "Render activity", state: "IN_PROGRESS", updated_at: "2026-09-28T11:59:30Z" }
  ],
  tasksTruncated: true,
  latestActivityAge: "30s ago",
  latestLeaseRenewalAge: "1m 0s ago",
  nextLeaseExpiry: "2m 0s remaining"
});

const unknown = view.sessionCard({ id: "SES-OLD", activity_status: "invented" }, now);
assert.equal(unknown.status, "history");
assert.equal(unknown.statusLabel, "HISTORY");
assert.equal(unknown.currentTaskCount, 0);
assert.equal(unknown.activeLeaseCount, 0);
assert.equal(view.sessionCard({ activity_status: "claim_only" }, now).statusLabel, "CLAIM RECORDED");
assert.equal(view.sessionCard({ activity_status: "lease_only" }, now).statusLabel, "LEASE ONLY");

const connectedRuntime = {
  id: "RUN-1",
  task_id: "TSK-1",
  session_id: "SES-1",
  client_name: "claude-code",
  client_title: "Claude Code",
  client_version: "1.2.3",
  client_self_reported: true,
  started_at: "2026-09-28T11:00:00Z",
  last_heartbeat_at: "2026-09-28T11:59:45Z",
  last_activity_at: "2026-09-28T11:59:30Z",
  sequence: 9
};
assert.deepEqual(view.runtimeCard(connectedRuntime, now), {
  id: "RUN-1",
  taskID: "TSK-1",
  sessionID: "SES-1",
  clientFamily: "claude-code",
  identitySource: "self-reported ClientInfo name, server-canonicalized",
  status: "CONNECTED",
  tone: "mint",
  connectedFor: "1h 0m",
  lastHeartbeatAge: "15s ago",
  lastActivityAge: "30s ago",
  sequence: 9,
  endedAt: null,
  endReason: ""
});

// The public freshness contract is inclusive at 15s/30s. One millisecond
// beyond either boundary must change the browser presentation without waiting
// for another SSE frame.
assert.equal(view.runtimeCard({ ...connectedRuntime, last_activity_at: "2026-09-28T11:59:29.999Z" }, now).status, "IDLE");
assert.equal(view.runtimeCard({ ...connectedRuntime, last_heartbeat_at: "2026-09-28T11:59:44.999Z" }, now).status, "STALE");

const endedRuntime = view.runtimeCard({
  ...connectedRuntime,
  ended_at: "2026-09-28T11:20:00Z",
  end_reason: "disconnect"
}, now);
assert.equal(endedRuntime.status, "ENDED");
assert.equal(endedRuntime.tone, "violet");
assert.equal(endedRuntime.connectedFor, "20m 0s");
assert.equal(endedRuntime.endReason, "disconnect");

const anonymousRuntime = view.runtimeCard({
  started_at: "2026-09-28T11:59:00Z",
  last_heartbeat_at: "2026-09-28T12:00:00Z",
  last_activity_at: "2026-09-28T12:00:00Z"
}, now);
assert.equal(anonymousRuntime.clientFamily, "unknown-client");
assert.equal(anonymousRuntime.identitySource, "self-reported ClientInfo name, server-canonicalized");

// Raw caller-controlled title/version are never part of the presentation. The
// server-owned family is the only displayed identity, and runtime ID remains
// available to distinguish two unknown/custom clients.
const customRuntime = view.runtimeCard({
  ...connectedRuntime,
  id: "RUN-CUSTOM",
  client_name: "unknown-client",
  client_title: "credential-shaped-title",
  client_version: "secret-version"
}, now);
assert.equal(customRuntime.clientFamily, "unknown-client");
assert.equal(customRuntime.id, "RUN-CUSTOM");
assert.equal(JSON.stringify(customRuntime).includes("credential-shaped-title"), false);
assert.equal(JSON.stringify(customRuntime).includes("secret-version"), false);

process.stdout.write("client behavior assertions passed\n");
