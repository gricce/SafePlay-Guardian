(async () => {
  const H = window.Hearth;
  await H.requireAuth();

  const personID = new URLSearchParams(location.search).get("id");
  if (!personID) {
    document.querySelector("main.main").innerHTML =
      '<div class="empty-block">No person selected. <a href="/home">Back to home →</a></div>';
    return;
  }

  // ---- formatting helpers ----
  const dayNames = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  const dayLetters = ["S", "M", "T", "W", "T", "F", "S"];

  function fmtDateShort(iso) {
    if (!iso) return "—";
    return new Date(iso).toLocaleDateString(undefined,
      { month: "short", day: "numeric", year: "numeric" });
  }

  function presenceForDevice(d) {
    if (!d) return "—";
    if (d.state === "paused") return "manually paused";
    if (!d.last_seen) return "never seen";
    const m = Math.round((Date.now() - new Date(d.last_seen).getTime()) / 60000);
    if (m < 5) return "in use now";
    if (m < 60) return `last seen ${m}m ago`;
    return `last seen ${Math.round(m / 60)}h ago`;
  }

  // ---- policy → English ----
  function summarizeBudget(tb) {
    if (!tb) return '<span class="muted">No daily limit.</span>';
    const d = tb.minutes_by_weekday;
    const weekdays = [d[1], d[2], d[3], d[4], d[5]];
    const weekend = [d[0], d[6]];
    const wdSame = weekdays.every((m) => m === weekdays[0]);
    const wkSame = weekend.every((m) => m === weekend[0]);
    if (wdSame && wkSame) {
      if (weekdays[0] === weekend[0]) {
        if (!weekdays[0]) return '<span class="em">No screens at all.</span>';
        return `<span class="em">${H.fmtDuration(weekdays[0])}</span> every day.`;
      }
      return `<span class="em">${H.fmtDuration(weekdays[0])}</span> on weekdays, <span class="em">${H.fmtDuration(weekend[0])}</span> on weekends.`;
    }
    // Mixed schedule: just describe today.
    return `<span class="em">${H.fmtDuration(d[new Date().getDay()])}</span> today.`;
  }

  function summarizeSchedule(sched) {
    if (!sched || !sched.blocks || !sched.blocks.length) {
      return '<span class="muted">No schedule rules set.</span>';
    }
    const out = sched.blocks.map((b) => {
      const labelStart = (b.start || "00:00").replace(/^0/, "");
      const labelEnd   = (b.end   || "00:00").replace(/^0/, "");
      let when;
      const wd = b.weekdays || [];
      if (wd.length === 7) when = "every day";
      else if (wd.length === 5 && [1,2,3,4,5].every((d) => wd.includes(d))) when = "weekdays";
      else if (wd.length === 2 && wd.includes(0) && wd.includes(6)) when = "weekends";
      else when = wd.map((w) => dayNames[w]).join(", ");
      const verb = b.kind === "blocked" ? "Blocked" : (b.kind || "Block");
      return `<span class="em">${H.escapeHTML(verb)} ${H.escapeHTML(labelStart)}–${H.escapeHTML(labelEnd)}</span> on ${H.escapeHTML(when)}`;
    });
    return out.join(". ") + ".";
  }

  function summarizeContent(c) {
    if (!c) return '<span class="muted">No content rules.</span>';
    const bits = [];
    const n = (arr) => (arr && arr.length) || 0;
    if (n(c.blocked_domains)) bits.push(`<span class="em">${n(c.blocked_domains)} site${n(c.blocked_domains) === 1 ? "" : "s"} blocked</span>`);
    if (n(c.blocked_apps))    bits.push(`<span class="em">${n(c.blocked_apps)} app${n(c.blocked_apps) === 1 ? "" : "s"} blocked</span>`);
    if (n(c.blocked_categories)) bits.push(`<span class="em">${n(c.blocked_categories)} categor${n(c.blocked_categories) === 1 ? "y" : "ies"} blocked</span>`);
    if (!bits.length) return '<span class="muted">No content rules.</span>';
    return bits.join(", ") + ".";
  }

  function summarizeMonitoring(level) {
    switch (level) {
      case "metadata":
        return '<span class="em">Metadata</span> — domain-level activity, no exact URLs.';
      case "granular":
        return '<span class="em">Detailed activity</span> — every site and app logged. Use sparingly.';
      case "enforcement_only":
      default:
        return '<span class="em">Enforcement only</span> — we record when a block happens, not what was browsed.';
    }
  }

  // ---- icons (kept inline; mirror the mockup) ----
  function deviceIconHTML(kind) {
    switch (kind) {
      case "phone":      return '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="6" y="2" width="12" height="20" rx="2.5"/><circle cx="12" cy="18" r="0.8" fill="currentColor"/></svg>';
      case "tablet":     return '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="3" width="16" height="18" rx="2"/><path d="M11 19h2"/></svg>';
      case "laptop":
      case "desktop":    return '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="4" width="20" height="12" rx="1.5"/><path d="M2 20h20"/></svg>';
      case "tv":         return '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="12" rx="1.5"/><path d="M8 21h8M12 17v4"/></svg>';
      case "console":    return '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="6" width="18" height="12" rx="3"/><circle cx="8" cy="12" r="1.4"/><circle cx="16" cy="12" r="1.4"/></svg>';
      default:           return '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="14" rx="2"/></svg>';
    }
  }

  // ---- main load ----
  let person, devices, policy, st1Day, st7Day, recentBlocks;
  try {
    [person, devices, policy, st1Day, st7Day, recentBlocks] = await Promise.all([
      H.api("GET", `/api/people/${encodeURIComponent(personID)}`),
      H.api("GET", `/api/devices`),
      H.api("GET", `/api/people/${encodeURIComponent(personID)}/policy`).catch((e) => {
        if (e.status === 404) return null;
        throw e;
      }),
      H.api("GET", `/api/reports/screen-time?person_id=${encodeURIComponent(personID)}&days=1`),
      H.api("GET", `/api/reports/screen-time?person_id=${encodeURIComponent(personID)}&days=7`),
      H.api("GET", `/api/events?person_id=${encodeURIComponent(personID)}&kind=block&limit=5`),
    ]);
  } catch (e) {
    if (e.status === 404) {
      document.querySelector("main.main").innerHTML =
        '<div class="empty-block">That person doesn\'t exist. <a href="/home">Back to home →</a></div>';
      return;
    }
    throw e;
  }
  const peopleList = (await H.api("GET", "/api/people")).people || [];
  const myDevices = (devices.devices || []).filter((d) => d.person_id === personID && d.state !== "retired");
  const personIdx = peopleList.findIndex((p) => p.id === personID);
  const cls = H.colorClass(personIdx);
  const initial = H.initialOf(person.name);

  H.renderFamilyNav(peopleList, personID);

  // ---- HERO ----
  document.querySelector('[data-fill="child-hero"]').innerHTML = `
    <div class="avatar ${cls}">${H.escapeHTML(initial)}</div>
    <div>
      <h1>${H.escapeHTML(person.name)}</h1>
      <div class="meta">
        <span>${myDevices.length} device${myDevices.length === 1 ? "" : "s"}</span>
        <span>Joined ${fmtDateShort(person.created_at)}</span>
      </div>
    </div>
    <div class="actions">
      <button class="btn" data-action="pause-all" ${myDevices.length === 0 ? "disabled" : ""}>Pause ${H.escapeHTML(person.name)}'s devices</button>
    </div>`;

  // ---- COMPUTED VALUES ----
  const dow = new Date().getDay();
  const budgetMins = (policy && policy.time_budget) ? (policy.time_budget.minutes_by_weekday[dow] || 0) : 0;
  const usedMins = (st1Day.days && st1Day.days.length) ? (st1Day.days[st1Day.days.length - 1].minutes || 0) : 0;
  const remainingMins = Math.max(0, budgetMins - usedMins);

  // The resolver tells us the current effective lock state.
  let resolved = null;
  try {
    const oneDevice = myDevices[0];
    if (oneDevice) {
      resolved = await H.api("GET", `/api/devices/${encodeURIComponent(oneDevice.id)}/resolved`);
    }
  } catch (_) {}

  let nowState, nowSubtitle;
  if (resolved && resolved.screen_locked) {
    nowState = resolved.lock_reason === "schedule" ? "Schedule block" : "Time budget reached";
    nowSubtitle = resolved.lock_reason === "schedule"
      ? "Screens paused by the active schedule rule."
      : `Daily budget of ${H.fmtDuration(resolved.time_budget_minutes || budgetMins)} reached for today.`;
  } else if (myDevices.some((d) => presenceForDevice(d) === "in use now")) {
    nowState = "Active";
    nowSubtitle = budgetMins > 0
      ? `${H.fmtDuration(remainingMins)} of today's budget remaining.`
      : "Free time — no daily cap set.";
  } else {
    nowState = "Idle";
    nowSubtitle = budgetMins > 0
      ? `${H.fmtDuration(remainingMins)} of today's budget left when they next pick up a device.`
      : "No devices active right now.";
  }

  // ---- HERO BAR (today's screen time card) ----
  const usagePct = budgetMins > 0 ? Math.min(100, Math.round((usedMins / budgetMins) * 100)) : 0;
  const pillCls = budgetMins === 0 ? "muted"
                : usedMins >= budgetMins ? "warn"
                : "ok";
  const pillText = budgetMins === 0 ? "No limit today"
                 : usedMins >= budgetMins ? "Over budget"
                 : "Within limits";
  const todayCardHTML = `
    <section class="card today-card">
      <div class="row-top">
        <h3>Today's screen time</h3>
        <span class="pill ${pillCls}"><span class="dot"></span> ${pillText}</span>
      </div>
      <div class="big num">${H.fmtDuration(usedMins)} ${budgetMins > 0 ? `<span class="of">/ ${H.fmtDuration(budgetMins)}</span>` : '<span class="of">/ no cap</span>'}</div>
      ${budgetMins > 0
        ? `<div class="bar"><span style="width: ${usagePct}%"></span></div>
           <div class="bar-meta">
             <span>${H.fmtDuration(remainingMins)} remaining today</span>
             <span>Resets at midnight</span>
           </div>`
        : `<div class="bar muted"><span style="width: 50%"></span></div>
           <div class="bar-meta"><span>No daily limit</span><span>Resets at midnight</span></div>`}
      <div class="now">
        <div class="now-icon">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>
        </div>
        <div class="now-text">
          <strong>${H.escapeHTML(nowState)}</strong>
          <span>${H.escapeHTML(nowSubtitle)}</span>
        </div>
      </div>
    </section>`;

  // ---- DEVICE LIST ----
  function rowFor(d) {
    const presence = presenceForDevice(d);
    const ctrl = d.agent_token
      ? '<span class="control-badge full">Full agent</span>'
      : '<span class="control-badge net">Network-only</span>';
    const pauseLabel = d.state === "paused" ? "Resume" : "Pause";
    return `
      <div class="dev-row" data-device-id="${H.escapeHTML(d.id)}" data-state="${H.escapeHTML(d.state)}">
        <div class="dev-icon">${deviceIconHTML(d.kind)}</div>
        <div>
          <div class="dev-name">${H.escapeHTML(d.label || "Unnamed device")}</div>
          <div class="dev-sub">${H.escapeHTML(d.kind || "device")} · ${H.escapeHTML(presence)}</div>
        </div>
        ${ctrl}
        <div class="dev-actions">
          <button class="btn btn-sm" data-action="toggle-pause">${pauseLabel}</button>
        </div>
      </div>`;
  }

  // Capability gap: if the policy needs caps the agent can't provide on this
  // device, surface it. Right now the only proxy we have is agent_token: any
  // network-only device can't enforce time_budget — call that out.
  function gapNote() {
    if (budgetMins <= 0) return "";
    const netOnly = myDevices.filter((d) => !d.agent_token && d.state !== "paused");
    if (!netOnly.length) return "";
    const list = netOnly.map((d) => d.label).join(", ");
    return `
      <div class="gap-note" style="margin-top: 18px;">
        <svg class="gap-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M12 8v5M12 16.5v0.1"/></svg>
        <div>
          <strong>Time limits can't apply to ${H.escapeHTML(list)}.</strong><br>
          ${netOnly.length === 1 ? "This device" : "These devices"} ${netOnly.length === 1 ? "has" : "have"} no Hearth agent installed, so we can't measure screen time on ${netOnly.length === 1 ? "it" : "them"}. <a href="/devices">See what's covered.</a>
        </div>
      </div>`;
  }

  const devicesCardHTML = `
    <section class="card">
      <div class="card-head">
        <h3>${H.escapeHTML(person.name)}'s devices</h3>
        <a class="quick-link" href="/devices">Manage all devices →</a>
      </div>
      <div class="dev-list">
        ${myDevices.length ? myDevices.map(rowFor).join("") : '<div class="empty-block">No devices yet. Claim one from <a href="/devices">the devices page</a>.</div>'}
      </div>
      ${gapNote()}
    </section>`;

  // ---- POLICY CARD ----
  const policyHTML = `
    <section class="card">
      <div class="card-head">
        <h3>Policy</h3>
        <span class="muted" style="font-size:12.5px">${policy ? "" : "No policy yet"}</span>
      </div>
      <div class="policy-block">
        <div class="p-label">Time budget</div>
        <div class="p-text">${summarizeBudget(policy && policy.time_budget)}</div>
        <a class="btn btn-sm" href="/policy-time?id=${encodeURIComponent(personID)}">Edit</a>
      </div>
      <div class="policy-block">
        <div class="p-label">Schedule</div>
        <div class="p-text">${summarizeSchedule(policy && policy.schedule)}</div>
        <a class="btn btn-sm" href="/policy-schedule?id=${encodeURIComponent(personID)}">Edit</a>
      </div>
      <div class="policy-block">
        <div class="p-label">Content</div>
        <div class="p-text">${summarizeContent(policy && policy.content)}</div>
        <a class="btn btn-sm" href="/policy-time?id=${encodeURIComponent(personID)}">Edit</a>
      </div>
      <div class="policy-block">
        <div class="p-label">Monitoring</div>
        <div class="p-text">${summarizeMonitoring(policy && policy.monitoring)}</div>
        <a class="btn btn-sm" href="/policy-monitoring?id=${encodeURIComponent(personID)}">Edit</a>
      </div>
    </section>`;

  // ---- RIGHT RAIL ----
  const blocksToday = (recentBlocks.events || []).filter((e) => {
    const d = new Date(e.at);
    const now = new Date();
    return d.getFullYear() === now.getFullYear() &&
           d.getMonth() === now.getMonth() &&
           d.getDate() === now.getDate();
  }).length;

  // 7-day chart heights, normalized to the max so the tallest bar is full.
  const week = (st7Day.days || []).slice(-7);
  // Pad with zeros so we always show 7 bars.
  while (week.length < 7) week.unshift({ day: "", minutes: 0 });
  const maxMins = Math.max(1, ...week.map((d) => d.minutes || 0));
  const todayIdx = week.length - 1;
  const weeklyAvg = week.reduce((a, d) => a + (d.minutes || 0), 0) / week.length;

  const dayLabels = [];
  for (let i = week.length - 1; i >= 0; i--) {
    const d = new Date(); d.setDate(d.getDate() - i);
    dayLabels.unshift(dayLetters[d.getDay()]);
  }

  // Bedtime: look at the policy schedule for the first "blocked" block whose
  // start is in the second half of the day (a heuristic, but visible to the
  // parent so they can sanity-check it).
  let bedtimeText = "—";
  if (policy && policy.schedule && policy.schedule.blocks) {
    const candidate = policy.schedule.blocks.find((b) =>
      b.kind === "blocked" && b.start && parseInt(b.start, 10) >= 17
    );
    if (candidate) {
      const [h, m] = candidate.start.split(":");
      const dt = new Date(); dt.setHours(parseInt(h, 10), parseInt(m, 10), 0, 0);
      bedtimeText = H.fmtTimeShort(dt);
    }
  }

  const rightRailHTML = `
    <aside class="stack" style="--gap: 24px;">
      <section class="card qf">
        <div class="qf-row">
          <span class="qf-label">Current state</span>
          <span class="qf-value">${H.escapeHTML(nowState)}</span>
        </div>
        <div class="qf-row">
          <span class="qf-label">Active devices</span>
          <span class="qf-value">${myDevices.filter((d) => presenceForDevice(d) === "in use now").length} of ${myDevices.length}</span>
        </div>
        <div class="qf-row">
          <span class="qf-label">Blocks today</span>
          <span class="qf-value num">${blocksToday}</span>
        </div>
        <div class="qf-row">
          <span class="qf-label">Bedtime starts</span>
          <span class="qf-value num">${H.escapeHTML(bedtimeText)}</span>
        </div>
        <div class="qf-row">
          <span class="qf-label">Weekly average</span>
          <span class="qf-value num">${H.fmtDuration(weeklyAvg)}</span>
        </div>
      </section>

      <section class="card">
        <div class="card-head"><h3>Last 7 days</h3></div>
        <div class="mini-chart">
          ${week.map((d, i) => {
            const h = Math.max(4, Math.round((d.minutes || 0) / maxMins * 100));
            return `<div class="${i === todayIdx ? "today" : ""}" style="height: ${h}%" title="${H.fmtDuration(d.minutes || 0)}"></div>`;
          }).join("")}
        </div>
        <div class="mini-chart-labels">
          ${dayLabels.map((l) => `<span>${l}</span>`).join("")}
        </div>
        <a class="quick-link" href="/reports" style="margin-top: 14px; display: inline-flex;">See full activity →</a>
      </section>

      ${recentBlocks.events && recentBlocks.events.length
        ? `<section class="card flat" style="background: var(--accent-soft); border: none;">
             <h4 style="color: var(--accent-ink); margin-bottom: 6px;">Recent block</h4>
             <p style="font-size: 13px; color: var(--accent-ink); line-height: 1.5;">
               ${H.escapeHTML(describeBlock(recentBlocks.events[0]))}
             </p>
           </section>`
        : ""}
    </aside>`;

  function describeBlock(ev) {
    const time = new Date(ev.at).toLocaleString(undefined, { weekday: "short", hour: "numeric", minute: "2-digit" });
    const cap = ev.attrs && ev.attrs.capability;
    const value = ev.attrs && ev.attrs.value;
    if (value) return `${person.name} tried to reach ${value} at ${time}. The block worked.`;
    if (cap)   return `A ${cap.replace("_", " ")} rule fired at ${time}. The block worked.`;
    return `A block fired at ${time}.`;
  }

  // ---- assemble ----
  document.querySelector('[data-fill="child-cols"]').innerHTML =
    `<div class="stack" style="--gap: 24px;">${todayCardHTML}${devicesCardHTML}${policyHTML}</div>` + rightRailHTML;

  // ---- interactions ----
  document.addEventListener("click", async (ev) => {
    const pauseAll = ev.target.closest('[data-action="pause-all"]');
    if (pauseAll) {
      pauseAll.disabled = true;
      try {
        await Promise.all(myDevices.map((d) =>
          H.api("PATCH", `/api/devices/${encodeURIComponent(d.id)}`, { state: "paused" })
        ));
        location.reload();
      } catch (e) {
        alert("Couldn't pause all devices: " + e.message);
        pauseAll.disabled = false;
      }
      return;
    }
    const toggle = ev.target.closest('[data-action="toggle-pause"]');
    if (toggle) {
      const row = toggle.closest("[data-device-id]");
      const newState = row.dataset.state === "paused" ? "active" : "paused";
      toggle.disabled = true;
      try {
        await H.api("PATCH", `/api/devices/${encodeURIComponent(row.dataset.deviceId)}`, { state: newState });
        location.reload();
      } catch (e) {
        alert("Couldn't change state: " + e.message);
        toggle.disabled = false;
      }
    }
  });
})();
