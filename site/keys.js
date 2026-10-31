(() => {
  const shortcuts = new Map();
  document.querySelectorAll('[data-key]').forEach((element) => {
    const key = element.dataset.key.toLowerCase();
    if (!shortcuts.has(key)) shortcuts.set(key, element);
  });

  document.addEventListener('keydown', (event) => {
    if (event.metaKey || event.ctrlKey || event.altKey) return;
    const target = event.target;
    if (target instanceof HTMLElement && target.closest('input, textarea, select, [contenteditable="true"]')) return;
    const element = shortcuts.get(event.key.toLowerCase());
    if (!element) return;
    const href = element.getAttribute('href');
    if (!href) return;
    event.preventDefault();
    if (href.startsWith('#')) {
      document.querySelector(href)?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    } else {
      window.location.href = href;
    }
  });
})();
