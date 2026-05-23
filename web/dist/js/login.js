(async () => {
  const H = window.Hearth;
  const form = document.getElementById("login-form");
  const err = document.getElementById("login-err");

  // If somehow we hit /login while setup isn't done, send to first-run instead.
  const me = await fetch("/api/auth/me").then((r) => r.json());
  if (me.authenticated) {
    const next = new URLSearchParams(location.search).get("next");
    location.replace(next || "/home");
    return;
  }
  if (!me.setup_complete) {
    location.replace("/first-run");
    return;
  }

  function showErr(msg) {
    err.textContent = msg;
    err.hidden = false;
  }

  form.addEventListener("submit", async (ev) => {
    ev.preventDefault();
    err.hidden = true;
    const data = Object.fromEntries(new FormData(form));
    try {
      const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (res.status === 401) {
        showErr("Wrong username or password.");
        return;
      }
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        showErr(body.error || `HTTP ${res.status}`);
        return;
      }
      const next = new URLSearchParams(location.search).get("next");
      location.replace(next || "/home");
    } catch (e) {
      showErr(e.message);
    }
  });
})();
