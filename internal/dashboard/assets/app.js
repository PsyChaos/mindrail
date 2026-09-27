(() => {
  "use strict";
  const $ = (selector) => document.querySelector(selector);
  const STATES = ["OPEN", "CLAIMED", "IN_PROGRESS", "BLOCKED", "READY_TO_COMPLETE", "COMPLETED", "ABANDONED"];
  const TONES = { OPEN:"cyan", CLAIMED:"violet", IN_PROGRESS:"cyan", BLOCKED:"amber", READY_TO_COMPLETE:"mint", COMPLETED:"mint", ABANDONED:"red", active:"mint", expired:"amber", released:"violet", pass:"mint", fail:"red", timeout:"amber", error:"red" };
  const fmt = new Intl.DateTimeFormat(undefined, {month:"short",day:"2-digit",hour:"2-digit",minute:"2-digit",second:"2-digit"});
  let current = null;
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
  function age(value) { return value ? fmt.format(new Date(value)) : "—"; }
  function tone(value) { return TONES[value] || "cyan"; }

  document.querySelectorAll(".tab").forEach(tab => tab.addEventListener("click", () => {
    document.querySelectorAll(".tab,.view").forEach(el => el.classList.remove("active"));
    tab.classList.add("active");
    $("#" + tab.dataset.view).classList.add("active");
  }));

  function render(snapshot) {
    current = snapshot;
    $("#project-name").textContent = snapshot.project.name || "Mindrail Project";
    $("#project-meta").textContent = `${snapshot.project.id} · ${snapshot.project.workspace_id} · ${snapshot.project.linked_worktree ? "LINKED WORKTREE" : "PRIMARY WORKTREE"}`;
    $("#last-frame").textContent = age(snapshot.generated_at);
    $("#sequence").textContent = `SEQ ${String(snapshot.sequence).padStart(4,"0")}`;
    renderStage="SUMMARY"; renderSummary(snapshot);
    renderStage="FACTORY"; renderFactory(snapshot);
    renderStage="AGENTS"; renderAgents(snapshot);
    renderStage="EVENTS"; renderEvents(snapshot);
    renderStage="DONE";
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
    [
      ["JEV", s.jev.configured ? `${s.jev.provider} / ${s.jev.source}` : "not configured"],
      ["CI", `${s.ci.status} / ${s.ci.source || "local"}`],
      ["MERGE", s.merge.status],
      ["GATE", s.completion.status],
      ["TRANSPORT", s.capabilities.live_transport.toUpperCase()],
      ["READINESS", s.capabilities.readiness_freshness.replaceAll("_"," ")],
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
    [["OWNER",task.claimed_by || "unclaimed"],["OPENED",task.opened_by],["UPDATED",age(task.updated_at)]].forEach(([k,v])=>{const row=node("div");row.append(node("span","",k),node("i","",short(v,16)));meta.append(row);});
    el.append(head,node("div","task-title",task.title),meta);
    if(task.blocked_reason) el.append(node("div","blocked-note",task.blocked_reason));
    return el;
  }

  function renderAgents(s) {
    $("#session-count").textContent=String(s.sessions.length); $("#lease-count").textContent=String(s.leases.length);
    const grid=$("#agent-grid"); grid.replaceChildren();
    s.sessions.forEach(agent=>{
      const card=node("article","agent-card"+(agent.engaged?" engaged":""));
      card.append(node("div","agent-name",agent.id),node("div","agent-label",agent.label||"unlabelled session"));
      const stats=node("div","agent-stats");
      [[agent.task_count,"TASK LINKS"],[agent.lease_count,"LEASES"],[age(agent.started_at),"STARTED"]].forEach(([v,k])=>{const box=node("div");box.append(node("b","",String(v)),node("span","",k));stats.append(box);});
      card.append(stats); grid.append(card);
    }); if(!s.sessions.length) empty(grid);
    const list=$("#lease-list"); list.replaceChildren();
    s.leases.forEach(lease=>{
      const row=node("div","data-row"); row.dataset.tone=tone(lease.status);
      row.append(node("strong","",`${lease.target_kind} / ${lease.target_key}`),node("span","status",lease.status),node("small","",`${short(lease.holder,18)} · ${lease.release_reason||"held"}`),node("time","",age(lease.status==="released"?lease.released_at:lease.expires_at))); list.append(row);
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
    observedEvents.forEach(item=>{const el=node("article","event");el.dataset.tone=item.tone;const head=node("div","event-head");head.append(node("b","",item.kind),node("time","",age(item.at)));el.append(head,node("p","",`${item.title} — ${item.detail}`));list.append(el);}); if(!observedEvents.length)empty(list);
    $("#evidence-count").textContent=String(s.evidence.length); const evidence=$("#evidence-list");evidence.replaceChildren();
    (s.evidence||[]).forEach(item=>{const row=node("div","data-row");row.dataset.tone=tone(item.status);row.append(node("strong","",`${item.profile} / ${item.type}`),node("span","status",item.status),node("small","",`snapshot ${item.snapshot_hash} · exit ${item.exit_code}`),node("time","",age(item.created_at)));evidence.append(row);}); if(!s.evidence || !s.evidence.length)empty(evidence);
    $("#profile-count").textContent=String((s.profiles||[]).length); const profiles=$("#profile-list");profiles.replaceChildren();
    (s.profiles||[]).forEach(item=>{const row=node("div","data-row");row.dataset.tone=tone(item.latest_status);row.append(node("strong","",`${item.name} / ${item.type}`),node("span","status",item.latest_status),node("small","",`${item.scope_count} scopes · ${item.command_count} commands`),node("time","",age(item.latest_at)));profiles.append(row);}); if(!s.profiles || !s.profiles.length)empty(profiles);
  }

  function connection(mode, text) { const pulse=document.querySelector(".pulse");pulse.className="pulse "+mode;$("#connection").textContent=text; }
  fetch("api/snapshot",{cache:"no-store"}).then(r=>{if(!r.ok)throw new Error();return r.json();}).then(render).catch(()=>connection("error","SNAPSHOT ERROR"));
  const stream=new EventSource("events");
  stream.addEventListener("open",()=>connection("live","LIVE / SSE"));
  stream.addEventListener("snapshot",event=>{try{render(JSON.parse(event.data));connection("live","LIVE / SSE");}catch(error){connection("error",`FRAME ${renderStage}: ${String(error && error.message || "ERROR").slice(0,54).toUpperCase()}`);}});
  stream.addEventListener("error",()=>connection("error","RECONNECTING"));
})();
