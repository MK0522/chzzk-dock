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
  const cssByMode = (css && typeof css === 'object')
    ? { 0: String(css[0] || ''), 1: String(css[1] || ''), 2: String(css[2] || '') }
    : { 0: String(css || ''), 1: String(css || ''), 2: '' };

  const displayMode = 'html.chzzk-dock-overlay:is([data-overlay-mode="0"],[data-overlay-mode="1"])';
  const interactMode = 'html.chzzk-dock-overlay[data-overlay-mode="2"]';
  const notices = [
    '[class*="chatting_list_fixed"]', '[class*="chatting_fixed_"]',
    '[class^="_fixed_message_"], [class*=" _fixed_message_"]', '[class*="_pinned_message_"]', '[class*="_pinnedMessage_"]',
    '[class*="_filter_ik55w_"]', '[class*="_welcome_ik55w_"]',
    '[aria-label="고정 메시지"]', '[title="고정 메시지"]', '[data-chzzk-system-notice]',
  ].join(',');
  const chromeStyle = document.createElement('style');
  chromeStyle.textContent = [
    'html.chzzk-dock-overlay, html.chzzk-dock-overlay body { overflow: hidden !important; margin: 0; padding: 0; }',
    displayMode + ', ' + displayMode + ' * { scrollbar-width: none !important; }',
    displayMode + '::-webkit-scrollbar, ' + displayMode + ' *::-webkit-scrollbar { display: none !important; }',
    'html.chzzk-dock-overlay [data-chzzk-system-notice], html.chzzk-dock-overlay [class*="_item_"]:has(> [data-chzzk-system-notice]), html.chzzk-dock-overlay [class*="_filter_ik55w_"], html.chzzk-dock-overlay [class*="_welcome_ik55w_"] { visibility: hidden !important; display: none !important; }',
    displayMode + ' #chzzk-dock-bottom-tools { display: none !important; }',
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
      // Chat scale (100%~170%) controlled from source settings across all modes.
      'html.chzzk-dock-overlay [role="log"] [class*="_wrapper_1li40_"], html.chzzk-dock-overlay [role="log"] [class*="_floating_1li40_"] { --chat-scale: var(--dock-chat-scale, 1) !important; }',
      'html.chzzk-dock-overlay [role="log"] [class*="_chatting_message_"] { font-size: calc(14px * var(--dock-chat-scale, 1)) !important; line-height: calc(20px * var(--dock-chat-scale, 1)) !important; }',
      'html.chzzk-dock-overlay [role="log"] [class*="_chatting_message_"] [class*="_button_"] img { width: calc(24px * var(--dock-chat-scale, 1)) !important; height: calc(24px * var(--dock-chat-scale, 1)) !important; }',
      'html.chzzk-dock-overlay [role="log"] [class*="_nickname_"] img { width: calc(18px * var(--dock-chat-scale, 1)) !important; height: calc(18px * var(--dock-chat-scale, 1)) !important; }',
      // Interact mode: keep [role="log"] flex-basis 0% so scroll never squashes ranking strip.
      interactMode + ' [role="log"] { min-height: 0 !important; flex: 1 1 0% !important; }',
      interactMode + ' #aside-chatting > :not([role="log"]):not([class*="_area_nu1j9_"]), ' + interactMode + ' [class*="_container_wl8bq_"] { flex: 0 0 auto !important; flex-shrink: 0 !important; }',
      // Remove top "채팅" bar height while keeping the React menu wrapper mounted for dropdown portal alignment.
      interactMode + ' #aside-chatting > [class*="_container_17o7b_"] { height: 0 !important; min-height: 0 !important; border: 0 !important; margin: 0 !important; padding: 0 !important; overflow: visible !important; z-index: 2000 !important; }',
      interactMode + ' #aside-chatting > [class*="_container_17o7b_"] > h2, ' + interactMode + ' #aside-chatting > [class*="_container_17o7b_"] [class*="_scale_17o7b_"], ' + interactMode + ' #aside-chatting > [class*="_container_17o7b_"] [class*="_menu_17o7b_"] > button { display: none !important; }',
      interactMode + ' [class*="_menu_17o7b_"] > :not(button) { position: fixed !important; top: auto !important; bottom: var(--dock-menu-bottom, 52px) !important; right: 8px !important; max-width: calc(100vw - 16px) !important; max-height: calc(100vh - 64px) !important; overflow-y: auto !important; z-index: 2147483640 !important; }',
      // Bottom toolbar proxy controls placed immediately to the left of #send_chat_or_donate.
      interactMode + ' [class*="_tools_19u4u_"] { gap: 4px !important; flex-wrap: nowrap !important; }',
      interactMode + ' #send_chat_or_donate { margin-left: 2px !important; flex: none !important; }',
      '#chzzk-dock-bottom-tools { display: inline-flex; align-items: center; gap: 2px; margin-left: auto; flex: none; }',
      '.chzzk-dock-tool-btn { height: 24px; min-width: 24px; padding: 0 5px; border-radius: 6px; border: 0; background: var(--sem-color-surface-neutral-base, rgba(255,255,255,0.08)); color: var(--sem-color-content-neutral-cool-strong, #e2e8f0); font-size: 13px; font-weight: 700; line-height: 1; display: inline-flex; align-items: center; justify-content: center; cursor: pointer; }',
      '.chzzk-dock-tool-btn:hover:not(:disabled) { filter: brightness(1.2); }',
      '.chzzk-dock-tool-btn:disabled { opacity: 0.35; cursor: default; }',
      '.chzzk-dock-tool-scale { min-width: 34px; text-align: center; font-size: 11px; font-weight: 700; font-variant-numeric: tabular-nums; color: var(--sem-color-content-neutral-cool-strong, #cbd5e1); }',
      '.chzzk-dock-tool-more { background: transparent; padding: 0 4px; color: var(--color-content-04, #94a3b8); font-size: 15px; }',
      // 1-line compact marquee ranking when collapsed + hide when empty + floating zero-reflow drawer when expanded.
      'html.chzzk-dock-overlay [class*="_container_wl8bq_"][data-chzzk-ranking-empty="true"] { display: none !important; }',
      interactMode + ' [class*="_title_wl8bq_"][class*="_ranking_close_"] { display: none !important; }',
      interactMode + ' [class*="_container_wl8bq_"]:not([data-chzzk-expanded="true"]) [class*="_ranking_button_wl8bq_"] { width: 100% !important; height: 35px !important; margin: 0 !important; padding: 0 10px !important; align-items: center !important; }',
      interactMode + ' [class*="_container_wl8bq_"]:not([data-chzzk-expanded="true"]) [class*="_list_qxsox_"] { display: flex !important; flex-wrap: nowrap !important; width: max-content !important; height: 35px !important; padding: 0 !important; gap: 14px !important; align-items: center !important; }',
      interactMode + ' [class*="_container_wl8bq_"]:not([data-chzzk-expanded="true"]) [class*="_list_qxsox_"][data-chzzk-marquee="true"] { will-change: transform; animation: chzzk-ranking-marquee 12s ease-in-out infinite alternate; }',
      interactMode + ' [class*="_container_wl8bq_"]:not([data-chzzk-expanded="true"]):hover :is([class*="_list_qxsox_"], [class*="_area_19i63_"]) { animation-play-state: paused !important; }',
      '@keyframes chzzk-ranking-marquee { 0%, 10% { transform: translateX(0); } 90%, 100% { transform: translateX(calc(-1 * var(--marquee-shift, 0px))); } }',
      interactMode + ' #aside-chatting { box-sizing: border-box !important; }',
      interactMode + ' #aside-chatting[data-chzzk-ranking-expanded="true"] { padding-top: var(--dock-ranking-height, 36px) !important; }',
      interactMode + ' [class*="_container_wl8bq_"][data-chzzk-expanded="true"] { position: absolute !important; top: 0 !important; left: 0 !important; right: 0 !important; z-index: 1500 !important; max-height: calc(100vh - 92px) !important; overflow-y: auto !important; box-shadow: none !important; contain: layout style paint !important; background-color: var(--sem-color-background-neutral-base, #141517) !important; }',
      interactMode + ' [class*="_container_wl8bq_"][data-chzzk-expanded="true"] [class*="_contents_wl8bq_"] { max-height: calc(100vh - 170px) !important; overflow-y: auto !important; }',
      '@media (max-width: 360px) { ' + interactMode + ' [class*="_area_nu1j9_"] { padding: 8px 10px !important; } ' + interactMode + ' [class*="_donation_text_"] { font-size: 0 !important; letter-spacing: 0 !important; } }',
      '@media (max-height: 380px) { ' + interactMode + ' [class*="_area_nu1j9_"] { min-height: 0 !important; padding: 6px 12px !important; } ' + interactMode + ' [class*="_tools_19u4u_"] { margin-top: 5px !important; } ' + interactMode + ' textarea { min-height: 24px !important; } }',
    ].join('\n');
  }
  // Browser-source widgets can render the same system text with different
  // markup. Match the system sentences, keeping ordinary chat rows intact.
  const systemNotice = /^(?:쾌적한 시청 환경을 위해 일부 메시지는 필터링 됩니다\.?|클린 라이브 채팅 문화 만들기에 동참해 주세요\.?|채팅방에 오신 것을 환영합니다!?|드롭스 캠페인이 진행 중인 방송입니다!?)$/;
  function markSystemNotices(node) {
    if (!node) return;
    const walker = document.createTreeWalker(node, NodeFilter.SHOW_TEXT);
    let text = node.nodeType === Node.TEXT_NODE ? node : walker.nextNode();
    for (; text; text = walker.nextNode()) {
      const value = text.textContent.trim().replace(/\s+/g, ' ');
      const isWelcome = value.includes('채팅방에 오신 것을 환영합니다') && value.length < 60;
      const isFilter = value.startsWith('쾌적한 시청 환경을 위해 일부 메시지는 필터링 됩니다') && value.length < 120;
      const isDrops = value.includes('드롭스 캠페인이 진행 중인 방송입니다') && value.length < 100;
      if (!systemNotice.test(value) && !isWelcome && !isFilter && !isDrops) continue;
      const element = text.parentElement;
      if (!element || element.closest('script, style, textarea, button, [class*="_chatting_message_"], [class*="chatting_message_text"]')) continue;
      const card = element.closest('[class*="_container_ik55w_"], [class*="_filter_ik55w_"], [class*="_welcome_ik55w_"]');
      if (card && !card.querySelector('[class*="_chatting_message_"], [role="log"]')) {
        card.setAttribute('data-chzzk-system-notice', '');
        continue;
      }
      const parent = element.parentElement;
      const combined = parent?.textContent.trim().replace(/\s+/g, ' ');
      const filterCard = parent && combined?.startsWith('쾌적한 시청 환경을 위해 일부 메시지는 필터링 됩니다')
        && combined.includes('클린 라이브 채팅 문화 만들기에 동참해 주세요')
        && combined.length < 120 && parent.children.length <= 3 && !parent.querySelector('input, textarea, button, [role="log"], [class*="_chatting_message_"]');
      const target = filterCard ? parent : element;
      const targetText = target.textContent.trim().replace(/\s+/g, ' ');
      if (targetText === value || (filterCard && target === parent) || isWelcome || isDrops) target.setAttribute('data-chzzk-system-notice', '');
    }
  }
  const noticeObserver = new MutationObserver(records => {
    let uiDirty = false;
    for (const record of records) {
      if (record.target?.id === 'chzzk-dock-bottom-tools' || record.target?.closest?.('#chzzk-dock-bottom-tools')) continue;
      if (record.type === 'childList' && Array.from(record.addedNodes).every(n => n.id === 'chzzk-dock-bottom-tools')) continue;
      uiDirty = true;
      if (record.type === 'characterData') markSystemNotices(record.target);
      else for (const node of record.addedNodes) markSystemNotices(node);
    }
    if (chzzkChat && uiDirty) {
      syncInteractLayout();
    }
  });
  markSystemNotices(document.body);
  noticeObserver.observe(document.body, { childList: true, subtree: true, characterData: true, attributes: true, attributeFilter: ['aria-expanded', 'disabled'] });
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

  let proxyEls = null;
  function ensureBottomChatTools() {
    const sendBtn = document.getElementById('send_chat_or_donate');
    const toolsRow = sendBtn?.parentElement;
    const header = document.querySelector('#aside-chatting > [class*="_container_17o7b_"]');
    if (!toolsRow || !sendBtn || !header) return;
    const moreBtn = header.querySelector('[class*="_menu_17o7b_"] > button, button[aria-label*="더보기"]');
    if (!moreBtn) return;

    if (!proxyEls || !proxyEls.bar.isConnected) {
      const existing = document.getElementById('chzzk-dock-bottom-tools');
      if (existing) existing.remove();
      const bar = document.createElement('div');
      bar.id = 'chzzk-dock-bottom-tools';
      const more = document.createElement('button');
      more.type = 'button';
      more.className = 'chzzk-dock-tool-btn chzzk-dock-tool-more';
      more.textContent = '⋮';
      more.setAttribute('aria-label', '더보기 메뉴');
      more.title = '더보기 메뉴';
      bar.append(more);
      proxyEls = { bar, more };
    }
    if (proxyEls.bar.parentElement !== toolsRow || proxyEls.bar.nextElementSibling !== sendBtn) {
      toolsRow.insertBefore(proxyEls.bar, sendBtn);
    }
    let menuOpenBeforeDown = false;
    proxyEls.more.onpointerdown = () => { menuOpenBeforeDown = moreBtn?.getAttribute('aria-expanded') === 'true'; };
    proxyEls.more.onmousedown = e => e.stopPropagation();
    proxyEls.more.onclick = e => {
      e.stopPropagation();
      const openNow = moreBtn?.getAttribute('aria-expanded') === 'true';
      if (!menuOpenBeforeDown || openNow) moreBtn?.click();
      menuOpenBeforeDown = false;
    };
    proxyEls.more.setAttribute('aria-expanded', moreBtn.getAttribute('aria-expanded') || 'false');

    if (moreBtn.getAttribute('aria-expanded') === 'true') {
      const rect = sendBtn.getBoundingClientRect();
      if (rect.top > 0) root.style.setProperty('--dock-menu-bottom', Math.max(36, Math.round(window.innerHeight - rect.top + 8)) + 'px');
    }
  }

  function syncRankingState() {
    const aside = document.getElementById('aside-chatting');
    const ranking = aside?.querySelector('[class*="_container_wl8bq_"]');
    if (!aside || !ranking) return;
    const hasRankers = Boolean(ranking.querySelector('[class*="_item_qxsox_"], [class*="_box_19i63_"]'));
    if (!hasRankers) {
      ranking.setAttribute('data-chzzk-ranking-empty', 'true');
      ranking.removeAttribute('data-chzzk-expanded');
      aside.removeAttribute('data-chzzk-ranking-expanded');
      return;
    }
    ranking.removeAttribute('data-chzzk-ranking-empty');
    const expanded = Boolean(ranking.querySelector('[aria-expanded="true"]'));
    if (expanded) {
      ranking.setAttribute('data-chzzk-expanded', 'true');
      aside.setAttribute('data-chzzk-ranking-expanded', 'true');
    } else {
      ranking.removeAttribute('data-chzzk-expanded');
      aside.removeAttribute('data-chzzk-ranking-expanded');
      if (root.dataset.overlayMode === '2' && ranking.offsetHeight > 0) {
        aside.style.setProperty('--dock-ranking-height', ranking.offsetHeight + 'px');
      }
      const btn = ranking.querySelector('[class*="_ranking_button_wl8bq_"]');
      const list = btn?.querySelector('[class*="_list_qxsox_"]');
      if (btn && list && root.dataset.overlayMode === '2') {
        const shift = Math.ceil(list.scrollWidth - btn.clientWidth);
        if (shift > 4) {
          list.style.setProperty('--marquee-shift', shift + 'px');
          list.setAttribute('data-chzzk-marquee', 'true');
        } else {
          list.removeAttribute('data-chzzk-marquee');
        }
      }
    }
  }

  function syncInteractLayout() {
    if (!chzzkChat) return;
    if (root.dataset.overlayMode === '2') {
      ensureBottomChatTools();
    }
    syncRankingState();
  }
  if (chzzkChat) {
    document.addEventListener('pointerdown', event => {
      if (root.dataset.overlayMode !== '2') return;
      const aside = document.getElementById('aside-chatting');
      if (!aside || !aside.hasAttribute('data-chzzk-ranking-expanded')) return;
      if (event.target?.closest?.('[class*="_container_wl8bq_"]')) return;
      aside.querySelector('[class*="_header_button_wl8bq_"], [class*="_swipe_button_wl8bq_"]')?.click();
    }, true);
    if (typeof ResizeObserver === 'function') {
      new ResizeObserver(syncInteractLayout).observe(root);
    }
  }

  document.body.append(drag);
  window.chzzkDockOverlay = {
    update(mode, backgroundOpacity, crop = [0, 0, 0, 0], backgroundColor = "#141517", donationOpacity = 100, chatScale = 100) {
      root.style.setProperty('--overlay-background-color', backgroundColor);
      root.style.setProperty('--donation-alpha', String(donationOpacity / 100));
      const clampedScale = Math.min(170, Math.max(100, Number(chatScale) || 100));
      root.style.setProperty('--dock-chat-scale', String(clampedScale / 100));
      const background = mode !== 2 && backgroundOpacity > 0;
      const modeCss = (cssByMode[mode] || '').trim();
      if (mode === 2 && !modeCss) {
        sourceStyle.disabled = true;
        sourceStyle.textContent = '';
      } else {
        sourceStyle.disabled = false;
        sourceStyle.textContent = modeCss || defaultCSS;
      }
      root.style.setProperty('--overlay-background-alpha', String(backgroundOpacity / 100));
      root.dataset.overlayMode = String(mode);
      root.dataset.overlayBackground = String(background);
      const scale = window.devicePixelRatio || 1;
      ['left', 'top', 'right', 'bottom'].forEach((side, i) => {
        root.style.setProperty('--crop-' + side, crop[i] / scale + 'px');
        drag.toggleAttribute('data-' + side, crop[i] > 0);
      });
      drag.hidden = mode !== 1;
      syncInteractLayout();
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
