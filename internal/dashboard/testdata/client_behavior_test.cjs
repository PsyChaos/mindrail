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

process.stdout.write("client behavior assertions passed\n");
