(chzzkChat, css, initialState) => {
  if (window.chzzkDockOverlay) {
    window.chzzkDockOverlay.update(...initialState);
    return;
  }

  const root = document.documentElement;
  root.classList.add('chzzk-dock-overlay');
  const sourceStyle = document.createElement('style');
  const defaultCSS = chzzkChat ? [
      'html.chzzk-dock-overlay, html.chzzk-dock-overlay body,',
      '.chzzk-dock-overlay #root, .chzzk-dock-overlay #root :is(div, section):has(aside),',
      '.chzzk-dock-overlay #root aside { background: transparent !important; }',
      '.chzzk-dock-overlay #root aside { border-color: transparent !important; outline: 0 !important; box-shadow: none !important; }',
      '.chzzk-dock-overlay [class*="_chatting_message_"] { text-shadow: 0 1px 3px #000, 0 0 4px #000; }',
    ].join('\n') : ':where(html.chzzk-dock-overlay, html.chzzk-dock-overlay body) { background: transparent; }';
  sourceStyle.textContent = css.trim() ? css : defaultCSS;

  const displayMode = 'html.chzzk-dock-overlay:is([data-overlay-mode="0"],[data-overlay-mode="1"])';
  const notices = [
    '[class*="chatting_list_fixed"]', '[class*="chatting_fixed_"]',
    '[class^="_fixed_message_"], [class*=" _fixed_message_"]', '[class*="_pinned_message_"]', '[class*="_pinnedMessage_"]',
    '[class*="_filter_ik55w_"]', '[class*="_welcome_ik55w_"]',
    '[aria-label="고정 메시지"]', '[title="고정 메시지"]', '[data-chzzk-system-notice]',
  ].join(',');
  const chromeStyle = document.createElement('style');
  chromeStyle.textContent = [
    displayMode + ', ' + displayMode + ' body { overflow: hidden !important; }',
    ':where(' + displayMode + ', ' + displayMode + ' body) { margin: 0; padding: 0; }',
    displayMode + ', ' + displayMode + ' * { scrollbar-width: none !important; }',
    displayMode + '::-webkit-scrollbar, ' + displayMode + ' *::-webkit-scrollbar { display: none !important; }',
    displayMode + ' [data-chzzk-system-notice] { visibility: hidden !important; }',
    displayMode + ' [data-chzzk-system-notice] { display: none !important; }',
    'html.chzzk-dock-overlay[data-overlay-background="true"] body { background-color: color-mix(in srgb, var(--overlay-background-color, #141517) calc(var(--overlay-background-alpha, 0) * 100%), transparent) !important; }',
    '#chzzk-overlay-drag { position: fixed; inset: var(--crop-top, 0px) var(--crop-right, 0px) var(--crop-bottom, 0px) var(--crop-left, 0px); z-index: 2147483646; border: 2px solid #e95252; box-sizing: border-box; background: transparent; cursor: move; touch-action: none; }',
    '#chzzk-overlay-drag[data-left] { border-left-color: #55d480; }',
    '#chzzk-overlay-drag[data-top] { border-top-color: #55d480; }',
    '#chzzk-overlay-drag[data-right] { border-right-color: #55d480; }',
    '#chzzk-overlay-drag[data-bottom] { border-bottom-color: #55d480; }',
    '#chzzk-overlay-drag > span { position: absolute; left: var(--x); top: var(--y); width: 7px; height: 7px; transform: translate(-50%,-50%); pointer-events: none; background: #e95252; }',
    '#chzzk-overlay-drag[data-left] > [data-edge~="left"], #chzzk-overlay-drag[data-top] > [data-edge~="top"], #chzzk-overlay-drag[data-right] > [data-edge~="right"], #chzzk-overlay-drag[data-bottom] > [data-edge~="bottom"] { background: #55d480; }',
    '#chzzk-overlay-drag[hidden] { display: none !important; }',
  ].join('\n');
  if (chzzkChat) {
    const log = '[role="log"]';
    const outside = ':not([role="log"], :has([role="log"]), #chzzk-overlay-drag, script, style, link)';
    chromeStyle.textContent += '\n' + [
      displayMode + ' { background: transparent !important; }',
      // Preserve the mounted, scrollable chat list even before the first message arrives.
      displayMode + ' :is(body, body *):has(' + log + ') > ' + outside + ' { display: none !important; }',
      displayMode + ' [role="log"] > [class*="_fixed_"], ' + displayMode + ' :is(' + notices + ') { display: none !important; }',
      displayMode + ' [role="log"] { --fixed-components-height: 0px !important; }',
      ...['neutral','dark-violet','teal','green','camel','brick'].map((color, i) => displayMode + ' [class*="_container_120bf_"]' + (i ? '[class*="_level' + (i-1) + '_120bf_"]' : '') + ' { background-color: color-mix(in srgb, var(--sem-color-surface-function-donation-' + color + ') calc(var(--donation-alpha, 1) * 100%), transparent) !important; }'),
      displayMode + ' [class*="_chatting_message_"] { text-shadow: 0 1px 3px #000; }',
    ].join('\n');
  }
  // Browser-source widgets can render the same system text with different
  // markup. Match the system sentences, keeping ordinary chat rows intact.
  const systemNotice = /^(?:쾌적한 시청 환경을 위해 일부 메시지는 필터링 됩니다\.?|클린 라이브 채팅 문화 만들기에 동참해 주세요\.?|채팅방에 오신 것을 환영합니다!?)$/;
  function markSystemNotices(node) {
    const walker = document.createTreeWalker(node, NodeFilter.SHOW_TEXT);
    let text = node.nodeType === Node.TEXT_NODE ? node : walker.nextNode();
    for (; text; text = walker.nextNode()) {
      const value = text.textContent.trim().replace(/\s+/g, ' ');
      if (!systemNotice.test(value) && !(value.startsWith('쾌적한 시청 환경을 위해 일부 메시지는 필터링 됩니다') && value.includes('클린 라이브 채팅 문화 만들기에 동참해 주세요') && value.length < 110)) continue;
      const element = text.parentElement;
      if (!element || element.closest('script, style, textarea, button, [class*="_chatting_message_"], [class*="chatting_message_text"]')) continue;
      const row = element.closest('[class*="_filter_ik55w_"], [class*="_welcome_ik55w_"]');
      const parent = element.parentElement;
      const combined = parent?.textContent.trim().replace(/\s+/g, ' ');
      const filterCard = parent && combined?.startsWith('쾌적한 시청 환경을 위해 일부 메시지는 필터링 됩니다')
        && combined.includes('클린 라이브 채팅 문화 만들기에 동참해 주세요')
        && combined.length < 110 && parent.children.length <= 3 && !parent.querySelector('input, textarea, button, [role="log"], [class*="_chatting_message_"]');
      const target = row || (filterCard ? parent : element);
      const targetText = target.textContent.trim().replace(/\s+/g, ' ');
      if (targetText === value || filterCard && target === parent) target.setAttribute('data-chzzk-system-notice', '');
    }
  }
  const noticeObserver = new MutationObserver(records => {
    for (const record of records) {
      if (record.type === 'characterData') markSystemNotices(record.target);
      else for (const node of record.addedNodes) markSystemNotices(node);
    }
  });
  markSystemNotices(document.body);
  noticeObserver.observe(document.body, { childList: true, subtree: true, characterData: true });
  document.head.append(sourceStyle, chromeStyle);

  const send = action => window.chrome?.webview?.postMessage('chat-overlay:' + action);
  const drag = document.createElement('div');
  drag.id = 'chzzk-overlay-drag';
  drag.title = '몸통 드래그: 이동 · 가장자리 드래그: 크기 · Alt+드래그: 자르기';
  for (const [y, vertical] of [['1px', 'top'], ['50%', ''], ['calc(100% - 1px)', 'bottom']]) {
    for (const [x, horizontal] of [['1px', 'left'], ['50%', ''], ['calc(100% - 1px)', 'right']]) {
      if (!vertical && !horizontal) continue;
      const handle = document.createElement('span');
      handle.style.setProperty('--x', x);
      handle.style.setProperty('--y', y);
      handle.dataset.edge = [vertical, horizontal].filter(Boolean).join(' ');
      drag.append(handle);
    }
  }
  drag.addEventListener('pointerdown', event => {
    if (event.button !== 0) return;
    event.preventDefault();
    send(event.altKey ? 'drag-crop' : 'drag');
  });
  drag.addEventListener('pointermove', event => {
    const rect = drag.getBoundingClientRect();
    const edge = 9 / (window.devicePixelRatio || 1);
    const x = event.clientX < rect.left + edge ? -1 : event.clientX >= rect.right - edge ? 1 : 0;
    const y = event.clientY < rect.top + edge ? -1 : event.clientY >= rect.bottom - edge ? 1 : 0;
    drag.style.cursor = x && y ? (x === y ? 'nwse-resize' : 'nesw-resize')
      : x ? 'ew-resize' : y ? 'ns-resize' : 'move';
  });

  document.body.append(drag);
  window.chzzkDockOverlay = {
    update(mode, backgroundOpacity, crop = [0, 0, 0, 0], backgroundColor = "#141517", donationOpacity = 100) {
      root.style.setProperty('--overlay-background-color', backgroundColor);
      root.style.setProperty('--donation-alpha', String(donationOpacity / 100));
      const background = mode !== 2 && backgroundOpacity > 0;
      sourceStyle.disabled = mode === 2;
      root.style.setProperty('--overlay-background-alpha', String(backgroundOpacity / 100));
      root.dataset.overlayMode = String(mode);
      root.dataset.overlayBackground = String(background);
      const scale = window.devicePixelRatio || 1;
      ['left', 'top', 'right', 'bottom'].forEach((side, i) => {
        root.style.setProperty('--crop-' + side, crop[i] / scale + 'px');
        drag.toggleAttribute('data-' + side, crop[i] > 0);
      });
      drag.hidden = mode !== 1;

    },
  };

  window.chzzkDockOverlay.update(...initialState);
  let notified = false;
  const surfaceReady = () => {
    if (notified) return;
    notified = true;
    send('surface-ready');
  };
  requestAnimationFrame(() => requestAnimationFrame(surfaceReady));
  setTimeout(surfaceReady, 150);
}
