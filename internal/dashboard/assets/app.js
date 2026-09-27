(() => {
  "use strict";
  const $ = (selector) => document.querySelector(selector);
  const view = window.MindrailView;
  const STATES = ["OPEN", "CLAIMED", "IN_PROGRESS", "BLOCKED", "READY_TO_COMPLETE", "COMPLETED", "ABANDONED"];
  const TONES = { OPEN:"cyan", CLAIMED:"violet", IN_PROGRESS:"cyan", BLOCKED:"amber", READY_TO_COMPLETE:"mint", COMPLETED:"mint", ABANDONED:"red", active:"mint", expired:"amber", released:"violet", pass:"mint", fail:"red", timeout:"amber", error:"red" };
  const fmt = new Intl.DateTimeFormat(undefined, {month:"short",day:"2-digit",hour:"2-digit",minute:"2-digit",second:"2-digit"});
  let current = null;
  let lastSnapshotReceivedAt = null;
  let renderStage = "BOOT";
  const observedEventKeys = new Set();
  let observedEvents = [];

  function node(tag, className, text) {
    const el = document.createElement(tag);
    if (className) el.className = className;
    if (text !== undefined) el.textContent = text;
    return el;
  }
  function empty(target) { target.replaceChildren(document.importNode($("#empty-template").content, true)); }
  function short(value, size = 14) { return value && value.length > size ? value.slice(0, size) + "…" : (value || "—"); }
  function instant(value) { const date=value && new Date(value); return date && !Number.isNaN(date.getTime()) ? fmt.format(date) : "—"; }
  function tone(value) { return TONES[value] || "cyan"; }

  function refreshClock() {
    if (!current) return;
    const runtime=view.runtime(current,lastSnapshotReceivedAt,Date.now());
    $("#dashboard-start").textContent=instant(runtime.startedAt);
    $("#dashboard-uptime").textContent=runtime.uptime;
    $("#last-frame").textContent=current.generated_at ? `${runtime.snapshotAge} AGO` : "—";
    $("#last-frame").title=instant(current.generated_at);
    document.querySelectorAll("[data-elapsed-from]").forEach(el=>{el.textContent=view.elapsed(el.dataset.elapsedFrom,Date.now());});
  }

  document.querySelectorAll(".tab").forEach(tab => tab.addEventListener("click", () => {
    document.querySelectorAll(".tab,.view").forEach(el => el.classList.remove("active"));
    tab.classList.add("active");
    $("#" + tab.dataset.view).classList.add("active");
  }));

  function render(snapshot) {
    current = snapshot;
    lastSnapshotReceivedAt = Date.now();
    $("#project-name").textContent = snapshot.project.name || "Mindrail Project";
    $("#project-meta").textContent = `${snapshot.project.id} · ${snapshot.project.workspace_id} · ${snapshot.project.linked_worktree ? "LINKED WORKTREE" : "PRIMARY WORKTREE"}`;
    $("#sequence").textContent = `SEQ ${String(snapshot.sequence).padStart(4,"0")}`;
    renderStage="SUMMARY"; renderSummary(snapshot);
    renderStage="FACTORY"; renderFactory(snapshot);
    renderStage="AGENTS"; renderAgents(snapshot);
    renderStage="EVENTS"; renderEvents(snapshot);
    renderStage="DONE";
    refreshClock();
  }

  function renderSummary(s) {
    const metrics = [
      ["OPEN", s.summary.states.OPEN || 0, "cyan"],
      ["IN PROGRESS", s.summary.states.IN_PROGRESS || 0, "cyan"],
      ["BLOCKED", s.summary.states.BLOCKED || 0, "amber"],
      ["READY", s.summary.states.READY_TO_COMPLETE || 0, "mint"],
      ["ACTIVE LEASES", s.summary.active_leases, "violet"],
      ["AGENTS ENGAGED", s.summary.agents_engaged, "mint"],
      ["EVIDENCE PASS", s.summary.evidence_pass, "mint"],
      ["EVIDENCE FAIL", s.summary.evidence_fail, s.summary.evidence_fail ? "red" : "cyan"]
    ];
    const target = $("#summary"); target.replaceChildren();
    const max = Math.max(1, ...metrics.map(x => x[1]));
    metrics.forEach(([label,value,toneName]) => {
      const el = node("div","metric"); el.dataset.tone = toneName; el.style.setProperty("--level", `${Math.max(4,value/max*100)}%`);
      el.append(node("span","metric-label",label), node("strong","",String(value))); target.append(el);
    });
  }

  function renderFactory(s) {
    const readiness = s.readiness || {};
    $("#readiness-badge").textContent = readiness.readiness || "UNKNOWN";
    $("#readiness-badge").dataset.tone = readiness.readiness === "READY" ? "mint" : "amber";
    const components = $("#readiness-components"); components.replaceChildren();
    Object.entries(readiness.components || {}).forEach(([name,value]) => {
      const el = node("div","component");
      el.append(node("span","component-name",name.replaceAll("_"," ")), node("span","component-state",value.state || "UNKNOWN"), node("span","component-summary",value.summary || "No detail"));
      components.append(el);
    });
    if (!components.children.length) empty(components);
    const telemetry = $("#telemetry"); telemetry.replaceChildren();
    const jev=s.jev || {};
    const jevState=jev.configured ? `${jev.provider || "typesafe"} / ${jev.source}` : jev.source === "unavailable" ? "unavailable" : "not configured";
    [
      ["JEV", jevState],
      ["JEV FRESHNESS", "startup snapshot · restart after change"],
      ["CI", `${s.ci.status} / ${s.ci.source || "local"}`],
      ["MERGE", s.merge.status],
      ["GATE", s.completion.status],
      ["TRANSPORT", s.capabilities.live_transport.toUpperCase()],
      ["READINESS", `${s.capabilities.readiness_freshness.replaceAll("_"," ")} · restart to refresh`],
      ["DETAIL LIMIT", Object.keys(s.truncated || {}).length ? Object.keys(s.truncated).join(", ") : "none"],
      ["MODE", "READ ONLY"]
    ].forEach(([key,value]) => { const row=node("div","telemetry-row"); row.append(node("span","",key),node("span","",value)); telemetry.append(row); });

    const board = $("#task-board"); board.replaceChildren();
    STATES.forEach(state => {
      const col = node("section","board-column"); const tasks = s.tasks.filter(t => t.state === state);
      const head = node("div","board-title"); head.append(node("span","",state.replaceAll("_"," ")),node("b","",String(tasks.length))); col.append(head);
      tasks.forEach(task => col.append(taskCard(task)));
      if (!tasks.length) col.append(node("div","empty","QUIET")); board.append(col);
    });
  }

  function taskCard(task) {
    const el=node("article","task-card"); el.dataset.tone=tone(task.state);
    const head=node("div","task-card-head"); head.append(node("b","",short(task.id,13)),node("span","",`REV ${task.revision}`));
    const meta=node("div","task-meta");
    [["OWNER",task.claimed_by || "unclaimed"],["OPENED",task.opened_by],["UPDATED",instant(task.updated_at)]].forEach(([k,v])=>{const row=node("div");row.append(node("span","",k),node("i","",short(v,16)));meta.append(row);});
    el.append(head,node("div","task-title",task.title),meta);
    if(task.blocked_reason) el.append(node("div","blocked-note",task.blocked_reason));
    return el;
  }

  function renderAgents(s) {
    $("#session-count").textContent=String(s.sessions.length); $("#lease-count").textContent=String(s.leases.length);
    const grid=$("#agent-grid"); grid.replaceChildren();
    s.sessions.forEach(agent=>{
      const presentation=view.sessionCard(agent,Date.now());
      const card=node("article","agent-card"+(presentation.status==="active_signal"?" engaged":""));
      card.dataset.tone=presentation.tone;
      const header=node("div","agent-card-head");
      header.append(node("div","agent-label",presentation.label),node("span","agent-status",presentation.statusLabel));
      card.append(header,node("div","agent-name",presentation.id));
      const stats=node("div","agent-stats");
      const sessionAge=node("b","",presentation.sessionAge); sessionAge.dataset.elapsedFrom=agent.started_at || "";
      [[presentation.currentTaskCount,"CURRENT TASKS"],[presentation.activeLeaseCount,"ACTIVE LEASES"]].forEach(([v,k])=>{const box=node("div");box.append(node("b","",String(v)),node("span","",k));stats.append(box);});
      const ageBox=node("div"); ageBox.append(sessionAge,node("span","","SESSION AGE")); stats.append(ageBox);
      card.append(stats);
      const tasks=node("div","agent-tasks");
      presentation.tasks.forEach(task=>{const row=node("div","agent-task");row.append(node("span","",short(task.id,12)),node("b","",(task.state||"UNKNOWN").replaceAll("_"," ")),node("small","",task.title));tasks.append(row);});
      if(presentation.tasksTruncated) tasks.append(node("div","projection-note",`More claimed tasks omitted; exact total ${presentation.currentTaskCount}.`));
      if(!presentation.tasks.length) tasks.append(node("div","projection-note","No current claimed task recorded."));
      card.append(tasks);
      const activity=node("dl","agent-activity");
      const activityRows=[
        ["SESSION START",instant(agent.started_at)],
        ["LATEST DURABLE ACTIVITY",agent.latest_activity_at ? `${presentation.latestActivityAge} · ${instant(agent.latest_activity_at)}` : presentation.latestActivityAge],
        ["LATEST LEASE RENEWAL",agent.latest_lease_renewed_at ? `${presentation.latestLeaseRenewalAge} · ${instant(agent.latest_lease_renewed_at)}` : presentation.latestLeaseRenewalAge],
        ["NEXT LEASE EXPIRY",agent.next_lease_expires_at ? `${presentation.nextLeaseExpiry} · ${instant(agent.next_lease_expires_at)}` : presentation.nextLeaseExpiry]
      ];
      activityRows.forEach(([key,value])=>{activity.append(node("dt","",key),node("dd","",value));});
      card.append(activity); grid.append(card);
    }); if(!s.sessions.length) empty(grid);
    const list=$("#lease-list"); list.replaceChildren();
    s.leases.forEach(lease=>{
      const row=node("div","data-row"); row.dataset.tone=tone(lease.status);
      const timing=lease.status==="released" ? `released ${instant(lease.released_at)}` : `renewed ${instant(lease.renewed_at)} · expires ${instant(lease.expires_at)}`;
      row.append(node("strong","",`${lease.target_kind} / ${lease.target_key}`),node("span","status",lease.status),node("small","",`${short(lease.holder,18)} · ${lease.release_reason||"held"}`),node("time","",timing)); list.append(row);
    }); if(!s.leases.length) empty(list);
  }

  function renderEvents(s) {
    const candidates=[];
    (s.tasks||[]).forEach(t=>candidates.push({key:`task:${t.id}:${t.revision}`,at:t.updated_at,kind:"TASK REVISION",tone:tone(t.state),title:`${t.id} · REV ${t.revision}`,detail:`${t.state} · ${t.title}`}));
    (s.checkpoints||[]).forEach(c=>candidates.push({key:`checkpoint:${c.id}`,at:c.created_at,kind:c.handoff?"HANDOFF":"CHECKPOINT",tone:c.handoff?"violet":"cyan",title:`${c.task_id} · ${c.session_id}`,detail:"checkpoint metadata · note withheld"}));
    (s.leases||[]).forEach(l=>{const at=l.status==="released"?l.released_at:l.status==="expired"?l.expires_at:(l.renewed_at||l.acquired_at);const renewed=l.status==="active"&&l.renewed_at&&l.renewed_at!==l.acquired_at;candidates.push({key:`lease:${l.id}:${l.status}:${at}`,at,kind:renewed?"LEASE RENEWED":`LEASE ${l.status.toUpperCase()}`,tone:tone(l.status),title:`${l.target_kind} · ${l.target_key}`,detail:`holder ${l.holder}${l.release_reason?" · "+l.release_reason:""}`});});
    (s.sessions||[]).forEach(a=>candidates.push({key:`session:${a.id}`,at:a.started_at,kind:"SESSION START",tone:a.engaged?"mint":"cyan",title:a.id,detail:a.label||"Unlabelled agent session"}));
    candidates.forEach(item=>{if(!observedEventKeys.has(item.key)){observedEventKeys.add(item.key);observedEvents.push(item);}});
    observedEvents.sort((a,b)=>new Date(b.at)-new Date(a.at));
    if(observedEvents.length>300) observedEvents=observedEvents.slice(0,300);
    observedEventKeys.clear(); observedEvents.forEach(item=>observedEventKeys.add(item.key));
    $("#event-count").textContent=String(observedEvents.length); const list=$("#event-list");list.replaceChildren();
    observedEvents.forEach(item=>{const el=node("article","event");el.dataset.tone=item.tone;const head=node("div","event-head");head.append(node("b","",item.kind),node("time","",instant(item.at)));el.append(head,node("p","",`${item.title} — ${item.detail}`));list.append(el);}); if(!observedEvents.length)empty(list);
    $("#evidence-count").textContent=String(s.evidence.length); const evidence=$("#evidence-list");evidence.replaceChildren();
    (s.evidence||[]).forEach(item=>{const row=node("div","data-row");row.dataset.tone=tone(item.status);row.append(node("strong","",`${item.profile} / ${item.type}`),node("span","status",item.status),node("small","",`snapshot ${item.snapshot_hash} · exit ${item.exit_code}`),node("time","",instant(item.created_at)));evidence.append(row);}); if(!s.evidence || !s.evidence.length)empty(evidence);
    $("#profile-count").textContent=String((s.profiles||[]).length); const profiles=$("#profile-list");profiles.replaceChildren();
    (s.profiles||[]).forEach(item=>{const row=node("div","data-row");row.dataset.tone=tone(item.latest_status);row.append(node("strong","",`${item.name} / ${item.type}`),node("span","status",item.latest_status),node("small","",`${item.scope_count} scopes · ${item.command_count} commands`),node("time","",instant(item.latest_at)));profiles.append(row);}); if(!s.profiles || !s.profiles.length)empty(profiles);
  }

  function connection(state, detail) { const status=view.connection(state,detail);const pulse=document.querySelector(".pulse");pulse.className="pulse "+status.mode;$("#connection").textContent=status.text; }
  fetch("api/snapshot",{cache:"no-store"}).then(r=>{if(!r.ok)throw new Error();return r.json();}).then(render).catch(()=>connection("snapshot_error"));
  const stream=new EventSource("events");
  stream.addEventListener("open",()=>connection("live"));
  stream.addEventListener("snapshot",event=>{try{render(JSON.parse(event.data));connection("live");}catch(error){connection("frame_error",`${renderStage}: ${String(error && error.message || "ERROR")}`);}});
  stream.addEventListener("error",()=>connection("reconnecting"));
  window.setInterval(refreshClock,1000);
})();
