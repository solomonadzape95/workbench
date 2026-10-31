(() => {
  async function writeClipboard(text) {
    if (navigator.clipboard && window.isSecureContext) {
      try {
        await navigator.clipboard.writeText(text);
        return;
      } catch {
        // Fall through to the textarea fallback.
      }
    }
    const area = document.createElement('textarea');
    area.value = text;
    area.setAttribute('readonly', '');
    area.style.position = 'fixed';
    area.style.opacity = '0';
    document.body.appendChild(area);
    area.select();
    const ok = document.execCommand('copy');
    area.remove();
    if (!ok) throw new Error('copy failed');
  }

  function feedback(button, label) {
    const original = button.dataset.label || button.textContent;
    button.dataset.label = original;
    button.textContent = label;
    window.setTimeout(() => {
      button.textContent = original;
    }, 1200);
  }

  function makeButton(getText) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'copy code-copy';
    button.textContent = 'copy';
    button.setAttribute('aria-label', 'Copy code');
    button.addEventListener('click', async () => {
      try {
        await writeClipboard(getText());
        feedback(button, 'copied');
      } catch {
        feedback(button, 'failed');
      }
    });
    return button;
  }

  function enhance(root) {
    root.querySelectorAll('pre').forEach((pre) => {
      if (pre.dataset.copyReady) return;
      const code = pre.querySelector('code');
      if (!code) return;
      const wrap = document.createElement('div');
      wrap.className = 'code-block';
      pre.parentNode.insertBefore(wrap, pre);
      wrap.appendChild(pre);
      wrap.appendChild(makeButton(() => code.textContent));
      pre.dataset.copyReady = '1';
    });
  }

  document.addEventListener('click', async (event) => {
    const button = event.target.closest('[data-copy]');
    if (!button) return;
    const target = document.getElementById(button.dataset.copy);
    if (!target) return;
    try {
      await writeClipboard(target.textContent);
      feedback(button, 'copied');
    } catch {
      feedback(button, 'failed');
    }
  });

  enhance(document);
  window.workbenchEnhanceCopy = enhance;
})();
