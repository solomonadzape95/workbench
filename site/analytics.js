(() => {
  const meta = document.querySelector('meta[name="wb-analytics"]');
  const value = meta ? (meta.getAttribute("content") || "").trim() : "";
  if (!value) return;

  const host = /^https?:\/\//.test(value)
    ? value.replace(/\/+$/, "")
    : "https://" + value + ".goatcounter.com";

  window.workbenchTrack = (name) => {
    const counter = window.goatcounter;
    if (!name || !counter || typeof counter.count !== "function") return;
    counter.count({ path: name, title: name, event: true });
  };

  const script = document.createElement("script");
  script.async = true;
  script.src = "https://gc.zgo.at/count.js";
  script.dataset.goatcounter = host + "/count";
  document.head.appendChild(script);
})();
