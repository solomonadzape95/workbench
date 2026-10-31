(() => {
  const dialog = document.getElementById('tui');
  const body = document.getElementById('tui-body');
  const title = document.getElementById('tui-title');
  if (!dialog || !body || !title) return;

  const titles = {
    install: 'Install',
    docs: 'Docs',
    releases: 'Releases',
    agents: 'Agents',
    help: 'Help',
  };

  function openPanel(name) {
    const template = document.getElementById(`panel-${name}`);
    if (!template) return;
    body.replaceChildren(template.content.cloneNode(true));
    title.textContent = titles[name] || name;
    dialog.showModal();
    body.scrollTop = 0;
    document.body.classList.add('tui-open');
  }

  function closePanel() {
    if (dialog.open) dialog.close();
    document.body.classList.remove('tui-open');
  }

  document.querySelectorAll('[data-panel]').forEach((element) => {
    element.addEventListener('click', (event) => {
      event.preventDefault();
      openPanel(element.dataset.panel);
    });
  });

  dialog.querySelectorAll('[data-close]').forEach((button) => {
    button.addEventListener('click', closePanel);
  });
  dialog.addEventListener('click', (event) => {
    if (event.target === dialog) closePanel();
  });
  dialog.addEventListener('close', () => {
    document.body.classList.remove('tui-open');
  });

  window.addEventListener('open-workbench-panel', (event) => openPanel(event.detail));
})();
