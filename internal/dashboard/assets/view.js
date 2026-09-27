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

  return Object.freeze({ duration, elapsed, remaining, runtime, connection, sessionCard });
});
