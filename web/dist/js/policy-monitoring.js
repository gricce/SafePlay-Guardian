(async () => {
  const H = window.Hearth;
  await H.requireAuth();

  const personID = new URLSearchParams(location.search).get("id");
  if (!personID) {
    location.replace("/home");
    return;
  }

  // ---- load context ----
  let person, policy, peopleList;
  try {
    [person, peopleList] = await Promise.all([
      H.api("GET", `/api/people/${encodeURIComponent(personID)}`),
      H.api("GET", "/api/people").then((r) => r.people || []),
    ]);
  } catch (e) {
    if (e.status === 404) {
      location.replace("/home");
      return;
    }
    throw e;
  }
  try {
    policy = await H.api("GET", `/api/people/${encodeURIComponent(personID)}/policy`);
  } catch (e) {
    if (e.status !== 404) throw e;
    policy = null;
  }

  // ---- chrome ----
  H.renderFamilyNav(peopleList, personID);
  document.querySelectorAll('[data-fill="person-name"]').forEach((el) => {
    el.textContent = person.name;
  });
  const crumb = document.querySelector('[data-fill="breadcrumb-link"]');
  if (crumb) {
    crumb.textContent = person.name;
    crumb.href = `/child?id=${encodeURIComponent(personID)}`;
  }
  document.querySelectorAll('[data-fill="page-title"]').forEach((el) => {
    el.textContent = `How much do we monitor ${person.name}?`;
  });

  // ---- level selection ----
  const cards = document.querySelectorAll(".level-card[data-level]");
  const currentLevel = (policy && policy.monitoring) || "enforcement_only";
  function select(level) {
    cards.forEach((c) => c.classList.toggle("selected", c.dataset.level === level));
  }
  select(currentLevel);
  cards.forEach((c) => {
    c.addEventListener("click", () => select(c.dataset.level));
    c.addEventListener("keydown", (ev) => {
      if (ev.key === "Enter" || ev.key === " ") {
        ev.preventDefault();
        select(c.dataset.level);
      }
    });
  });

  // ---- save / discard ----
  function selectedLevel() {
    const el = document.querySelector(".level-card.selected");
    return el ? el.dataset.level : "enforcement_only";
  }

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
          monitoring: selectedLevel(),
        };
        if (policy) {
          if (policy.time_budget) next.time_budget = policy.time_budget;
          if (policy.schedule)    next.schedule    = policy.schedule;
          if (policy.content)     next.content     = policy.content;
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
