(() => {
  "use strict";

  // sha256("admin-wb-page"). Replace this constant to rotate the password.
  // This gate is obscurity, not security: the page and everything it shows are
  // public. Never put private data behind it.
  const PASSWORD_HASH = "00256c852d82d65e2bbe1194880a16b26d59597a0426b1f6a87f99cc3d0352ba";
  const SESSION_KEY = "wb-admin";

  const RELEASES_URL = "https://api.github.com/repos/solomonadzape95/workbench/releases?per_page=100";
  const BREW_URL = "https://formulae.brew.sh/api/analytics/install/";
  const TAP = "solomonadzape95/tap/workbench";
  const BREW_WINDOWS = ["30d", "90d", "365d"];

  const ACCENT = [245, 167, 66];
  const ACCENT_BRIGHT = "#ffc46b";
  const ACCENT_MID = "#a37130";
  const MUTED = "#7c8792";
  const DIM = "#4d5a64";
  const BORDER = "#1c272f";
  const TEXT = "#d7dde3";
  const FONT = '"Maple Mono", "SFMono-Regular", Menlo, monospace';

  const BAYER = [
    0, 32, 8, 40, 2, 34, 10, 42,
    48, 16, 56, 24, 50, 18, 58, 26,
    12, 44, 4, 36, 14, 46, 6, 38,
    60, 28, 52, 20, 62, 30, 54, 22,
    3, 35, 11, 43, 1, 33, 9, 41,
    51, 19, 59, 27, 49, 17, 57, 25,
    15, 47, 7, 39, 13, 45, 5, 37,
    63, 31, 55, 23, 61, 29, 53, 21,
  ];

  let state = null;
  let brew = null;

  /* ---------------------------------------------------------------- helpers */

  async function sha256(value) {
    const bytes = new TextEncoder().encode(value);
    const digest = await crypto.subtle.digest("SHA-256", bytes);
    return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, "0")).join("");
  }

  async function getJSON(url) {
    const response = await fetch(url, { headers: { Accept: "*/*" } });
    if (!response.ok) throw new Error(String(response.status));
    return response.json();
  }

  function font(dpr, size, weight) {
    return (weight || 400) + " " + Math.round(size * dpr) + "px " + FONT;
  }

  function isBinary(name) {
    return /\.(tar\.gz|tgz|zip|deb|rpm|dmg|exe)$/i.test(String(name || ""));
  }

  function platformOf(name) {
    const lower = String(name || "").toLowerCase();
    const arch = lower.includes("arm64") ? "arm64" : lower.includes("amd64") ? "amd64" : "";
    if (!arch) return "";
    if (lower.includes("darwin")) return "macOS / " + arch;
    if (lower.includes("linux")) return "Linux / " + arch;
    return "";
  }

  function summarize(releases) {
    const items = [];
    const platforms = new Map();
    for (const release of releases) {
      if (!release || release.draft) continue;
      let downloads = 0;
      for (const asset of release.assets || []) {
        if (!isBinary(asset.name)) continue;
        const count = Number(asset.download_count) || 0;
        downloads += count;
        const label = platformOf(asset.name);
        if (label) platforms.set(label, (platforms.get(label) || 0) + count);
      }
      items.push({ tag: release.tag_name, date: release.published_at, downloads });
    }

    items.sort((a, b) => new Date(a.date) - new Date(b.date));

    let running = 0;
    const cumulative = items.map((item) => (running += item.downloads));

    return {
      items,
      cumulative,
      total: running,
      platforms: [...platforms.entries()]
        .sort((a, b) => b[1] - a[1])
        .map(([label, value]) => ({ label, value })),
    };
  }

  /* ------------------------------------------------------------------ kpis */

  function prettyDate(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value || "—";
    return date.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
  }

  function drawKpis() {
    const root = document.getElementById("admin-kpis");
    if (!root) return;
    const latest = state.items.length ? state.items[state.items.length - 1].tag : "—";

    const entries = [
      { key: "downloads", label: "downloads", value: state.total.toLocaleString() },
      { key: "releases", label: "releases", value: String(state.items.length) },
      { key: "latest", label: "latest", value: latest },
      { key: "brew", label: "brew 30d", value: "…" },
    ];

    root.replaceChildren(
      ...entries.map((entry) => {
        const cell = document.createElement("div");
        cell.className = "admin-kpi";
        cell.dataset.key = entry.key;
        const label = document.createElement("span");
        label.textContent = entry.label;
        const value = document.createElement("strong");
        value.textContent = entry.value;
        cell.append(label, value);
        return cell;
      })
    );
  }

  function setKpi(key, value) {
    const node = document.querySelector('.admin-kpi[data-key="' + key + '"] strong');
    if (node) node.textContent = value;
  }

  /* ---------------------------------------------------------------- charts */

  function mount(id) {
    const canvas = document.getElementById(id);
    if (!canvas) return null;
    const cssWidth = canvas.clientWidth || 720;
    const cssHeight = canvas.clientHeight || 190;
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    canvas.width = Math.max(1, Math.round(cssWidth * dpr));
    canvas.height = Math.max(1, Math.round(cssHeight * dpr));
    const ctx = canvas.getContext("2d");
    if (!ctx) return null;
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    return { ctx, w: canvas.width, h: canvas.height, dpr };
  }

  // Dither painter: writes cells into one ImageData buffer, then commits it in a
  // single putImageData. Anything drawn after commit() stays crisp.
  function painter(ctx, w, h, dpr) {
    const image = ctx.createImageData(w, h);
    const data = image.data;
    const cell = Math.max(2, Math.round(2.5 * dpr));

    function cells(x, y, width, height, density) {
      if (width <= 0 || height <= 0) return;
      const x0 = Math.max(0, Math.floor(x / cell));
      const y0 = Math.max(0, Math.floor(y / cell));
      const x1 = Math.min(Math.ceil(w / cell), Math.ceil((x + width) / cell));
      const y1 = Math.min(Math.ceil(h / cell), Math.ceil((y + height) / cell));

      for (let gy = y0; gy < y1; gy++) {
        const py = gy * cell;
        if (py >= y + height) continue;
        for (let gx = x0; gx < x1; gx++) {
          const px = gx * cell;
          if (px >= x + width) continue;
          const level = typeof density === "function" ? density(py) : density;
          if (level <= 0) continue;
          const threshold = (BAYER[(gy & 7) * 8 + (gx & 7)] + 0.5) / 64;
          if (threshold > level) continue;

          const bottom = Math.min(py + cell, h);
          const right = Math.min(px + cell, w);
          for (let yy = py; yy < bottom; yy++) {
            const row = yy * w;
            for (let xx = px; xx < right; xx++) {
              const index = (row + xx) * 4;
              data[index] = ACCENT[0];
              data[index + 1] = ACCENT[1];
              data[index + 2] = ACCENT[2];
              data[index + 3] = 255;
            }
          }
        }
      }
    }

    return {
      cells,
      commit() {
        ctx.putImageData(image, 0, 0);
      },
    };
  }

  function emptyState(view, message) {
    const { ctx, w, h, dpr } = view;
    ctx.fillStyle = DIM;
    ctx.font = font(dpr, 11);
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    ctx.fillText(message, w / 2, h / 2);
  }

  function barChart(id, items, emptyMessage) {
    const view = mount(id);
    if (!view) return;
    const { ctx, w, h, dpr } = view;
    const max = Math.max(0, ...items.map((item) => item.value));

    if (!items.length || max === 0) {
      emptyState(view, items.length ? emptyMessage : "no data");
      return;
    }

    const padL = 8 * dpr;
    const padR = 8 * dpr;
    const padT = 30 * dpr;
    const base = h - 30 * dpr;
    const plotH = base - padT;
    const plotW = w - padL - padR;
    const slot = plotW / items.length;
    const barW = Math.min(64 * dpr, slot * 0.52);

    const paint = painter(ctx, w, h, dpr);
    items.forEach((item, index) => {
      const center = padL + slot * (index + 0.5);
      const barH = Math.max(2 * dpr, (item.value / max) * plotH);
      paint.cells(center - barW / 2, base - barH, barW, barH, (py) => {
        const ratio = (base - py) / Math.max(1, plotH);
        return 0.82 - 0.42 * ratio;
      });
    });
    paint.commit();

    ctx.strokeStyle = BORDER;
    ctx.lineWidth = Math.max(1, dpr);
    ctx.beginPath();
    ctx.moveTo(0, base);
    ctx.lineTo(w, base);
    ctx.stroke();

    items.forEach((item, index) => {
      const center = padL + slot * (index + 0.5);
      const barH = Math.max(2 * dpr, (item.value / max) * plotH);

      ctx.fillStyle = ACCENT_BRIGHT;
      ctx.font = font(dpr, 11, 500);
      ctx.textAlign = "center";
      ctx.textBaseline = "bottom";
      ctx.fillText(item.value.toLocaleString(), center, base - barH - 6 * dpr);

      ctx.fillStyle = MUTED;
      ctx.font = font(dpr, 10);
      ctx.textBaseline = "top";
      ctx.fillText(item.label, center, base + 8 * dpr);
    });
  }

  function hBarChart(id, items, emptyMessage) {
    const view = mount(id);
    if (!view) return;
    const { ctx, w, h, dpr } = view;
    const max = Math.max(0, ...items.map((item) => item.value));

    if (!items.length || max === 0) {
      emptyState(view, items.length ? emptyMessage : "no data");
      return;
    }

    const labelW = Math.min(w * 0.34, 132 * dpr);
    const padR = 58 * dpr;
    const plotW = w - labelW - padR;
    const rowH = h / items.length;
    const barH = Math.min(16 * dpr, rowH * 0.46);

    const paint = painter(ctx, w, h, dpr);
    items.forEach((item, index) => {
      const center = rowH * (index + 0.5);
      const width = Math.max(2 * dpr, (item.value / max) * plotW);
      paint.cells(labelW, center - barH / 2, width, barH, 0.72);
    });
    paint.commit();

    items.forEach((item, index) => {
      const center = rowH * (index + 0.5);
      const width = Math.max(2 * dpr, (item.value / max) * plotW);

      ctx.fillStyle = MUTED;
      ctx.font = font(dpr, 10);
      ctx.textAlign = "left";
      ctx.textBaseline = "middle";
      ctx.fillText(item.label, 0, center);

      ctx.fillStyle = TEXT;
      ctx.font = font(dpr, 11);
      ctx.fillText(item.value.toLocaleString(), labelW + width + 10 * dpr, center);
    });
  }

  function stepChart(id, points, emptyMessage) {
    const view = mount(id);
    if (!view) return;
    const { ctx, w, h, dpr } = view;

    if (!points.length) {
      emptyState(view, emptyMessage);
      return;
    }

    const padL = 34 * dpr;
    const padR = 12 * dpr;
    const padT = 18 * dpr;
    const base = h - 44 * dpr;
    const plotH = base - padT;
    const plotW = w - padL - padR;
    const max = Math.max(1, points[points.length - 1].total);

    const times = points.map((point) => new Date(point.date).getTime());
    const first = Math.min(...times);
    // The last step runs from the newest release to now, so the chart ends on
    // the current cumulative total instead of collapsing to the release date.
    const end = Math.max(Date.now(), ...times);
    const span = end - first;
    const xFor = (time) => (span > 0 ? padL + ((time - first) / span) * plotW : padL + plotW / 2);
    const xs = times.map(xFor);
    const rightEdge = xFor(end);
    const ys = points.map((point) => base - (point.total / max) * plotH);

    const paint = painter(ctx, w, h, dpr);
    points.forEach((point, index) => {
      const x = xs[index];
      const next = index + 1 < xs.length ? xs[index + 1] : rightEdge;
      paint.cells(x, ys[index], Math.max(2 * dpr, next - x), base - ys[index], (py) => {
        const ratio = (base - py) / Math.max(1, plotH);
        return 0.28 + 0.5 * ratio;
      });
    });
    paint.commit();

    ctx.strokeStyle = BORDER;
    ctx.lineWidth = Math.max(1, dpr);
    ctx.beginPath();
    ctx.moveTo(padL, base);
    ctx.lineTo(w - padR, base);
    ctx.stroke();

    ctx.strokeStyle = ACCENT_MID;
    ctx.beginPath();
    ctx.moveTo(xs[0], ys[0]);
    for (let index = 0; index < xs.length; index++) {
      if (index > 0) ctx.lineTo(xs[index], ys[index - 1]);
      ctx.lineTo(xs[index], ys[index]);
    }
    ctx.lineTo(rightEdge, ys[ys.length - 1]);
    ctx.stroke();

    ctx.fillStyle = DIM;
    ctx.font = font(dpr, 10);
    ctx.textAlign = "right";
    ctx.textBaseline = "middle";
    ctx.fillText(String(max), padL - 8 * dpr, padT);
    ctx.fillText("0", padL - 8 * dpr, base);

    points.forEach((point, index) => {
      const x = xs[index];
      const y = base + 8 * dpr + (index % 2) * 13 * dpr;

      ctx.fillStyle = BORDER;
      ctx.fillRect(x, base, Math.max(1, dpr), 5 * dpr);

      ctx.fillStyle = MUTED;
      ctx.font = font(dpr, 10);
      ctx.textAlign = index === points.length - 1 ? "right" : "center";
      ctx.textBaseline = "top";
      ctx.fillText(point.label, x, y);
    });
  }

  function drawCharts() {
    if (!state) return;

    stepChart(
      "chart-timeline",
      state.items.map((item, index) => ({ label: item.tag, date: item.date, total: state.cumulative[index] })),
      "no downloads recorded yet"
    );
    barChart(
      "chart-releases",
      state.items.map((item) => ({ label: item.tag, value: item.downloads })),
      "no downloads recorded yet"
    );
    hBarChart(
      "chart-platforms",
      state.platforms.map((item) => ({ label: item.label.replace(" / ", " "), value: item.value })),
      "no downloads recorded yet"
    );

    if (brew) {
      barChart("chart-brew", brew, "no tap installs recorded yet");
    }
  }

  /* ------------------------------------------------------------------- data */

  async function loadBrew() {
    const results = await Promise.all(
      BREW_WINDOWS.map(async (window) => {
        try {
          const data = await getJSON(BREW_URL + window + ".json");
          const entry = (data.formulae || []).find((item) => item.formula === TAP);
          return { window, value: entry ? Number(entry.count) || 0 : 0 };
        } catch (error) {
          return { window, value: null };
        }
      })
    );

    const note = document.getElementById("brew-note");
    if (results.every((result) => result.value === null)) {
      if (note) note.textContent = "Homebrew analytics could not be read from this browser.";
      setKpi("brew", "—");
      return;
    }

    brew = results.map((result) => ({ label: result.window, value: result.value || 0 }));
    setKpi("brew", brew[0].value.toLocaleString());

    const total = brew.reduce((sum, item) => sum + item.value, 0);
    if (note && total === 0) {
      note.textContent = "No tap installs recorded yet. Homebrew only lists formulas installed at least once.";
    }

    if (state) barChart("chart-brew", brew, "no tap installs recorded yet");
  }

  async function load() {
    const sub = document.getElementById("admin-sub");
    let releases;
    try {
      releases = await getJSON(RELEASES_URL);
    } catch (error) {
      if (sub) sub.textContent = "Could not read GitHub releases (" + error.message + "). Reload in a minute.";
      return;
    }

    state = summarize(Array.isArray(releases) ? releases : []);
    if (sub) sub.textContent = "Installs and downloads, read from GitHub and Homebrew.";

    const updated = document.getElementById("admin-updated");
    if (updated) updated.textContent = new Date().toLocaleString();

    drawKpis();
    drawCharts();
    loadBrew();
  }

  /* ------------------------------------------------------------------- gate */

  function unlocked() {
    const gate = document.getElementById("gate");
    const dashboard = document.getElementById("dashboard");
    if (gate) gate.hidden = true;
    if (dashboard) dashboard.hidden = false;

    load();

    if (document.fonts && document.fonts.ready) {
      document.fonts.ready.then(() => {
        if (state) drawCharts();
      });
    }
  }

  function wire() {
    const form = document.getElementById("gate-form");
    const input = document.getElementById("gate-password");
    const error = document.getElementById("gate-error");

    if (form) {
      form.addEventListener("submit", async (event) => {
        event.preventDefault();
        if (!window.crypto || !window.crypto.subtle) {
          if (error) error.textContent = "open this page over https";
          return;
        }
        const hash = await sha256((input ? input.value : "").trim());
        if (hash === PASSWORD_HASH) {
          try {
            sessionStorage.setItem(SESSION_KEY, PASSWORD_HASH);
          } catch (storageError) {
            // Private mode with storage disabled still unlocks for this page view.
          }
          if (error) error.textContent = "";
          unlocked();
          return;
        }
        if (error) error.textContent = "wrong password";
        if (input) {
          input.value = "";
          input.focus();
        }
      });
    }

    let stored = null;
    try {
      stored = sessionStorage.getItem(SESSION_KEY);
    } catch (storageError) {
      stored = null;
    }
    if (stored === PASSWORD_HASH) {
      unlocked();
    } else if (input) {
      input.focus();
    }

    let timer = 0;
    window.addEventListener("resize", () => {
      const dashboard = document.getElementById("dashboard");
      if (!dashboard || dashboard.hidden) return;
      window.clearTimeout(timer);
      timer = window.setTimeout(drawCharts, 160);
    });
  }

  wire();
})();
