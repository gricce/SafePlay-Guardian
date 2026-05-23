(async () => {
  const H = window.Hearth;
  await H.requireAuth();

  const personID = new URLSearchParams(location.search).get("id");
  if (!personID) {
    location.replace("/home");
    return;
  }

  // ---- load context ----
  let person, policy, peopleList, allDevices;
  try {
    [person, peopleList, allDevices] = await Promise.all([
      H.api("GET", `/api/people/${encodeURIComponent(personID)}`),
      H.api("GET", "/api/people").then((r) => r.people || []),
      H.api("GET", "/api/devices").then((r) => r.devices || []),
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
  const myDevices = allDevices.filter((d) => d.person_id === personID && d.state !== "retired");

  H.renderFamilyNav(peopleList, personID);
  document.querySelectorAll('[data-fill="person-name"]').forEach((el) => { el.textContent = person.name; });
  const crumb = document.querySelector('[data-fill="breadcrumb-link"]');
  if (crumb) {
    crumb.textContent = person.name;
    crumb.href = `/child?id=${encodeURIComponent(personID)}`;
  }

  // ---- model ----
  // Internal storage: minutes_by_weekday in Go convention: Sun=0..Sat=6.
  // The slider UI runs Mon..Sun (mockup order). We translate on display.
  // Slider scale: hours, 0..6, step 0.25 (15 min).
  const goFromUi = [1, 2, 3, 4, 5, 6, 0]; // ui[i] → goDay
  let minutes = (policy && policy.time_budget && policy.time_budget.minutes_by_weekday)
    ? policy.time_budget.minutes_by_weekday.slice()
    : [120, 120, 120, 120, 120, 120, 120];
  // Same/Per-day starts on Same when all 7 are equal.
  let mode = minutes.every((m) => m === minutes[0]) ? "same" : "per-day";

  // ---- DOM refs ----
  const tabSame = document.getElementById("tab-same");
  const tabEach = document.getElementById("tab-each");
  const dayList = document.getElementById("dayList");
  const inputs = document.querySelectorAll(".budget-input .field");
  const hoursEl = inputs[0];
  const minsEl  = inputs[1];

  function applyView() {
    if (mode === "same") {
      tabSame.classList.add("active");
      tabEach.classList.remove("active");
      dayList.classList.remove("show");
    } else {
      tabEach.classList.add("active");
      tabSame.classList.remove("active");
      dayList.classList.add("show");
    }
  }

  function setSameInputs(mins) {
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    hoursEl.value = String(h);
    minsEl.value = String(m).padStart(2, "0");
  }

  function renderSliders() {
    // Re-render the per-day row values from `minutes`.
    document.querySelectorAll(".day-row").forEach((row, uiIdx) => {
      const slider = row.querySelector('input[type="range"]');
      const valueEl = row.querySelector(".day-value");
      const goDay = goFromUi[uiIdx];
      const mins = minutes[goDay];
      slider.value = String(mins / 60);
      valueEl.textContent = formatHM(mins);
    });
    renderPreview();
  }

  function formatHM(mins) {
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    return m === 0 ? `${h}h 00m` : `${h}h ${String(m).padStart(2, "0")}m`;
  }

  function renderPreview() {
    const bars = document.querySelectorAll(".preview-bars > div");
    const max = Math.max(60, ...minutes);
    bars.forEach((bar, uiIdx) => {
      const goDay = goFromUi[uiIdx];
      const mins = minutes[goDay];
      const pct = Math.round((mins / max) * 100);
      bar.style.height = `${pct}%`;
      bar.dataset.h = String(Math.round(mins / 60 * 10) / 10);
      bar.classList.toggle("weekend", goDay === 0 || goDay === 6);
    });
    const totals = document.querySelectorAll(".preview-totals .row .num");
    const total = minutes.reduce((a, b) => a + b, 0);
    const weekday = [minutes[1], minutes[2], minutes[3], minutes[4], minutes[5]];
    const weekend = [minutes[0], minutes[6]];
    const avg = (arr) => arr.reduce((a, b) => a + b, 0) / arr.length;
    if (totals[0]) totals[0].textContent = formatHM(total);
    if (totals[1]) totals[1].textContent = formatHM(Math.round(avg(weekday)));
    if (totals[2]) totals[2].textContent = formatHM(Math.round(avg(weekend)));
  }

  // ---- initial render ----
  applyView();
  setSameInputs(minutes[0]);
  renderSliders();
  renderGapNote();

  // ---- gap note: device(s) without an agent + a time-budget rule ----
  function renderGapNote() {
    const slot = document.querySelector('[data-fill="gap-note"]');
    if (!slot) return;
    const total = minutes.reduce((a, b) => a + b, 0);
    if (total === 0) { slot.innerHTML = ""; return; }
    const netOnly = myDevices.filter((d) => !d.agent_token && d.state !== "paused");
    if (!netOnly.length) { slot.innerHTML = ""; return; }
    const list = netOnly.map((d) => H.escapeHTML(d.label)).join(", ");
    slot.innerHTML = `
      <div class="gap-note">
        <svg class="gap-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M12 8v5M12 16.5v0.1"/></svg>
        <div>
          <strong>Time limits can't apply to ${list}.</strong><br>
          ${netOnly.length === 1 ? "This device has" : "These devices have"} no Hearth agent installed, so we can't measure their screen time. <a href="/devices">See what's covered.</a>
        </div>
      </div>`;
  }

  // ---- wire interactions ----
  tabSame.addEventListener("click", () => {
    mode = "same";
    setSameInputs(minutes[0]);
    applyView();
  });
  tabEach.addEventListener("click", () => {
    mode = "per-day";
    applyView();
  });

  function onSameInput() {
    const h = Math.max(0, Math.min(23, parseInt(hoursEl.value, 10) || 0));
    const m = Math.max(0, Math.min(59, parseInt(minsEl.value, 10) || 0));
    const mins = h * 60 + m;
    minutes = minutes.map(() => mins);
    renderSliders();
  }
  [hoursEl, minsEl].forEach((el) => {
    el.addEventListener("input", onSameInput);
    el.addEventListener("blur", () => {
      // Re-normalize zero-padding on minutes
      setSameInputs(minutes[0]);
    });
  });

  document.querySelectorAll(".day-row").forEach((row, uiIdx) => {
    const slider = row.querySelector('input[type="range"]');
    const valueEl = row.querySelector(".day-value");
    slider.addEventListener("input", () => {
      const mins = Math.round(parseFloat(slider.value) * 60);
      minutes[goFromUi[uiIdx]] = mins;
      valueEl.textContent = formatHM(mins);
      renderPreview();
      renderGapNote();
    });
  });

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
        const next = {
          timezone: (policy && policy.timezone) || "UTC",
          time_budget: { minutes_by_weekday: minutes },
        };
        if (policy) {
          if (policy.schedule)   next.schedule   = policy.schedule;
          if (policy.content)    next.content    = policy.content;
          if (policy.monitoring) next.monitoring = policy.monitoring;
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
