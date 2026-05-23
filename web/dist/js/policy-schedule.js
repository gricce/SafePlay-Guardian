(async () => {
  const H = window.Hearth;
  await H.requireAuth();

  const personID = new URLSearchParams(location.search).get("id");
  if (!personID) { location.replace("/home"); return; }

  // ---- context ----
  let person, peopleList, policy;
  try {
    [person, peopleList] = await Promise.all([
      H.api("GET", `/api/people/${encodeURIComponent(personID)}`),
      H.api("GET", "/api/people").then((r) => r.people || []),
    ]);
  } catch (e) {
    if (e.status === 404) { location.replace("/home"); return; }
    throw e;
  }
  try {
    policy = await H.api("GET", `/api/people/${encodeURIComponent(personID)}/policy`);
  } catch (e) {
    if (e.status !== 404) throw e;
    policy = null;
  }

  H.renderFamilyNav(peopleList, personID);
  const crumb = document.querySelector('[data-fill="breadcrumb-link"]');
  if (crumb) {
    crumb.textContent = person.name;
    crumb.href = `/child?id=${encodeURIComponent(personID)}`;
  }

  // ---- grid model ----
  // UI day order: Mon..Sun (rows 0..6). Backend uses Sun=0..Sat=6.
  const HOURS = 24;
  const DAYS_UI = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
  const goFromUi = (ui) => (ui + 1) % 7; // 0(Mon)->1, 5(Sat)->6, 6(Sun)->0
  const uiFromGo = (go) => (go + 6) % 7;

  // States the editor knows about. "free" means no rule; not stored.
  const STATES = ["free", "school", "blocked", "bedtime"];
  // cells[uiDay][hour] = state string
  const cells = Array.from({ length: 7 }, () => Array(HOURS).fill("free"));

  function parseHM(s) {
    if (!s) return null;
    const m = /^(\d{1,2}):(\d{2})$/.exec(s);
    if (!m) return null;
    const h = parseInt(m[1], 10), mm = parseInt(m[2], 10);
    if (h < 0 || h > 23 || mm < 0 || mm > 59) return null;
    return h * 60 + mm;
  }
  function fmtHM(mins) {
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;
  }

  // Load policy.schedule.blocks → cells. Hour granularity: start hour through
  // end hour exclusive. Wraps midnight when end < start.
  function blocksToGrid(blocks) {
    if (!blocks) return;
    for (const b of blocks) {
      if (!STATES.includes(b.kind)) continue;
      const startMin = parseHM(b.start);
      const endMin = parseHM(b.end);
      if (startMin == null || endMin == null) continue;
      const startH = Math.floor(startMin / 60);
      const endH = Math.ceil(endMin / 60);
      const wd = b.weekdays || [];
      for (const goDay of wd) {
        const uiDay = uiFromGo(goDay);
        if (startMin < endMin) {
          for (let h = startH; h < endH; h++) cells[uiDay][h] = b.kind;
        } else {
          // wrap: paint [startH, 24) on this day, [0, endH) on next
          for (let h = startH; h < HOURS; h++) cells[uiDay][h] = b.kind;
          const next = (uiDay + 1) % 7;
          for (let h = 0; h < endH; h++) cells[next][h] = b.kind;
        }
      }
    }
  }
  blocksToGrid(policy && policy.schedule && policy.schedule.blocks);

  // ---- build grid DOM ----
  const grid = document.getElementById("grid");
  grid.innerHTML = '<div class="hh-head corner"></div>';
  for (let h = 0; h < HOURS; h++) {
    const el = document.createElement("div");
    el.className = "hh-head";
    if (h % 3 === 0) {
      const hour12 = h === 0 ? "12a" : h < 12 ? `${h}a` : h === 12 ? "12p" : `${h - 12}p`;
      el.textContent = hour12;
    } else {
      el.textContent = "·";
    }
    grid.appendChild(el);
  }
  const cellEls = []; // [uiDay][hour]
  for (let d = 0; d < 7; d++) {
    cellEls.push([]);
    const label = document.createElement("div");
    label.className = "day-label";
    label.textContent = DAYS_UI[d];
    grid.appendChild(label);
    for (let h = 0; h < HOURS; h++) {
      const cell = document.createElement("div");
      cell.className = "cell";
      cell.dataset.day = String(d);
      cell.dataset.hour = String(h);
      cellEls[d].push(cell);
      grid.appendChild(cell);
    }
  }
  function paintCell(uiDay, h, state) {
    const el = cellEls[uiDay][h];
    el.classList.remove("s-blocked", "s-school", "s-bedtime");
    if (state === "blocked") el.classList.add("s-blocked");
    else if (state === "school") el.classList.add("s-school");
    else if (state === "bedtime") el.classList.add("s-bedtime");
    cells[uiDay][h] = state;
  }
  function repaintAll() {
    for (let d = 0; d < 7; d++) for (let h = 0; h < HOURS; h++) paintCell(d, h, cells[d][h]);
  }
  repaintAll();
  renderSummary();

  // ---- paint mode + drag ----
  let mode = "free";
  document.querySelectorAll(".paint-swatch").forEach((btn) => {
    btn.addEventListener("click", () => {
      document.querySelectorAll(".paint-swatch").forEach((b) => b.classList.remove("active"));
      btn.classList.add("active");
      mode = btn.dataset.mode;
    });
  });

  let painting = false;
  function paint(ev, el) {
    if (!el.classList.contains("cell")) return;
    const d = parseInt(el.dataset.day, 10), h = parseInt(el.dataset.hour, 10);
    paintCell(d, h, ev.shiftKey ? "free" : mode);
  }
  grid.addEventListener("mousedown", (ev) => {
    if (!ev.target.classList.contains("cell")) return;
    painting = true;
    paint(ev, ev.target);
    ev.preventDefault();
  });
  grid.addEventListener("mouseover", (ev) => {
    if (!painting) return;
    paint(ev, ev.target);
  });
  document.addEventListener("mouseup", () => {
    if (painting) {
      painting = false;
      renderSummary();
    }
  });

  // ---- presets ----
  const presetBtn = document.getElementById("presetSchool");
  const clearBtn = document.getElementById("clearAll");
  if (presetBtn) presetBtn.addEventListener("click", () => {
    for (let d = 0; d < 7; d++) for (let h = 0; h < HOURS; h++) cells[d][h] = "free";
    for (let d = 0; d < 7; d++) {
      const isWeekday = d < 5; // mockup Mon..Fri = ui 0..4
      for (let h = 0; h < HOURS; h++) {
        if (h >= 20 || h < 7) cells[d][h] = "bedtime";
        if (isWeekday && h >= 8 && h < 15) cells[d][h] = "school";
        if (isWeekday && h === 16) cells[d][h] = "blocked";
      }
    }
    repaintAll();
    renderSummary();
  });
  if (clearBtn) clearBtn.addEventListener("click", () => {
    for (let d = 0; d < 7; d++) for (let h = 0; h < HOURS; h++) cells[d][h] = "free";
    repaintAll();
    renderSummary();
  });

  // ---- grid → ScheduleBlock[] for save ----
  // For each uiDay and each non-"free" state, group contiguous runs into one
  // block. Wraps that cross midnight on the same logical "bedtime" rule are
  // still saved as two day-local blocks — simpler than merging, and the
  // resolver evaluates either form identically.
  function gridToBlocks() {
    const out = [];
    for (let d = 0; d < 7; d++) {
      const goDay = goFromUi(d);
      let i = 0;
      while (i < HOURS) {
        const s = cells[d][i];
        if (s === "free") { i++; continue; }
        let j = i + 1;
        while (j < HOURS && cells[d][j] === s) j++;
        out.push({
          kind: s,
          weekdays: [goDay],
          start: fmtHM(i * 60),
          end: j === HOURS ? "00:00" : fmtHM(j * 60),
        });
        i = j;
      }
    }
    // Cosmetic merge: for any two consecutive (in saved order) blocks with the
    // same kind on different weekdays and same start/end, group them.
    const grouped = [];
    for (const b of out) {
      const last = grouped[grouped.length - 1];
      if (last && last.kind === b.kind && last.start === b.start && last.end === b.end) {
        last.weekdays = [...last.weekdays, ...b.weekdays];
      } else {
        grouped.push({ ...b, weekdays: [...b.weekdays] });
      }
    }
    return grouped;
  }

  // ---- summary in plain language ----
  function renderSummary() {
    const slot = document.querySelector('[data-fill="summary-card"]');
    if (!slot) return;
    const blocks = gridToBlocks();
    const byKind = { school: [], blocked: [], bedtime: [], free: [] };
    for (const b of blocks) (byKind[b.kind] || []).push(b);

    function totalHours(kind) {
      let mins = 0;
      for (let d = 0; d < 7; d++) for (let h = 0; h < HOURS; h++) if (cells[d][h] === kind) mins += 60;
      return mins / 60;
    }
    const labels = { school: "School", blocked: "No screens", bedtime: "Bedtime" };
    const freeHrs = 24 * 7 - (totalHours("school") + totalHours("blocked") + totalHours("bedtime"));

    function describeKind(kind) {
      const list = byKind[kind];
      if (!list.length) return null;
      const rangeText = list.map((b) => {
        const wd = b.weekdays.length === 7 ? "every day"
                 : b.weekdays.length === 5 && [1,2,3,4,5].every((d) => b.weekdays.includes(d)) ? "weekdays"
                 : b.weekdays.length === 2 && b.weekdays.includes(0) && b.weekdays.includes(6) ? "weekends"
                 : b.weekdays.map((d) => ["Sun","Mon","Tue","Wed","Thu","Fri","Sat"][d]).join(", ");
        const start = b.start.replace(/^0/, "");
        const end = (b.end === "00:00" ? "midnight" : b.end.replace(/^0/, ""));
        return `${wd} ${start}–${end}`;
      });
      return rangeText.join("; ");
    }

    const rows = [];
    for (const kind of ["school", "blocked", "bedtime"]) {
      const desc = describeKind(kind);
      if (!desc) continue;
      rows.push(`
        <div class="summary-row">
          <span><span class="chip ${kind}"></span>${labels[kind]}</span>
          <span>${H.escapeHTML(desc)}</span>
          <span class="num">${totalHours(kind).toFixed(0)}h / wk</span>
        </div>`);
    }
    rows.push(`
      <div class="summary-row">
        <span><span class="chip free"></span>Free time</span>
        <span>What's left, capped by the daily budget.</span>
        <span class="num">${freeHrs.toFixed(0)}h / wk</span>
      </div>`);

    slot.innerHTML = `
      <div class="card-head" style="margin-bottom: 4px;"><h3>In plain language</h3></div>
      ${rows.join("")}`;
  }

  // ---- save / discard ----
  const buttons = document.querySelectorAll(".save-bar .btn");
  let discardBtn = null, saveBtn = null;
  buttons.forEach((b) => {
    if (b.textContent.trim() === "Discard") discardBtn = b;
    if (b.textContent.trim().startsWith("Save")) saveBtn = b;
  });
  if (discardBtn) discardBtn.addEventListener("click", () => location.reload());
  if (saveBtn) {
    saveBtn.addEventListener("click", async () => {
      saveBtn.disabled = true;
      const original = saveBtn.textContent;
      saveBtn.textContent = "Saving…";
      try {
        const blocks = gridToBlocks();
        const next = {
          timezone: (policy && policy.timezone) || "UTC",
          schedule: { blocks },
        };
        if (policy) {
          if (policy.time_budget) next.time_budget = policy.time_budget;
          if (policy.content)     next.content     = policy.content;
          if (policy.monitoring)  next.monitoring  = policy.monitoring;
        }
        await H.api("PUT", `/api/people/${encodeURIComponent(personID)}/policy`, next);
        saveBtn.textContent = "Saved ✓";
        setTimeout(() => { saveBtn.textContent = original; saveBtn.disabled = false; }, 1200);
      } catch (e) {
        alert("Couldn't save: " + e.message);
        saveBtn.disabled = false;
        saveBtn.textContent = original;
      }
    });
  }
})();
