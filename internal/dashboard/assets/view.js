(function (root, factory) {
  "use strict";
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  if (root) root.MindrailView = api;
})(typeof globalThis === "object" ? globalThis : this, function () {
  "use strict";

  const ACTIVITY = Object.freeze({
    active_signal: Object.freeze({ label: "ACTIVE SIGNAL", tone: "mint" }),
    claim_only: Object.freeze({ label: "CLAIM RECORDED", tone: "amber" }),
    lease_only: Object.freeze({ label: "LEASE ONLY", tone: "amber" }),
    history: Object.freeze({ label: "HISTORY", tone: "cyan" })
  });

  const PRESENCE = Object.freeze({
    CONNECTED: Object.freeze({ tone: "mint" }),
    IDLE: Object.freeze({ tone: "amber" }),
    STALE: Object.freeze({ tone: "red" }),
    ENDED: Object.freeze({ tone: "violet" })
  });

  function milliseconds(value) {
    const result = value instanceof Date ? value.getTime() : new Date(value).getTime();
    return Number.isFinite(result) ? result : null;
  }

  function duration(value) {
    let seconds = Math.max(0, Math.floor(Number(value)));
    if (!Number.isFinite(seconds)) return "—";
    const days = Math.floor(seconds / 86400); seconds %= 86400;
    const hours = Math.floor(seconds / 3600); seconds %= 3600;
    const minutes = Math.floor(seconds / 60); seconds %= 60;
    if (days) return `${days}d ${hours}h`;
    if (hours) return `${hours}h ${minutes}m`;
    if (minutes) return `${minutes}m ${seconds}s`;
    return `${seconds}s`;
  }

  function elapsed(value, now) {
    const start = milliseconds(value);
    const end = milliseconds(now);
    return start === null || end === null ? "—" : duration((end - start) / 1000);
  }

  function remaining(value, now) {
    const end = milliseconds(value);
    const start = milliseconds(now);
    if (start === null || end === null) return "—";
    if (end <= start) return "expired";
    return `${duration((end - start) / 1000)} remaining`;
  }

  function ageSeconds(value, now) {
    const observed = milliseconds(value);
    const current = milliseconds(now);
    if (observed === null || current === null) return null;
    return Math.max(0, (current - observed) / 1000);
  }

  function clientFamily(runtime) {
    return String(runtime.client_name || "").trim() || "unknown-client";
  }

  function runtimeCard(runtime, now) {
    runtime = runtime || {};
    const heartbeatAge = ageSeconds(runtime.last_heartbeat_at, now);
    const activityAge = ageSeconds(runtime.last_activity_at, now);
    let status;
    if (runtime.ended_at) status = "ENDED";
    else if (heartbeatAge === null || heartbeatAge > 15) status = "STALE";
    else if (activityAge !== null && activityAge <= 30) status = "CONNECTED";
    else status = "IDLE";
    const end = runtime.ended_at || now;
    const started = milliseconds(runtime.started_at);
    const ended = milliseconds(end);
    const connectedFor = started === null || ended === null ? "—" : duration((ended - started) / 1000);
    return Object.freeze({
      id: runtime.id || "—",
      taskID: runtime.task_id || "",
      sessionID: runtime.session_id || "",
      clientFamily: clientFamily(runtime),
      identitySource: "self-reported ClientInfo name, server-canonicalized",
      status,
      tone: PRESENCE[status].tone,
      connectedFor,
      lastHeartbeatAge: heartbeatAge === null ? "not recorded" : `${duration(heartbeatAge)} ago`,
      lastActivityAge: activityAge === null ? "not recorded" : `${duration(activityAge)} ago`,
      sequence: Number.isFinite(runtime.sequence) ? runtime.sequence : 0,
      endedAt: runtime.ended_at || null,
      endReason: runtime.end_reason || ""
    });
  }

  function runtime(snapshot, receivedAt, now) {
    const dashboard = snapshot && snapshot.dashboard || {};
    const received = milliseconds(receivedAt);
    const current = milliseconds(now);
    const extra = received === null || current === null ? 0 : Math.max(0, Math.floor((current - received) / 1000));
    const reported = Number(dashboard.uptime_seconds);
    const uptime = Number.isFinite(reported) ? duration(reported + extra) : elapsed(dashboard.started_at, now);
    return Object.freeze({
      startedAt: dashboard.started_at || null,
      uptime,
      snapshotAge: elapsed(snapshot && snapshot.generated_at, now)
    });
  }

  function connection(state, detail) {
    const states = {
      connecting: { mode: "", text: "DASHBOARD CONNECTING" },
      live: { mode: "live", text: "DASHBOARD LIVE / SSE" },
      reconnecting: { mode: "error", text: "DASHBOARD RECONNECTING" },
      snapshot_error: { mode: "error", text: "DASHBOARD SNAPSHOT ERROR" },
      frame_error: { mode: "error", text: `FRAME ${String(detail || "ERROR").slice(0, 54).toUpperCase()}` }
    };
    return Object.freeze(states[state] || states.connecting);
  }

  function sessionCard(agent, now) {
    agent = agent || {};
    const activity = ACTIVITY[agent.activity_status] || ACTIVITY.history;
    const tasks = Array.isArray(agent.current_tasks) ? agent.current_tasks : [];
    return Object.freeze({
      id: agent.id || "—",
      label: agent.label || "unlabelled session",
      status: agent.activity_status && ACTIVITY[agent.activity_status] ? agent.activity_status : "history",
      statusLabel: activity.label,
      tone: activity.tone,
      sessionAge: elapsed(agent.started_at, now),
      currentTaskCount: Number.isFinite(agent.current_task_count) ? agent.current_task_count : 0,
      activeLeaseCount: Number.isFinite(agent.lease_count) ? agent.lease_count : 0,
      tasks,
      tasksTruncated: agent.current_tasks_truncated === true,
      latestActivityAge: agent.latest_activity_at ? `${elapsed(agent.latest_activity_at, now)} ago` : "not recorded",
      latestLeaseRenewalAge: agent.latest_lease_renewed_at ? `${elapsed(agent.latest_lease_renewed_at, now)} ago` : "not recorded",
      nextLeaseExpiry: agent.next_lease_expires_at ? remaining(agent.next_lease_expires_at, now) : "none"
    });
  }

  return Object.freeze({ duration, elapsed, remaining, runtime, connection, sessionCard, runtimeCard });
});
