(async () => {
  const H = window.Hearth;
  await H.requireAuth();

  // ---- icons ----
  const ICON_PHONE = '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="6" y="2" width="12" height="20" rx="2.5"/><circle cx="12" cy="18" r="0.8" fill="currentColor"/></svg>';
  const ICON_TABLET = '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="3" width="16" height="18" rx="2"/><path d="M11 19h2"/></svg>';
  const ICON_LAPTOP = '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="4" width="20" height="12" rx="1.5"/><path d="M2 20h20"/></svg>';
  const ICON_TV = '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="12" rx="1.5"/><path d="M8 21h8M12 17v4"/></svg>';
  const ICON_CONSOLE = '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="6" width="18" height="12" rx="3"/><circle cx="8" cy="12" r="1.4"/><circle cx="16" cy="12" r="1.4"/></svg>';
  const ICON_GENERIC = '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="14" rx="2"/></svg>';
  const ICON_MENU = '<svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.5"/><circle cx="12" cy="12" r="1.5"/><circle cx="19" cy="12" r="1.5"/></svg>';

  function iconForKind(k) {
    switch (k) {
      case "phone":      return ICON_PHONE;
      case "tablet":     return ICON_TABLET;
      case "laptop":
      case "desktop":    return ICON_LAPTOP;
      case "tv":         return ICON_TV;
      case "console":    return ICON_CONSOLE;
      default:           return ICON_GENERIC;
    }
  }

  // "Away" is derived, not stored — the registry only tracks last_seen.
  // Anything seen in the last 5 minutes is Active; older Active rows render
  // as Away in the UI without changing their stored state.
  function effectiveState(d) {
    if (d.state === "paused") return "paused";
    if (d.state === "retired") return "retired";
    if (!d.last_seen) return "away";
    const ageMs = Date.now() - new Date(d.last_seen).getTime();
    return ageMs > 5 * 60 * 1000 ? "away" : "active";
  }

  function controlBadge(d) {
    if (d.agent_token) {
      return '<span class="control-badge full col-control">Full agent</span>';
    }
    return '<span class="control-badge net col-control">Network-only</span>';
  }

  function statePill(eff) {
    switch (eff) {
      case "active":
        return '<span class="state-pill active col-state"><span class="dot"></span>Active</span>';
      case "paused":
        return '<span class="state-pill paused col-state"><span class="dot"></span>Paused</span>';
      case "retired":
        return '<span class="state-pill away col-state"><span class="dot"></span>Retired</span>';
      default:
        return '<span class="state-pill away col-state"><span class="dot"></span>Away</span>';
    }
  }

  function presenceText(eff, d) {
    if (eff === "active") return "on Wi-Fi";
    if (eff === "paused") return "manually paused";
    if (eff === "retired") return "removed";
    if (!d.last_seen) return "never seen";
    const ageMin = Math.round((Date.now() - new Date(d.last_seen).getTime()) / 60000);
    if (ageMin < 60) return `last seen ${ageMin}m ago`;
    return `last seen ${Math.round(ageMin / 60)}h ago`;
  }

  function macIdentity(d) {
    const m = (d.identities || []).find((i) => i.kind === "mac");
    return m ? m.value : "";
  }

  function deviceRow(d) {
    const eff = effectiveState(d);
    const mac = macIdentity(d);
    const subtitle = mac
      ? `${H.escapeHTML(d.kind || "device")} · ${H.escapeHTML(mac)}`
      : H.escapeHTML(d.kind || "device");
    return `
      <div class="dev-table-row" data-device-id="${H.escapeHTML(d.id)}" data-state="${eff}" data-search="${H.escapeHTML((d.label + " " + mac).toLowerCase())}">
        <div class="dev-icon-cell">${iconForKind(d.kind)}</div>
        <div class="dev-name-cell">
          <strong>${H.escapeHTML(d.label || "Unnamed device")}</strong>
          <div class="sub">${subtitle}</div>
        </div>
        ${statePill(eff)}
        <span class="col-presence muted" style="font-size: 12.5px;">${H.escapeHTML(presenceText(eff, d))}</span>
        ${controlBadge(d)}
        <button class="menu-btn" data-action="open-menu" aria-label="More actions for ${H.escapeHTML(d.label || "device")}">${ICON_MENU}</button>
      </div>`;
  }

  function deviceGroup(headerHTML, devices, count) {
    return `
      <section class="group" data-group>
        <div class="group-head">
          ${headerHTML}
          <span class="count">${count} device${count === 1 ? "" : "s"}</span>
        </div>
        <div class="dev-table">
          <div class="dev-table-row head">
            <span></span><span>Device</span>
            <span class="col-state">State</span>
            <span class="col-presence">Presence</span>
            <span class="col-control">Control</span>
            <span></span>
          </div>
          ${devices.map(deviceRow).join("")}
        </div>
      </section>`;
  }

  function personGroup(person, idx, devices) {
    const cls = H.colorClass(idx);
    const initial = H.initialOf(person.name);
    const header = `<span class="avatar ${cls}">${H.escapeHTML(initial)}</span><h3>${H.escapeHTML(person.name)}</h3>`;
    return deviceGroup(header, devices, devices.length);
  }

  function unassignedGroup(devices) {
    const header = `<span class="avatar" style="background: var(--surface-2); color: var(--muted);">?</span><h3>Unassigned</h3>`;
    return deviceGroup(header, devices, devices.length);
  }

  function discoveredRow(d) {
    const mac = macIdentity(d);
    return `
      <div class="discovered-card" data-device-id="${H.escapeHTML(d.id)}">
        <div class="dev-icon-cell">${iconForKind(d.kind)}</div>
        <div>
          <strong>${H.escapeHTML(d.label || "Unknown device")}</strong>
          <div class="sub"><span>${H.escapeHTML(relTime(d.last_seen))}</span><span>·</span><span>${H.escapeHTML(mac)}</span></div>
        </div>
        <div class="actions">
          <button class="btn btn-sm" data-action="ignore">Ignore</button>
          <button class="btn btn-sm btn-primary" data-action="claim">Claim →</button>
        </div>
      </div>`;
  }

  function discoveredSection(devices) {
    if (!devices.length) {
      return `
        <section class="discovered-section" data-group="discovered">
          <div class="group-head" style="margin-bottom: 12px;">
            <h3>Discovered — waiting to be set up</h3>
            <span class="count">0 unclaimed</span>
          </div>
          <div class="empty-block">Nothing new on the network yet. The passive scanner refreshes every 30s.</div>
        </section>`;
    }
    return `
      <section class="discovered-section" data-group="discovered">
        <div class="group-head" style="margin-bottom: 12px;">
          <h3>Discovered — waiting to be set up</h3>
          <span class="count">${devices.length} unclaimed</span>
        </div>
        ${devices.map(discoveredRow).join("")}
      </section>`;
  }

  function relTime(iso) {
    if (!iso) return "—";
    const m = Math.round((Date.now() - new Date(iso).getTime()) / 60000);
    if (m < 1) return "just now";
    if (m < 60) return `first seen ${m} min ago`;
    const h = Math.round(m / 60);
    if (h < 24) return `first seen ${h}h ago`;
    return `first seen ${Math.round(h / 24)}d ago`;
  }

  // ---- state ----
  let allPeople = [];
  let allDevices = [];
  let activeFilter = "all";
  let searchQuery = "";

  function applyFilters() {
    const groupsEl = document.querySelector('[data-fill="device-groups"]');
    if (!groupsEl) return;
    const q = searchQuery.trim().toLowerCase();
    const filter = activeFilter;
    groupsEl.querySelectorAll("[data-group]").forEach((group) => {
      let groupVisible = false;
      group.querySelectorAll(".dev-table-row[data-device-id], .discovered-card[data-device-id]").forEach((row) => {
        const state = row.dataset.state || "discovered";
        const text = row.dataset.search || "";
        let show = true;
        if (filter !== "all") {
          if (filter === "unassigned") {
            show = group.dataset.group === "unassigned";
          } else {
            show = state === filter;
          }
        }
        if (show && q) show = text.includes(q);
        row.style.display = show ? "" : "none";
        if (show) groupVisible = true;
      });
      group.style.display = groupVisible ? "" : "none";
    });
  }

  function render() {
    H.renderFamilyNav(allPeople, null);

    const claimed = allDevices.filter((d) => d.state !== "discovered" && d.state !== "retired");
    const discovered = allDevices.filter((d) => d.state === "discovered");

    const byPerson = new Map();
    const unassigned = [];
    for (const d of claimed) {
      if (d.person_id) {
        if (!byPerson.has(d.person_id)) byPerson.set(d.person_id, []);
        byPerson.get(d.person_id).push(d);
      } else {
        unassigned.push(d);
      }
    }

    // Counts (states are *effective* for the pills the parent sees)
    const counts = { all: claimed.length, active: 0, away: 0, paused: 0, unassigned: unassigned.length };
    for (const d of claimed) counts[effectiveState(d)] = (counts[effectiveState(d)] || 0) + 1;
    for (const key of Object.keys(counts)) H.fill(`count-${key}`, String(counts[key]));

    const sortedPeople = allPeople.slice().sort((a, b) => a.name.localeCompare(b.name));
    let html = "";
    sortedPeople.forEach((p, idx) => {
      const list = byPerson.get(p.id) || [];
      if (!list.length) return; // omit empty groups — keeps the page calm
      html += personGroup(p, idx, list);
    });
    if (unassigned.length) {
      const wrap = unassignedGroup(unassigned);
      // Tag the group so the Unassigned filter tab can target it.
      html += wrap.replace('<section class="group"', '<section class="group" data-group="unassigned"');
    }
    if (!html) {
      html = '<div class="empty-block">No claimed devices yet. Claim one from the Discovered list below to assign it to a person.</div>';
    }
    html += discoveredSection(discovered);

    document.querySelector('[data-fill="device-groups"]').innerHTML = html;
    applyFilters();
  }

  async function load() {
    const [{ people }, { devices }] = await Promise.all([
      H.api("GET", "/api/people"),
      H.api("GET", "/api/devices"),
    ]);
    allPeople = people || [];
    allDevices = devices || [];
    render();
  }

  // ---- interactions ----
  document.querySelectorAll(".filter-tab[data-filter]").forEach((tab) => {
    tab.addEventListener("click", () => {
      document.querySelectorAll(".filter-tab").forEach((t) => t.classList.remove("active"));
      tab.classList.add("active");
      activeFilter = tab.dataset.filter;
      applyFilters();
    });
  });
  const searchEl = document.getElementById("device-search");
  if (searchEl) {
    searchEl.addEventListener("input", () => {
      searchQuery = searchEl.value;
      applyFilters();
    });
  }
  // Refresh-scan button reloads from the server (passive scan keeps running).
  document.querySelectorAll(".page-actions .btn").forEach((b) => {
    if (b.textContent.trim() === "Refresh scan") {
      b.addEventListener("click", load);
    }
  });

  // Claim / ignore in the discovered section, plus the per-row menu.
  document.addEventListener("click", async (ev) => {
    const claimBtn = ev.target.closest('[data-action="claim"]');
    const ignoreBtn = ev.target.closest('[data-action="ignore"]');
    const menuBtn = ev.target.closest('[data-action="open-menu"]');

    if (claimBtn) {
      const row = claimBtn.closest("[data-device-id]");
      await claim(row.dataset.deviceId, claimBtn);
      return;
    }
    if (ignoreBtn) {
      const row = ignoreBtn.closest("[data-device-id]");
      await patchDevice(row.dataset.deviceId, { state: "retired" });
      await load();
      return;
    }
    if (menuBtn) {
      const row = menuBtn.closest("[data-device-id]");
      openMenu(menuBtn, row);
      return;
    }
    // Click anywhere else dismisses an open menu.
    const open = document.querySelector(".dev-menu-pop");
    if (open && !ev.target.closest(".dev-menu-pop")) open.remove();
  });

  async function patchDevice(id, body) {
    return H.api("PATCH", `/api/devices/${encodeURIComponent(id)}`, body);
  }

  async function claim(deviceId, btn) {
    // Inline mini-form: ask which person + optional rename, then PATCH.
    const row = btn.closest("[data-device-id]");
    if (row.querySelector(".inline-claim")) return; // already open
    btn.disabled = true;
    const personOpts = allPeople.length
      ? allPeople.map((p) => `<option value="${H.escapeHTML(p.id)}">${H.escapeHTML(p.name)}</option>`).join("")
      : "";
    const form = document.createElement("form");
    form.className = "inline-claim";
    form.style.cssText = "grid-column: 1 / -1; display: flex; gap: 8px; padding-top: 10px; margin-top: 6px; border-top: 1px solid var(--border); align-items: center;";
    form.innerHTML = `
      <input name="label" placeholder="Rename (optional)" style="flex: 1; padding: 6px 10px; border: 1px solid var(--border); border-radius: 8px; font-size: 13px;">
      <select name="person_id" style="padding: 6px 10px; border: 1px solid var(--border); border-radius: 8px; font-size: 13px;">
        <option value="">— person —</option>${personOpts}
      </select>
      <button class="btn btn-sm btn-primary" type="submit">Confirm</button>
      <button class="btn btn-sm" type="button" data-action="cancel-claim">Cancel</button>
    `;
    row.appendChild(form);
    form.addEventListener("submit", async (ev) => {
      ev.preventDefault();
      const data = Object.fromEntries(new FormData(form));
      const patch = { state: "active" };
      if (data.label && data.label.trim()) patch.label = data.label.trim();
      if (data.person_id) patch.person_id = data.person_id;
      try {
        await patchDevice(deviceId, patch);
        await load();
      } catch (e) {
        alert(`Couldn't claim: ${e.message}`);
        btn.disabled = false;
      }
    });
    form.querySelector('[data-action="cancel-claim"]').addEventListener("click", () => {
      form.remove();
      btn.disabled = false;
    });
  }

  function openMenu(anchor, row) {
    const existing = document.querySelector(".dev-menu-pop");
    if (existing) existing.remove();

    const deviceId = row.dataset.deviceId;
    const eff = row.dataset.state;
    const items = [];
    if (eff === "paused") {
      items.push({ label: "Resume", action: () => patchDevice(deviceId, { state: "active" }).then(load) });
    } else {
      items.push({ label: "Pause", action: () => patchDevice(deviceId, { state: "paused" }).then(load) });
    }
    items.push({ label: "Retire", action: () => {
      if (confirm("Soft-retire this device? It stops appearing in lists but its history stays.")) {
        patchDevice(deviceId, { state: "retired" }).then(load);
      }
    }});

    const pop = document.createElement("div");
    pop.className = "dev-menu-pop";
    pop.style.cssText = "position: absolute; z-index: 100; min-width: 140px; background: var(--surface); border: 1px solid var(--border-strong); border-radius: var(--r-md); box-shadow: var(--shadow-2); padding: 4px;";
    pop.innerHTML = items.map((it, i) =>
      `<button data-idx="${i}" style="display: block; width: 100%; text-align: left; background: transparent; border: 0; padding: 8px 12px; font-size: 13px; color: var(--fg); cursor: pointer; border-radius: 6px;">${H.escapeHTML(it.label)}</button>`
    ).join("");
    pop.querySelectorAll("button").forEach((btn) => {
      btn.addEventListener("mouseover", () => { btn.style.background = "var(--surface-2)"; });
      btn.addEventListener("mouseout", () => { btn.style.background = "transparent"; });
      btn.addEventListener("click", () => {
        items[parseInt(btn.dataset.idx, 10)].action();
        pop.remove();
      });
    });

    const rect = anchor.getBoundingClientRect();
    pop.style.top = (window.scrollY + rect.bottom + 4) + "px";
    pop.style.left = (window.scrollX + rect.right - 140) + "px";
    document.body.appendChild(pop);
  }

  await load();
  setInterval(load, 20000);
})();
