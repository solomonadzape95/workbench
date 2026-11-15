(() => {
  const endpoint = "https://api.github.com/repos/solomonadzape95/workbench/releases/latest";
  let pending;

  function load() {
    if (!pending) {
      pending = fetch(endpoint, { headers: { Accept: "application/vnd.github+json" } })
        .then((response) => (response.ok ? response.json() : null))
        .catch(() => null);
    }
    return pending;
  }

  function size(bytes) {
    if (!Number.isFinite(bytes) || bytes <= 0) return "";
    return (bytes / (1024 * 1024)).toFixed(1) + " MB";
  }

  function platform(name) {
    const lower = String(name || "").toLowerCase();
    const arch = lower.includes("arm64") ? "arm64" : lower.includes("amd64") ? "amd64" : "";
    if (!arch) return "";
    if (lower.includes("darwin")) return "darwin / " + arch;
    if (lower.includes("linux")) return "linux / " + arch;
    return "";
  }

  function assetRow(asset) {
    const label = platform(asset.name);
    if (!label) return null;
    const link = document.createElement("a");
    link.href = asset.browser_download_url;
    link.target = "_blank";
    link.rel = "noreferrer";

    const left = document.createElement("span");
    left.textContent = label;
    const right = document.createElement("span");
    right.textContent = size(asset.size);
    link.append(left, right);
    return link;
  }

  async function enhance(root) {
    const version = root.querySelector("#release-version");
    const summary = root.querySelector("#release-summary");
    const assets = root.querySelector("#release-assets");
    const checksums = root.querySelector("#release-checksums");
    if (!version || !assets) return;

    const release = await load();
    if (!release || !release.tag_name) return;

    version.textContent = release.tag_name;
    if (summary) {
      summary.textContent = release.name && release.name !== release.tag_name
        ? release.name
        : "Latest release from GitHub.";
    }

    const rows = (release.assets || [])
      .filter((asset) => asset.name && asset.name.endsWith(".tar.gz"))
      .map(assetRow)
      .filter(Boolean);
    if (rows.length > 0) assets.replaceChildren(...rows);

    const sum = (release.assets || []).find((asset) => asset.name === "checksums.txt");
    if (sum && checksums) checksums.href = sum.browser_download_url;
  }

  window.workbenchEnhanceRelease = enhance;
})();
