(async () => {
  const H = window.Hearth;
  await H.requireAuth();

  const state = { personID: "", days: 7 };

  // ---- helpers ----
  const dayLabels = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

  function shortDayLabel(date) {
    return dayLabels[date.getDay()].slice(0, 3);
  }

  function ymd(date) {
    const y = date.getUTCFullYear();
    const m = String(date.getUTCMonth() + 1).padStart(2, "0");
    const d = String(date.getUTCDate()).padStart(2, "0");
    return `${y}-${m}-${d}`;
  }

  // Pad a screen-time series into a contiguous daily array (UTC-day buckets)
  // covering the last N days, including today.
  function padSeries(rows, days) {
    const byDay = new Map((rows || []).map((r) => [r.day, r.minutes || 0]));
    const out = [];
    const now = new Date();
    const start = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
    start.setUTCDate(start.getUTCDate() - (days - 1));
    for (let i = 0; i < days; i++) {
      const d = new Date(start);
      d.setUTCDate(d.getUTCDate() + i);
      out.push({ date: d, minutes: byDay.get(ymd(d)) || 0 });
    }
    return out;
  }

  function relTime(iso) {
    if (!iso) return "—";
    const date = new Date(iso);
    const now = new Date();
    const sameDay = date.toDateString() === now.toDateString();
    if (sameDay) {
      return `Today, ${date.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" })}`;
    }
    const diffMs = now - date;
    const diffDays = Math.floor(diffMs / (24 * 3600 * 1000));
    if (diffDays < 7) {
      const wd = date.toLocaleDateString(undefined, { weekday: "short" });
      return `${wd}, ${date.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" })}`;
    }
    return date.toLocaleString(undefined, { month: "short", day: "numeric" });
  }

  function selectedPerson(people) {
    return people.find((p) => p.id === state.personID) || null;
  }

  // ---- renderers ----
  function renderChildTabs(people) {
    const el = document.querySelector('[data-fill="child-tabs"]');
    const tabs = [`<button class="child-tab ${state.personID === "" ? "active" : ""}" data-person="">All children</button>`];
    const sorted = (people || []).slice().sort((a, b) => a.name.localeCompare(b.name));
    sorted.forEach((p, i) => {
      const cls = H.colorClass(i);
      const initial = H.initialOf(p.name);
      const active = state.personID === p.id ? "active" : "";
      tabs.push(
        `<button class="child-tab ${active}" data-person="${H.escapeHTML(p.id)}">` +
        `<span class="avatar sm ${cls}">${H.escapeHTML(initial)}</span>${H.escapeHTML(p.name)}</button>`
      );
    });
    el.innerHTML = tabs.join("");
  }

  function renderChart(currSeries, prevSeries) {
    const card = document.querySelector('[data-fill="chart-card"]');
    const total = currSeries.reduce((a, s) => a + s.minutes, 0);
    const prevTotal = prevSeries.reduce((a, s) => a + s.minutes, 0);
    const delta = total - prevTotal;
    const deltaText = delta === 0
      ? "no change"
      : `${delta < 0 ? "↓" : "↑"} ${H.fmtDuration(Math.abs(delta))} vs. previous ${state.days} day${state.days === 1 ? "" : "s"}`;
    const deltaClass = delta <= 0 ? "delta-good" : "delta-up";

    // y-axis range
    const max = Math.max(60, ...currSeries.map((s) => s.minutes));
    const ceil = Math.ceil(max / 60) * 60; // round up to whole hours
    const hours = Math.ceil(ceil / 60);
    const gridLabels = [];
    for (let i = hours; i >= 0; i--) gridLabels.push(`${i}h`);

    // SVG geometry — viewBox 700×220; the page reserves 40px for y-axis labels
    // in padding-left. Match the mockup's whitespace at the top.
    const W = 700, H_ = 220;
    const xs = currSeries.length === 1
      ? [W / 2]
      : currSeries.map((_, i) => (i / (currSeries.length - 1)) * W);
    const ys = currSeries.map((s) => H_ - (s.minutes / ceil) * H_);
    const polyline = currSeries.map((_, i) => `${xs[i]},${ys[i]}`).join(" ");
    const circles = currSeries.map((s, i) => {
      const today = i === currSeries.length - 1;
      const r = today ? 5 : 4;
      const stroke = today ? ' stroke="var(--surface)" stroke-width="2"' : "";
      return `<circle cx="${xs[i]}" cy="${ys[i]}" r="${r}"${stroke}><title>${H.fmtDuration(s.minutes)} on ${s.date.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" })}</title></circle>`;
    }).join("");

    const xLabels = currSeries.map((s, i) => {
      const today = i === currSeries.length - 1;
      return `<span>${today ? "Today" : shortDayLabel(s.date)}</span>`;
    }).join("");

    card.innerHTML = `
      <div class="chart-head">
        <div>
          <h2>Total screen time</h2>
          <div class="total num">${H.fmtDuration(total)}</div>
          <div class="delta"><strong class="${deltaClass}">${deltaText}</strong></div>
        </div>
        <div class="legend-row">
          <span><span class="swatch s-active"></span>Active time</span>
        </div>
      </div>
      <div class="chart-area">
        <div class="chart-grid">
          ${gridLabels.map((lbl) => `<div data-val="${lbl}"></div>`).join("")}
        </div>
        <svg viewBox="0 0 ${W} ${H_}" preserveAspectRatio="none" style="padding-left: 40px;">
          <polyline points="${polyline}" fill="none" stroke="var(--accent)" stroke-width="2.5" stroke-linejoin="round"/>
          <g fill="var(--accent)">${circles}</g>
        </svg>
      </div>
      <div class="chart-x">${xLabels}</div>`;
  }

  function renderBlocks(blocks, peopleById, devicesById) {
    const card = document.querySelector('[data-fill="blocks-card"]');
    if (!blocks.length) {
      card.innerHTML = `
        <div class="card-head"><h3>Recent blocks</h3><span class="muted" style="font-size:12.5px;">last ${state.days} day${state.days === 1 ? "" : "s"}</span></div>
        <div class="empty-block">No blocks recorded yet. Once a policy fires for the first time, it lands here.</div>`;
      return;
    }
    const rows = blocks.map((ev) => {
      const dev = devicesById.get(ev.device_id);
      const personID = ev.person_id || (dev && dev.person_id) || "";
      const person = personID ? peopleById.get(personID) : null;
      const personIdx = person ? Array.from(peopleById.values()).findIndex((p) => p.id === person.id) : -1;
      const cls = personIdx >= 0 ? H.colorClass(personIdx) : "";
      const personHTML = person
        ? `<span class="avatar sm ${cls}" style="vertical-align:middle;margin-right:6px">${H.escapeHTML(H.initialOf(person.name))}</span>${H.escapeHTML(person.name)}`
        : `<span class="muted">Unassigned device</span>`;
      const devLabel = dev ? dev.label : ev.device_id.slice(0, 8);
      const value = ev.attrs && ev.attrs.value;
      const cap = (ev.attrs && ev.attrs.capability) || "block";
      const what = value
        ? `${value} — ${cap.replace("_", " ")}`
        : `${cap.replace("_", " ")} fired`;
      return `
        <div class="block-row">
          <div class="block-icon">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M5.6 5.6l12.8 12.8"/></svg>
          </div>
          <div>
            <div class="who">${personHTML} <small>· ${H.escapeHTML(devLabel)}</small></div>
            <div class="what">${H.escapeHTML(what)}</div>
          </div>
          <div class="when">${H.escapeHTML(relTime(ev.at))}</div>
        </div>`;
    }).join("");
    card.innerHTML = `
      <div class="card-head"><h3>Recent blocks</h3><span class="muted" style="font-size:12.5px;">last ${state.days} day${state.days === 1 ? "" : "s"}</span></div>
      <div class="blocks-list">${rows}</div>`;
  }

  function renderByDevice(rows, devicesById) {
    const card = document.querySelector('[data-fill="by-device-card"]');
    if (!rows.length) {
      card.innerHTML = `
        <div class="card-head"><h3>By device</h3><span class="muted" style="font-size:12.5px;">last ${state.days} day${state.days === 1 ? "" : "s"} · time online</span></div>
        <div class="empty-block">No device activity in this window yet.</div>`;
      return;
    }
    const max = Math.max(1, ...rows.map((r) => r.minutes));
    const html = rows.map((r) => {
      const dev = devicesById.get(r.device_id);
      const label = dev ? dev.label : r.device_id.slice(0, 8);
      const noAgent = dev && !dev.agent_token;
      const w = Math.round((r.minutes / max) * 100);
      return `
        <div class="dev-stack-row">
          <div class="dev-label">${H.escapeHTML(label)}</div>
          <div class="dev-bar"><span style="width: ${w}%"></span></div>
          <div class="dev-val ${noAgent ? "muted" : ""}">${H.fmtDuration(r.minutes)}</div>
        </div>`;
    }).join("");
    const netNote = rows.some((r) => {
      const d = devicesById.get(r.device_id);
      return d && !d.agent_token;
    });
    card.innerHTML = `
      <div class="card-head"><h3>By device</h3><span class="muted" style="font-size:12.5px;">last ${state.days} day${state.days === 1 ? "" : "s"} · time online</span></div>
      <div class="dev-stack">${html}</div>
      ${netNote ? `<p class="muted" style="font-size: 12px; margin-top: 14px; line-height: 1.5;">Network-only devices report on/off-network presence — not necessarily active use.</p>` : ""}`;
  }

  // ---- load ----
  let people = [];
  let devices = [];

  async function fullLoad() {
    const [peopleResp, devicesResp] = await Promise.all([
      H.api("GET", "/api/people"),
      H.api("GET", "/api/devices"),
    ]);
    people = peopleResp.people || [];
    devices = devicesResp.devices || [];
    H.renderFamilyNav(people, null);
    renderChildTabs(people);
    await reloadData();
  }

  async function reloadData() {
    const personParam = state.personID ? `&person_id=${encodeURIComponent(state.personID)}` : "";
    const peopleById = new Map(people.map((p) => [p.id, p]));
    const devicesById = new Map(devices.map((d) => [d.id, d]));

    const [currST, byDev, eventsResp] = await Promise.all([
      H.api("GET", `/api/reports/screen-time?days=${state.days}${personParam}`),
      H.api("GET", `/api/reports/by-device?days=${state.days}${personParam}`),
      H.api("GET", `/api/events?kind=block&limit=10${personParam}`),
    ]);

    // Previous-period series for the delta number. Fetch days*2 then take the
    // first half.
    let prevDays = [];
    try {
      const dbl = await H.api("GET", `/api/reports/screen-time?days=${state.days * 2}${personParam}`);
      const padded = padSeries(dbl.days || [], state.days * 2);
      prevDays = padded.slice(0, state.days);
    } catch (_) {}

    const currSeries = padSeries(currST.days, state.days);
    renderChart(currSeries, prevDays);
    renderBlocks(eventsResp.events || [], peopleById, devicesById);
    renderByDevice(byDev.devices || [], devicesById);
  }

  // ---- interactions ----
  document.addEventListener("click", async (ev) => {
    const personTab = ev.target.closest('[data-person]');
    const rangeTab  = ev.target.closest('[data-range]');
    if (personTab) {
      document.querySelectorAll('[data-person]').forEach((t) => t.classList.remove("active"));
      personTab.classList.add("active");
      state.personID = personTab.dataset.person;
      await reloadData();
      return;
    }
    if (rangeTab) {
      document.querySelectorAll('[data-range]').forEach((t) => t.classList.remove("active"));
      rangeTab.classList.add("active");
      state.days = parseInt(rangeTab.dataset.range, 10) || 7;
      await reloadData();
      return;
    }
  });

  await fullLoad();
  setInterval(reloadData, 60000);
})();
