(async () => {
  const H = window.Hearth;

  // If we already have an account, first-run should be unreachable.
  const me = await fetch("/api/auth/me").then((r) => r.json());
  if (me.setup_complete) {
    location.replace(me.authenticated ? "/home" : "/login");
    return;
  }

  // The mockup first-run has a 3-step layout. Wave 1 collapses it into one
  // form: parent account + first child. The remaining steps (discovery,
  // policy preset) link to /devices and /policy-time after setup.
  const steps = document.querySelectorAll(".steps .step");
  if (steps.length) {
    const formHTML = `
      <div class="step current" id="combined-step">
        <div class="step-num">1</div>
        <div class="step-text">
          <h3>Create your parent account &amp; add a child</h3>
          <p>This is the only account that can be created without an existing session. The child's name is just a label — you can add more later.</p>
          <form id="setup-form" style="margin-top:14px; display: grid; gap: 10px; max-width: 340px;">
            <label style="font-size: 12.5px; color: var(--muted); display: grid; gap: 4px;">
              Parent username
              <input name="username" required autocomplete="username" />
            </label>
            <label style="font-size: 12.5px; color: var(--muted); display: grid; gap: 4px;">
              Parent password (min 8 chars)
              <input name="password" type="password" required autocomplete="new-password" minlength="8" />
            </label>
            <label style="font-size: 12.5px; color: var(--muted); display: grid; gap: 4px;">
              First child's name
              <input name="child" required />
            </label>
            <div id="setup-err" style="color: oklch(38% 0.13 25); font-size: 12.5px; min-height: 0;" hidden></div>
            <button class="btn btn-primary" type="submit" style="margin-top: 4px; justify-self: start;">Finish setup</button>
          </form>
        </div>
      </div>`;
    const stepsEl = document.querySelector(".steps");
    if (stepsEl) {
      stepsEl.innerHTML = formHTML;
    }
  }

  // The mockup's "We found N devices" copy is aspirational — /api/devices is
  // session-gated, and there's no session during first-run. Leaving the
  // placeholder copy in place is correct here; the real count appears on /home
  // after setup.

  const form = document.getElementById("setup-form");
  const err = document.getElementById("setup-err");
  if (!form) return;
  form.addEventListener("submit", async (ev) => {
    ev.preventDefault();
    err.hidden = true;
    const data = Object.fromEntries(new FormData(form));
    try {
      const setupRes = await fetch("/api/auth/setup", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: data.username, password: data.password }),
      });
      if (!setupRes.ok) {
        const b = await setupRes.json().catch(() => ({}));
        throw new Error(b.error || `HTTP ${setupRes.status}`);
      }
      await H.api("POST", "/api/people", { name: data.child });
      location.replace("/home");
    } catch (e) {
      err.textContent = e.message;
      err.hidden = false;
    }
  });
})();
