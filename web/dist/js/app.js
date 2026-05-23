// Shared client for every Hearth page. Pages load /js/app.js first, then
// their own /js/<page>.js which calls into Hearth.* once the DOM is ready.

(function (global) {
  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

  function escapeHTML(s) {
    return String(s ?? "").replace(/[&<>"']/g, (c) => ({
      "&": "&amp;", "<": "&lt;", ">": "&gt;", "\"": "&quot;", "'": "&#39;",
    }[c]));
  }

  async function api(method, path, body) {
    const opts = { method, headers: { Accept: "application/json" } };
    if (body !== undefined) {
      opts.headers["Content-Type"] = "application/json";
      opts.body = JSON.stringify(body);
    }
    const res = await fetch(path, opts);
    if (res.status === 401) {
      // Session expired — bounce to login. The redirect target is "this page"
      // so a refresh after sign-in lands the parent back where they were.
      const here = encodeURIComponent(location.pathname + location.search);
      location.replace(`/login?next=${here}`);
      throw new Error("unauthenticated");
    }
    const ct = res.headers.get("Content-Type") || "";
    const data = ct.includes("application/json") ? await res.json() : null;
    if (!res.ok) {
      const msg = (data && data.error) || `HTTP ${res.status}`;
      const err = new Error(msg);
      err.status = res.status;
      err.body = data;
      throw err;
    }
    return data;
  }

  // requireAuth ensures the page is reached by an authenticated parent.
  // Returns the User. If not signed in OR setup is incomplete, redirects.
  async function requireAuth() {
    const me = await fetch("/api/auth/me").then((r) => r.json());
    if (!me.authenticated) {
      if (!me.setup_complete) {
        location.replace("/first-run");
      } else {
        const here = encodeURIComponent(location.pathname + location.search);
        location.replace(`/login?next=${here}`);
      }
      // Throwing aborts the calling page's bootstrap.
      throw new Error("redirecting");
    }
    return me.user;
  }

  function colorClass(idx) {
    return "c" + (((idx % 3) + 3) % 3 + 1); // 1..3
  }
  function initialOf(name) {
    const s = String(name || "?").trim();
    return s ? s[0].toUpperCase() : "?";
  }
  function fmtDateLocale(date) {
    return date.toLocaleDateString(undefined, {
      weekday: "long", month: "long", day: "numeric",
    });
  }
  function fmtTimeShort(date) {
    return date.toLocaleTimeString(undefined, {
      hour: "numeric", minute: "2-digit",
    });
  }
  function fmtDuration(mins) {
    mins = Math.max(0, Math.round(mins));
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    if (h === 0) return `${m}m`;
    if (m === 0) return `${h}h`;
    return `${h}h ${String(m).padStart(2, "0")}m`;
  }

  // renderFamilyNav fills every [data-fill="family-nav"] slot with anchor
  // tags for each person. activeId highlights one.
  function renderFamilyNav(people, activeId) {
    const slots = $$('[data-fill="family-nav"]');
    if (!slots.length) return;
    if (!people || !people.length) {
      slots.forEach((slot) => { slot.innerHTML = ""; });
      return;
    }
    const sorted = people.slice().sort((a, b) => a.name.localeCompare(b.name));
    const html = sorted.map((p, i) => {
      const cls = colorClass(i);
      const active = p.id === activeId ? " class=\"active\"" : "";
      return `<a href="/child?id=${encodeURIComponent(p.id)}"${active}>` +
             `<span class="avatar sm ${cls}">${escapeHTML(initialOf(p.name))}</span> ${escapeHTML(p.name)}</a>`;
    }).join("\n");
    slots.forEach((slot) => { slot.innerHTML = html; });
  }

  async function logout() {
    try { await api("POST", "/api/auth/logout"); } catch (_) {}
    location.replace("/login");
  }

  function fill(name, value) {
    $$(`[data-fill="${name}"]`).forEach((el) => { el.textContent = value; });
  }
  function fillHTML(name, html) {
    $$(`[data-fill="${name}"]`).forEach((el) => { el.innerHTML = html; });
  }

  global.Hearth = {
    $, $$, escapeHTML, api, requireAuth,
    colorClass, initialOf,
    fmtDateLocale, fmtTimeShort, fmtDuration,
    renderFamilyNav, logout, fill, fillHTML,
  };
})(window);
