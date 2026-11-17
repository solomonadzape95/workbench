(() => {
  const endpoint = "https://api.github.com/repos/solomonadzape95/workbench/releases?per_page=100";
  let pending;

  function platform(name) {
    const lower = String(name || "").toLowerCase();
    const arch = lower.includes("arm64") ? "arm64" : lower.includes("amd64") ? "amd64" : "";
    if (!arch) return "";
    if (lower.includes("darwin")) return "macOS / " + arch;
    if (lower.includes("linux")) return "Linux / " + arch;
    return "";
  }

  function isBinary(name) {
    const lower = String(name || "").toLowerCase();
    return /\.(tar\.gz|tgz|zip|deb|rpm|dmg|exe)$/.test(lower);
  }

  function load() {
    if (!pending) {
      pending = fetch(endpoint, { headers: { Accept: "application/vnd.github+json" } })
        .then((response) => (response.ok ? response.json() : []))
        .catch(() => []);
    }
    return pending;
  }

  async function enhance(root) {
    const slot = root.querySelector("#release-downloads");
    if (!slot) return;

    const releases = await load();
    let total = 0;
    const platforms = new Map();

    for (const release of releases) {
      if (!release || release.draft) continue;
      for (const asset of release.assets || []) {
        if (!isBinary(asset.name)) continue;
        const count = Number(asset.download_count) || 0;
        total += count;
        const label = platform(asset.name);
        if (label) platforms.set(label, (platforms.get(label) || 0) + count);
      }
    }

    if (!total) {
      slot.hidden = true;
      return;
    }

    const split = [...platforms.entries()]
      .sort((a, b) => b[1] - a[1])
      .map(([label, count]) => label + " " + count.toLocaleString())
      .join(" · ");

    slot.textContent =
      total.toLocaleString() + " downloads across all releases" + (split ? " · " + split : "");
    slot.hidden = false;
  }

  window.workbenchEnhanceFunnel = enhance;
})();
