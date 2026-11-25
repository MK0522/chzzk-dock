(() => {
  const manager = document.querySelector('.overlay-manager');
  if (!manager) return;
  const urlsPanel = manager.closest('#urls-panel');
  const dialog = manager.querySelector('#overlay-editor');
  const list = manager.querySelector('#layers');
  const editor = document.querySelector('#editor');
  const status = document.querySelector('#status');
  const editorStatus = document.querySelector('#editor-status');
  const editorTitle = document.querySelector('#editor-title');
  let editorTrigger = null;
  const modeTabs = Array.from(document.querySelectorAll('.mode-tabs [data-mode]'));
  let layers = [];
  let running = {};
  let editing = null;
  let selected = null;
  let busy = false;
  let previewQueue = Promise.resolve();
  let previewOriginal = null;
  let draggedId = null;

  function announce(message, error = false) {
    status.textContent = message;
    status.dataset.error = String(error);
  }

  async function request(path = '', method = 'GET', data) {
    const response = await apiFetch('/overlay-layers' + path, {
      method, body: data === undefined ? undefined : JSON.stringify(data),
    });
    const result = await response.json();
    if (!response.ok) throw new Error(result.message || result.error || '요청이 실패했습니다.');
    return result;
  }

  function button(label, action) {
    const element = document.createElement('button');
    element.type = 'button';
    element.textContent = label;
    element.addEventListener('click', action);
    return element;
  }

  function render() {
    if (!layers.some(layer => layer.id === selected)) selected = layers[0]?.id ?? null;
    list.replaceChildren(...layers.map(layer => {
      const row = document.createElement('div');
      row.className = 'source-row';
      row.dataset.selected = String(layer.id === selected);
      row.dataset.visible = String(Boolean(running[layer.id]));
      row.draggable = true;
      row.addEventListener('dragstart', event => {
        if (busy || event.target.closest('.visibility')) { event.preventDefault(); return; }
        draggedId = layer.id;
        event.dataTransfer.effectAllowed = 'move';
        event.dataTransfer.setData('text/plain', layer.id);
        row.classList.add('dragging');
      });
      row.addEventListener('dragover', event => {
        if (!draggedId || draggedId === layer.id) return;
        event.preventDefault();
        event.dataTransfer.dropEffect = 'move';
        list.querySelectorAll('.drop-before, .drop-after').forEach(item => item.classList.remove('drop-before', 'drop-after'));
        row.classList.add(event.clientY < row.getBoundingClientRect().top + row.offsetHeight / 2 ? 'drop-before' : 'drop-after');
      });
      row.addEventListener('drop', event => {
        if (!draggedId || draggedId === layer.id) return;
        event.preventDefault();
        const source = draggedId;
        const after = row.classList.contains('drop-after');
        clearDrag();
        perform(async () => {
          const next = [...layers];
          const index = next.findIndex(item => item.id === source);
          const target = next.findIndex(item => item.id === layer.id);
          if (index < 0 || target < 0) return;
          const [moved] = next.splice(index, 1);
          const destination = next.findIndex(item => item.id === layer.id) + (after ? 1 : 0);
          next.splice(destination, 0, moved);
          selected = source;
          await save(next);
        });
      });
      row.addEventListener('dragend', clearDrag);
      const name = button(layer.name, () => select(layer.id));
      name.className = 'source-name';
      name.setAttribute('aria-pressed', String(layer.id === selected));
      name.addEventListener('dblclick', () => openEditor(document.querySelector('#properties'), layer));
      name.title = layer.id === 'chzzk' ? '기본 치지직 채팅' : '브라우저 소스';
      const toggle = button('', () => perform(async () => {
        const visible = Boolean(running[layer.id]);
        await request('/action', 'POST', { id: layer.id, action: visible ? 'hide' : 'show' });
        await refreshUntilChanged(layer.id, !visible);
      }));
      toggle.className = 'visibility';
      toggle.setAttribute('aria-pressed', String(Boolean(running[layer.id])));
      toggle.setAttribute('aria-label', layer.name + (running[layer.id] ? ' 숨기기' : ' 표시'));
      toggle.title = running[layer.id] ? '숨기기' : '표시';
      const icon = document.createElement('span');
      icon.className = 'eye';
      icon.setAttribute('aria-hidden', 'true');
      toggle.append(icon);
      row.append(toggle, name);
      return row;
    }));
    syncSelection();
  }

  function clearDrag() {
    draggedId = null;
    list.querySelectorAll('.dragging, .drop-before, .drop-after').forEach(row => row.classList.remove('dragging', 'drop-before', 'drop-after'));
  }
  list.addEventListener('dragleave', event => {
    if (!list.contains(event.relatedTarget)) list.querySelectorAll('.drop-before, .drop-after').forEach(row => row.classList.remove('drop-before', 'drop-after'));
  });

  function select(id) {
    selected = id;
    for (const [index, row] of Array.from(list.children).entries()) {
      const active = layers[index].id === id;
      row.dataset.selected = String(active);
      row.querySelector('.source-name').setAttribute('aria-pressed', String(active));
    }
    syncSelection();
  }

  const controls = {
    donationBackgroundOpacity: document.querySelector("#donation-opacity"),
    windowOpacity: document.querySelector('#window-opacity'),
    backgroundOpacity: document.querySelector('#background-opacity'),
    chatScale: document.querySelector('#chat-scale'),
  };

  function syncSelection() {
    const index = layers.findIndex(layer => layer.id === selected);
    const layer = layers[index];
    document.querySelector('#remove').disabled = !layer;
    document.querySelector('#properties').disabled = !layer;
    document.querySelector('#up').disabled = index < 1;
    document.querySelector('#down').disabled = !layer || index >= layers.length - 1;

  }

  for (const input of [...Object.values(controls), document.querySelector("#background-color")]) {
    input.addEventListener('input', () => {
      if (input.type === 'range') document.querySelector('#' + input.id + '-value').textContent = input.value + '%';
      if (editing && running[editing]) {
        queuePreview({
          donationBackgroundOpacity: Number(controls.donationBackgroundOpacity.value),
          backgroundColor: editor.elements.backgroundColor.value,
          id: editing,
          windowOpacity: Number(controls.windowOpacity.value),
          backgroundOpacity: Number(controls.backgroundOpacity.value),
          chatScale: Number(controls.chatScale.value),
        });
      }
    });
  }
  function queuePreview(values) {
    previewQueue = previewQueue.catch(() => {}).then(() => request('/action', 'POST', { action: 'preview', ...values }));
    previewQueue.catch(error => { editorStatus.textContent = error.message; editorStatus.hidden = false; });
    return previewQueue;
  }
  document.querySelector('#properties').addEventListener('click', event => {
    const layer = layers.find(layer => layer.id === selected);
    if (layer) openEditor(event.currentTarget, layer);
  });
  document.querySelector('#remove').addEventListener('click', async () => {
    const id = selected;
    if (!id || busy) return;
    const layer = layers.find(item => item.id === id);
    const label = layer?.name || '선택한 소스';
    const confirmed = (typeof window.showConfirmModal === 'function')
      ? await window.showConfirmModal({
          title: '소스 삭제',
          message: `'${label}' 소스를 삭제하시겠습니까?`,
          confirmText: '삭제',
          cancelText: '취소',
          danger: true,
        })
      : window.confirm(`'${label}' 소스를 삭제하시겠습니까?`);
    if (!confirmed) return;
    perform(async () => {
      await save(layers.filter(layer => layer.id !== id));
      if (editing === id) closeEditor();
    });
  });
  for (const [selector, offset] of [['#up', -1], ['#down', 1]]) {
    document.querySelector(selector).addEventListener('click', () => move(layers.findIndex(layer => layer.id === selected), offset));
  }

  const MODE_DESCRIPTIONS = {
    1: '위치·크기 조절 (투명)',
    2: '채팅 입력·클릭 (불투명)',
    0: '클릭 통과·송출 (투명)',
  };
  const modeDesc = document.querySelector('#overlay-mode-desc');
  const layoutButtons = Array.from(document.querySelectorAll('.layout-toggle [data-layout]'));

  function applySourceColumns(cols, persist = false) {
    const normalized = Number(cols) === 2 ? 2 : 1;
    list.dataset.columns = String(normalized);
    for (const btn of layoutButtons) {
      btn.setAttribute('aria-pressed', String(Number(btn.dataset.layout) === normalized));
    }
    if (persist) {
      apiFetch('/app-settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ overlay_source_columns: normalized }),
      }).catch(() => {});
    }
  }
  for (const btn of layoutButtons) {
    btn.addEventListener('click', () => applySourceColumns(btn.dataset.layout, true));
  }
  apiFetch('/app-settings').then(r => r.ok ? r.json() : null).then(data => {
    if (data && (data.overlay_source_columns === 1 || data.overlay_source_columns === 2)) {
      applySourceColumns(data.overlay_source_columns, false);
    }
  }).catch(() => {});

  function highlightMode(mode) {
    for (const tab of modeTabs) tab.setAttribute('aria-pressed', String(Number(tab.dataset.mode) === mode));
    if (modeDesc) {
      modeDesc.textContent = MODE_DESCRIPTIONS[mode] || MODE_DESCRIPTIONS[1];
      modeDesc.dataset.activeMode = String(mode);
    }
  }

  function update(data) {
    const changed = JSON.stringify([layers, running]) !== JSON.stringify([data.layers, data.running]);
    layers = data.layers;
    running = data.running;
    highlightMode(data.mode);
    if (changed) render();
  }

  async function refresh() { update(await request()); }
  async function refreshUntilChanged(id, expected) {
    const previous = running[id];
    for (let attempt = 0; attempt < 12; attempt++) {
      await refresh();
      if (Boolean(running[id]) === expected) return;
      await new Promise(resolve => setTimeout(resolve, 250));
    }
    if (Boolean(previous) !== expected) announce('창의 상태가 아직 바뀌지 않았습니다. 잠시 뒤 다시 확인해 주세요.');
  }

  async function save(next) { update(await request('', 'PUT', next)); }

  async function perform(action, reportError = message => announce(message, true)) {
    if (busy) return;
    busy = true;
    announce('');
    try { await action(); }
    catch (error) { reportError(error.message); }
    finally { busy = false; }
  }

  function move(index, direction) {
    return perform(async () => {
      const next = [...layers];
      const other = index + direction;
      [next[index], next[other]] = [next[other], next[index]];
      await save(next);
    });
  }

  const cssTabs = Array.from(document.querySelectorAll('.css-mode-tabs [data-css-tab]'));
  const cssHelps = Array.from(document.querySelectorAll('#custom-css [data-css-help]'));
  const cssPanes = Array.from(document.querySelectorAll('#custom-css [data-css-pane]'));
  function selectCssTab(tabName) {
    for (const tab of cssTabs) tab.setAttribute('aria-selected', String(tab.dataset.cssTab === tabName));
    for (const help of cssHelps) help.hidden = help.dataset.cssHelp !== tabName;
    for (const pane of cssPanes) pane.hidden = pane.dataset.cssPane !== tabName;
  }
  for (const tab of cssTabs) {
    tab.addEventListener('click', () => selectCssTab(tab.dataset.cssTab));
  }

  function openEditor(trigger, layer) {
    if (busy) return;
    closeEditor();
    editing = layer?.id ?? null;
    editor.elements.backgroundColor.value = layer?.backgroundColor ?? "#141517";
    previewOriginal = layer ? {
      donationBackgroundOpacity: layer.donationBackgroundOpacity ?? 100,
      backgroundColor: layer.backgroundColor ?? "#141517",
      id: layer.id,
      windowOpacity: layer.windowOpacity ?? 100,
      backgroundOpacity: layer.backgroundOpacity ?? 0,
      chatScale: layer.chatScale ?? 100,
    } : null;
    editorTitle.textContent = layer ? '소스 수정' : '소스 추가';
    editorStatus.hidden = true;
    editor.elements.name.value = layer?.name ?? '';
    editor.elements.url.required = layer?.id !== 'chzzk';
    editor.elements.url.value = layer?.url ?? '';
    const hasPerModeCss = Boolean(layer?.cssArrange || layer?.cssInteract || layer?.cssPass);
    const legacyCss = hasPerModeCss ? '' : (layer?.css ?? '');
    editor.elements.cssArrange.value = layer?.cssArrange ?? legacyCss;
    editor.elements.cssInteract.value = layer?.cssInteract ?? '';
    editor.elements.cssPass.value = layer?.cssPass ?? legacyCss;
    selectCssTab('arrange');
    document.querySelector('#custom-css').open = false;
    const appearanceDetails = document.querySelector('#appearance-settings');
    if (appearanceDetails) appearanceDetails.open = false;
    editor.elements.linkLogin.checked = layer?.linkLogin ?? true;
    for (const [key, input] of Object.entries(controls)) {
      input.value = layer?.[key] ?? (key === 'backgroundOpacity' ? 0 : 100);
      document.querySelector('#' + input.id + '-value').textContent = input.value + '%';
    }
    editorTrigger?.setAttribute('aria-expanded', 'false');
    editorTrigger = trigger;
    trigger.setAttribute('aria-expanded', 'true');
    dialog.removeAttribute('style');
    dialog.removeAttribute('popover');
    dialog.showModal();
    editor.elements.name.focus({ preventScroll: true });
  }

  function closeEditor() {
    if (busy) return;
    if (dialog.open) dialog.close();
  }

  function resetEditor() {
    if (dialog.open) return;
    if (previewOriginal) queuePreview(previewOriginal);
    previewOriginal = null;
    editorTrigger?.setAttribute('aria-expanded', 'false');
    editorTrigger = null;
    editing = null;
  }

  let backdropDownOutside = false;
  dialog.addEventListener('pointerdown', event => {
    if (!dialog.open || busy) {
      backdropDownOutside = false;
      return;
    }
    const rect = dialog.getBoundingClientRect();
    backdropDownOutside = event.clientX < rect.left || event.clientX > rect.right
      || event.clientY < rect.top || event.clientY > rect.bottom;
  });
  dialog.addEventListener('click', event => {
    if (!dialog.open || busy || !backdropDownOutside) return;
    backdropDownOutside = false;
    const rect = dialog.getBoundingClientRect();
    if (event.clientX < rect.left || event.clientX > rect.right
      || event.clientY < rect.top || event.clientY > rect.bottom) {
      closeEditor();
    }
  });
  dialog.addEventListener('close', resetEditor);
  dialog.addEventListener('cancel', event => { if (busy) event.preventDefault(); });
  document.querySelector('#add').addEventListener('click', event => openEditor(event.currentTarget));
  document.querySelector('#cancel').addEventListener('click', closeEditor);

  for (const tab of modeTabs) {
    tab.addEventListener('click', () => perform(async () => {
      const mode = Number(tab.dataset.mode);
      await request('/action', 'POST', { action: 'mode', mode });
      highlightMode(mode);
    }));
  }
  editor.addEventListener('submit', event => {
    event.preventDefault();
    editorStatus.hidden = true;
    perform(async () => {
      const values = Object.fromEntries(new FormData(editor));
      values.linkLogin = editor.elements.linkLogin.checked;
      values.donationBackgroundOpacity = Number(values.donationBackgroundOpacity);
      values.windowOpacity = Number(values.windowOpacity);
      values.backgroundOpacity = Number(values.backgroundOpacity);
      values.chatScale = Number(values.chatScale);
      const before = layers.find(layer => layer.id === editing);
      const layer = { ...before, ...values, css: '', url: values.url ?? '', id: editing ?? crypto.randomUUID() };
      const next = editing ? layers.map(item => item.id === editing ? layer : item) : [...layers, layer];
      await previewQueue;
      await save(next);
      previewOriginal = null;
      select(layer.id);
      if (dialog.open) dialog.close();
      announce('레이어를 저장했습니다.');
    }, message => {
      if (!dialog.open) return announce(message, true);
      editorStatus.textContent = message;
      editorStatus.hidden = false;
    });
  });
  if (urlsPanel) manager.addEventListener('overlay-activated', () => perform(refresh));
  else perform(refresh);

  setInterval(() => {
    if (!busy && !draggedId && manager.getClientRects().length && (!urlsPanel || urlsPanel.classList.contains('open'))) {
      refresh().catch(() => {});
    }
  }, 1200);
})();

