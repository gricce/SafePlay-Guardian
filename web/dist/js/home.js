(async () => {
  const H = window.Hearth;
  await H.requireAuth();

  function relTime(iso) {
    if (!iso) return "—";
    const ms = Date.now() - new Date(iso).getTime();
    const m = Math.round(ms / 60000);
    if (m < 1) return "just now";
    if (m < 60) return `${m} min ago`;
    const h = Math.round(m / 60);
    if (h < 24) return `${h}h ago`;
    const d = Math.round(h / 24);
    return `${d}d ago`;
  }

  function deviceChip(d) {
    const cls = d.state === "paused" ? "paused"
              : d.state === "active" ? ""
              : "away";
    const label = d.label || d.kind || "device";
    return `<span class="device-chip ${cls}"><span class="ddot"></span>${H.escapeHTML(label)}</span>`;
  }

  function childCard(p, idx, devices, screenTimeMins, budgetMins) {
    const cls = H.colorClass(idx);
    const initial = H.initialOf(p.name);
    const pct = budgetMins > 0 ? Math.min(100, Math.round((screenTimeMins / budgetMins) * 100)) : 50;
    const pillCls = budgetMins === 0 ? "muted"
                  : screenTimeMins >= budgetMins ? "warn"
                  : "ok";
    const pillText = budgetMins === 0 ? "No limit today"
                   : screenTimeMins >= budgetMins ? "Over budget"
                   : "Within limits";
    const usageText = budgetMins > 0
      ? `${H.fmtDuration(screenTimeMins)} <span class="muted">/ ${H.fmtDuration(budgetMins)}</span>`
      : `${H.fmtDuration(screenTimeMins)} <span class="muted">/ no cap</span>`;
    const barCls = budgetMins === 0 ? "bar muted" : "bar";
    const chips = devices.length
      ? devices.slice(0, 4).map(deviceChip).join("")
      : '<span class="device-chip"><span class="ddot"></span><span class="muted">no devices yet</span></span>';
    return `
      <a class="child-card card" href="/child?id=${encodeURIComponent(p.id)}">
        <div class="child-head">
          <div class="avatar lg ${cls}">${H.escapeHTML(initial)}</div>
          <div class="child-meta">
            <h3>${H.escapeHTML(p.name)}</h3>
            <span class="subtitle">${devices.length} device${devices.length === 1 ? "" : "s"}</span>
          </div>
          <span style="margin-left:auto" class="pill ${pillCls}"><span class="dot"></span> ${pillText}</span>
        </div>
        <div class="usage">
          <div class="usage-row">
            <span class="label">Screen time today</span>
            <span class="num">${usageText}</span>
          </div>
          <div class="${barCls}"><span style="width: ${pct}%"></span></div>
        </div>
        <div class="device-strip">${chips}</div>
      </a>`;
  }

  function discoveredRow(d) {
    const macIdent = (d.identities || []).find((i) => i.kind === "mac");
    const mac = macIdent ? macIdent.value : "";
    return `
      <div class="discovered-row" data-device-id="${H.escapeHTML(d.id)}">
        <div class="d-icon">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="12" rx="1.5"/><path d="M8 21h8M12 17v4"/></svg>
        </div>
        <div class="d-text">
          <strong>${H.escapeHTML(d.label || d.kind || "Unknown device")}</strong>
          <div class="sub">${relTime(d.last_seen)} <span>·</span> <span class="mac">${H.escapeHTML(mac)}</span></div>
        </div>
        <button class="btn btn-sm btn-primary" data-action="claim">Claim</button>
      </div>`;
  }

  async function load() {
    const now = new Date();
    H.fill("greeting", `${H.fmtDateLocale(now)} · ${H.fmtTimeShort(now)}`);
    H.fill("hero-title", "How's the family doing?");

    const [{ people }, { devices }] = await Promise.all([
      H.api("GET", "/api/people"),
      H.api("GET", "/api/devices"),
    ]);

    H.renderFamilyNav(people || [], null);

    const peopleList = (people || []).slice().sort((a, b) => a.name.localeCompare(b.name));
    const discovered = (devices || []).filter((d) => d.state === "discovered");
    const claimed = (devices || []).filter((d) => d.state !== "discovered");
    const active = claimed.filter((d) => d.state === "active");

    // Fetch each person's policy + today's screen time. Sequential is fine
    // at family scale and keeps the API simple.
    const cards = [];
    let blocksToday = 0;
    let withinLimits = 0;
    for (let i = 0; i < peopleList.length; i++) {
      const p = peopleList[i];
      const myDevices = claimed.filter((d) => d.person_id === p.id);
      let budgetMins = 0;
      try {
        const policy = await H.api("GET", `/api/people/${encodeURIComponent(p.id)}/policy`);
        if (policy && policy.time_budget) {
          const wd = now.getDay();
          budgetMins = policy.time_budget.minutes_by_weekday[wd] || 0;
        }
      } catch (e) {
        if (e.status !== 404) console.warn("policy fetch", p.id, e);
      }
      let usedMins = 0;
      try {
        const st = await H.api("GET", `/api/reports/screen-time?person_id=${encodeURIComponent(p.id)}&days=1`);
        const days = st.days || [];
        if (days.length) usedMins = days[days.length - 1].minutes || 0;
      } catch (_) {}
      if (budgetMins === 0 || usedMins < budgetMins) withinLimits++;
      cards.push(childCard(p, i, myDevices, usedMins, budgetMins));
    }
    H.fillHTML("child-cards", cards.join("\n") ||
      `<div class="empty-block">No people yet. <a href="/first-run">Add your first child</a> to get started.</div>`);

    // Recent block count today
    try {
      const blocks = await H.api("GET", `/api/reports/blocks?days=1`);
      blocksToday = (blocks.blocks || []).reduce((acc, b) => acc + (b.count || 0), 0);
    } catch (_) {}

    // Today strip
    const totalPeople = peopleList.length;
    const summary = [
      { label: "Within limits", value: `${withinLimits} of ${totalPeople || 0}`, sub: totalPeople ? "checked just now" : "add a child to start" },
      { label: "Active devices", value: String(active.length), sub: `of ${claimed.length} claimed` },
      { label: "Blocks today", value: String(blocksToday), sub: blocksToday ? "see Activity" : "nothing flagged" },
      { label: "Capability gaps", value: "—", sub: '<a class="quick-link" href="/devices">Review devices</a>' },
    ];
    const strip = document.querySelector('[data-fill="today-strip"]');
    if (strip) {
      strip.innerHTML = summary.map((c) => `
        <div>
          <span class="label">${H.escapeHTML(c.label)}</span>
          <span class="value">${H.escapeHTML(c.value)}</span>
          <span class="sub">${c.sub}</span>
        </div>`).join("");
    }

    // Discovered list
    H.fill("discovered-count", discovered.length
      ? `${discovered.length} new on your network`
      : "nothing new on your network");
    H.fillHTML("discovered-list", discovered.length
      ? discovered.map(discoveredRow).join("\n")
      : '<div class="empty-block">No new devices waiting. The scanner refreshes every 30 seconds.</div>');
  }

  // Quick-claim from the home screen — defaults to "unknown" kind, no person
  // assigned. The parent can rename and assign on the devices page.
  document.addEventListener("click", async (ev) => {
    const btn = ev.target.closest('[data-action="claim"]');
    if (!btn) return;
    const row = btn.closest("[data-device-id]");
    if (!row) return;
    btn.disabled = true;
    try {
      await H.api("PATCH", `/api/devices/${encodeURIComponent(row.dataset.deviceId)}`,
        { state: "active" });
      await load();
    } catch (e) {
      alert(`Couldn't claim that device: ${e.message}`);
      btn.disabled = false;
    }
  });

  await load();
  setInterval(load, 15000);
})();
