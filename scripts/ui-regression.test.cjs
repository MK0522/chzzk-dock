const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const assert = require('node:assert/strict');

const root = process.argv[2] || '.';
const read = (rel) => fs.readFileSync(path.join(root, rel), 'utf8');
const flush = () => new Promise((resolve) => setImmediate(resolve));

function createClassList(initial = []) {
  const set = new Set(initial);
  return {
    add: (...cls) => cls.forEach((c) => set.add(c)),
    remove: (...cls) => cls.forEach((c) => set.delete(c)),
    contains: (c) => set.has(c),
    toggle: (c, force) => {
      if (force === undefined) {
        if (set.has(c)) { set.delete(c); return false; }
        set.add(c); return true;
      }
      if (force) { set.add(c); return true; }
      set.delete(c); return false;
    },
    _set: set,
  };
}

function createEl(id, initialClasses = []) {
  const props = {};
  const classList = createClassList(initialClasses);
  return {
    id,
    textContent: '',
    innerHTML: '',
    value: '',
    checked: false,
    className: initialClasses.join(' '),
    classList,
    dataset: {},
    attributes: {},
    style: {
      setProperty(k, v) { props[k] = v; },
      removeProperty(k) { delete props[k]; },
      getPropertyValue(k) { return props[k] || ''; },
    },
    setAttribute(k, v) { this.attributes[k] = String(v); },
    getAttribute(k) { return this.attributes[k] ?? null; },
    removeAttribute(k) { delete this.attributes[k]; },
    _props: props,
  };
}

function createDockTestEnv(opts = {}) {
  const nodes = opts.nodes || new Map();
  const docListeners = {};
  const winListeners = {};
  const noop = () => {};
  const getTheme = opts.getTheme || (() => 'dark');
  const getHasFocus = opts.hasFocus || (() => true);
  const getActiveElement = opts.getActiveElement || (() => null);

  const document = {
    documentElement: { getAttribute: (k) => (k === 'data-theme' ? getTheme() : null), setAttribute: noop },
    body: createEl('body', []),
    hasFocus: getHasFocus,
    get activeElement() { return getActiveElement(); },
    createElement: () => {
      const node = createEl('tmp', []);
      Object.defineProperty(node, 'textContent', {
        get() { return this._text || ''; },
        set(v) {
          this._text = String(v);
          this.innerHTML = this._text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
        },
      });
      return node;
    },
    addEventListener(ev, fn) {
      docListeners[ev] = docListeners[ev] || [];
      docListeners[ev].push(fn);
    },
    getElementById(id) {
      if (!nodes.has(id) && opts.autoCreate !== false) nodes.set(id, createEl(id, []));
      return nodes.get(id) || null;
    },
    querySelectorAll: opts.querySelectorAll || (() => []),
    querySelector: () => null,
  };

  const storage = new Map();
  const localStorage = {
    getItem: (k) => (storage.has(k) ? storage.get(k) : null),
    setItem: (k, v) => storage.set(k, String(v)),
    removeItem: (k) => storage.delete(k),
    clear: () => storage.clear(),
  };

  const ctx = {
    document,
    window: {
      addEventListener(ev, fn) {
        winListeners[ev] = winListeners[ev] || [];
        winListeners[ev].push(fn);
      },
      location: { origin: 'http://localhost:8081' },
    },
    navigator: {},
    console,
    setTimeout: opts.setTimeout || setTimeout,
    clearTimeout,
    localStorage,
    sessionStorage: { getItem: () => null, removeItem() {} },
    chzzkApi: opts.chzzkApi || {},
    apiFetch: opts.apiFetch || (async () => ({ ok: false, status: 500, json: async () => ({}) })),
    showToast: noop,
    ...(opts.globals || {}),
  };
  vm.createContext(ctx);
  return { ctx, nodes, document, docListeners, winListeners };
}

// 1. Static Tailwind CSS compilation & CDN removal checks
function testStaticTailwindParity() {
  const docks = [
    'ui/main-dock/index.html',
    'ui/mini-dock/index.html',
    'ui/stats-dock/index.html',
  ];
  for (const dock of docks) {
    const html = read(dock);
    assert(!html.includes('cdn.tailwindcss.com'), `${dock} must not reference cdn.tailwindcss.com`);
    const twIdx = html.indexOf('/ui/shared/tailwind.css');
    const themeIdx = html.indexOf('/ui/shared/theme.css');
    assert(twIdx !== -1, `${dock} must load /ui/shared/tailwind.css`);
    assert(themeIdx !== -1, `${dock} must load /ui/shared/theme.css`);
    assert(twIdx < themeIdx, `${dock} must load tailwind.css before theme.css`);
  }

  const twCss = read('ui/shared/tailwind.css');
  const mustHaveSelectors = [
    '.-translate-y-1\\/2',
    '.top-1\\/2',
    '.border-r',
    '.\\!flex',
    '.hidden',
    '.backdrop-blur-sm',
    '.text-obsAccent',
  ];
  for (const sel of mustHaveSelectors) {
    assert(twCss.includes(sel), `compiled tailwind.css missing critical selector: ${sel}`);
  }
  console.log('PASS: static Tailwind CSS parity & CDN removal across all 3 docks');
}

// 2. Remote filter color WCAG AA contrast (Option A) across dark & light modes
function testRemoteFilterColorContrast() {
  let theme = 'dark';
  const nodes = new Map([['filter-color-popover', createEl('filter-color-popover', ['hidden'])]]);
  const cards = [createEl('card-1', ['remote-feed-card'])];
  cards[0].dataset.feedCat = 'FOLLOW';

  const { ctx, document } = createDockTestEnv({
    nodes,
    getTheme: () => theme,
    querySelectorAll: (sel) => (sel === '#remote-feed-list .remote-feed-card' ? cards : []),
    globals: { saveAppSettingField() {} },
  });
  vm.runInContext(read('ui/main-dock/js/moderation.js'), ctx);

  const presets = ['#00FFA3', '#38BDF8', '#A78BFA', '#F472B6', '#FBBF24', '#FB7185', '#0000FF', '#FFFF00', '#000000', '#FFFFFF'];
  for (const mode of ['dark', 'light']) {
    theme = mode;
    for (const hex of presets) {
      ctx.setFilterChipColor('FOLLOW', hex);
      const chipBtn = document.getElementById('feed-filter-follow');
      const fgChip = chipBtn.style.getPropertyValue('--chip-fg');
      const chipBg = ctx.blendAlphaOver(ctx.parseHexRgb(hex), 0.13, mode === 'light' ? [248, 250, 252] : [13, 17, 25]);
      const chipRatio = ctx.contrastRatio(ctx.parseHexRgb(fgChip), chipBg);
      assert(chipRatio >= 4.5, `${mode} chip contrast < 4.5 for ${hex}: ${fgChip} (${chipRatio.toFixed(2)})`);

      const fgCard = cards[0].style.getPropertyValue('--msg-fg');
      const cardBg = ctx.blendAlphaOver(ctx.parseHexRgb(hex), 0.16, mode === 'light' ? [255, 255, 255] : [9, 13, 19]);
      const cardRatio = ctx.contrastRatio(ctx.parseHexRgb(fgCard), cardBg);
      assert(cardRatio >= 4.5, `${mode} card contrast < 4.5 for ${hex}: ${fgCard} (${cardRatio.toFixed(2)})`);
    }
  }

  theme = 'dark';
  ctx.setFilterChipColor('FOLLOW', '#FBBF24');
  const darkFg = document.getElementById('feed-filter-follow').style.getPropertyValue('--chip-fg');
  theme = 'light';
  ctx.refreshRemoteFilterChipContrast();
  const lightFg = document.getElementById('feed-filter-follow').style.getPropertyValue('--chip-fg');
  assert.notEqual(darkFg, lightFg, 'theme switch must recompute contrast-adjusted chip foreground color');
  console.log('PASS: remote filter colors preserve hue & guarantee WCAG AA (>=4.5:1) in dark/light themes');
}

// 3. Stats Dock: Target FPS gradient, bitrate gradient vs CQP, separate stream/record toggles, 3-retry disconnect
function testStatsDockBehaviors() {
  const html = read('ui/stats-dock/index.html');
  const scriptSrc = [...html.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/g)].at(-1)[1];

  let theme = 'dark';
  const nodes = new Map();
  const timers = new Map();
  let seq = 0;
  class MockSocket {
    static OPEN = 1;
    constructor() { this.readyState = 1; }
    close() {}
    send() {}
  }

  const document = {
    documentElement: {
      getAttribute: (k) => (k === 'data-theme' ? theme : null),
      setAttribute: (k, v) => { if (k === 'data-theme') theme = v; },
    },
    getElementById(id) {
      if (!nodes.has(id)) {
        const init = (id === 'box-output-details-table' || id === 'row-stream-detail' || id === 'row-record-detail') ? ['hidden'] : [];
        nodes.set(id, createEl(id, init));
      }
      return nodes.get(id);
    },
  };

  const store = {};
  const ctx = {
    document,
    console,
    performance: { now: () => 1000 },
    WebSocket: MockSocket,
    window: { addEventListener() {} },
    localStorage: { getItem: (k) => store[k] ?? null, setItem: (k, v) => { store[k] = v; } },
    chzzkApi: {},
    apiFetch: async () => ({ ok: false, status: 500, json: async () => ({}) }),
    setTimeout(fn, ms) { timers.set(++seq, { fn, ms }); return seq; },
    clearTimeout(id) { timers.delete(id); },
  };
  vm.createContext(ctx);
  vm.runInContext(scriptSrc, ctx);

  // Target FPS from GetVideoSettings (59.94) + Red/Yellow/Green gradient
  ctx.handleMessage({ op: 7, d: { requestId: 'GetVideoSettings-1', responseData: { fpsNumerator: 60000, fpsDenominator: 1001 } } });
  const targetFps = vm.runInContext('targetFps', ctx);
  assert(Math.abs(targetFps - (60000 / 1001)) < 0.01, `expected fractional targetFps ~59.94, got ${targetFps}`);
  const fpsVal = document.getElementById('val-fps');
  ctx.applyFpsThreshold(fpsVal, 0, targetFps);
  assert.equal(fpsVal.style.color, 'rgb(244, 63, 94)');
  ctx.applyFpsThreshold(fpsVal, targetFps / 2, targetFps);
  assert.equal(fpsVal.style.color, 'rgb(251, 191, 36)');
  ctx.applyFpsThreshold(fpsVal, targetFps, targetFps);
  assert.equal(fpsVal.style.color, 'rgb(0, 255, 163)');

  // Light theme FPS gradient switch
  ctx.syncStatsDockTheme('light');
  ctx.applyFpsThreshold(fpsVal, targetFps, targetFps);
  assert.equal(fpsVal.style.color, 'rgb(0, 122, 78)');
  ctx.syncStatsDockTheme('dark');

  // CQP clears inline color while fixed target uses gradient
  const brVal = document.getElementById('val-record-bitrate');
  const brUnit = document.getElementById('label-rec-bitrate-unit');
  ctx.applyBitrateColor(brVal, brUnit, 3000, true, { fixed: true, targetKbps: 6000 });
  assert.equal(brVal.style.color, 'rgb(251, 191, 36)');
  ctx.applyBitrateColor(brVal, brUnit, 3000, true, { fixed: false, targetKbps: 0 });
  assert.equal(brVal.style.color, '');
  assert(brVal.className.includes('text-white'));

  // Independent stream & record toggles
  ctx.toggleOutputDetailRow('stream');
  assert(!document.getElementById('row-stream-detail').classList.contains('hidden'));
  assert(document.getElementById('row-record-detail').classList.contains('hidden'));
  assert(!document.getElementById('box-output-details-table').classList.contains('hidden'));
  assert(document.getElementById('btn-toggle-stream-detail').classList.contains('is-active-stream'));

  ctx.toggleOutputDetailRow('record');
  assert(!document.getElementById('row-record-detail').classList.contains('hidden'));
  assert(document.getElementById('btn-toggle-record-detail').classList.contains('is-active-record'));

  ctx.toggleOutputDetailRow('stream');
  assert(document.getElementById('row-stream-detail').classList.contains('hidden'));
  assert(!document.getElementById('row-record-detail').classList.contains('hidden'));
  assert(!document.getElementById('box-output-details-table').classList.contains('hidden'));

  ctx.toggleOutputDetailRow('record');
  assert(document.getElementById('box-output-details-table').classList.contains('hidden'));
  console.log('PASS: Stats Dock FPS/bitrate gradients, CQP fallback, & independent stream/record toggles');
}

// 4. Contextmenu blocking vs editable/selection + unified topmost Esc closing
function testContextMenuAndEscStack() {
  const apiListeners = {};
  let selectedText = '';
  const apiCtx = {
    window: { getSelection: () => ({ toString: () => selectedText }) },
    document: { addEventListener: (ev, fn) => { apiListeners[ev] = fn; } },
    fetch: async () => ({ ok: true, status: 200, json: async () => ({}) }),
    AbortSignal: { timeout: () => undefined },
    console,
  };
  vm.createContext(apiCtx);
  vm.runInContext(read('ui/shared/api.js'), apiCtx);

  const fireCtxMenu = (isEditable, selText) => {
    selectedText = selText;
    let prevented = false;
    apiListeners.contextmenu({
      target: { closest: () => (isEditable ? {} : null) },
      preventDefault: () => { prevented = true; },
    });
    return prevented;
  };
  assert.equal(fireCtxMenu(false, ''), true, 'empty space right-click must be prevented');
  assert.equal(fireCtxMenu(true, ''), false, 'input/textarea right-click must be allowed');
  assert.equal(fireCtxMenu(false, 'copied text'), false, 'selected text right-click must be allowed');

  const coreNodes = new Map();
  const makeCoreEl = (id, cls = []) => {
    const el = createEl(id, cls);
    el.style.display = id === 'category-dropdown' ? 'block' : '';
    return el;
  };
  for (const id of ['modal-guide-image', 'modal-custom-confirm', 'modal-thumbnail', 'popover-chatmode']) {
    coreNodes.set(id, makeCoreEl(id, []));
  }
  coreNodes.set('settings-panel', makeCoreEl('settings-panel', ['open']));

  const coreListeners = {};
  const coreCtx = {
    document: {
      addEventListener: (ev, fn) => { coreListeners[ev] = fn; },
      createElement: () => ({ textContent: '', innerHTML: '' }),
      getElementById: (id) => coreNodes.get(id) || null,
    },
    window: {},
    console,
    setTimeout,
    clearTimeout,
    closeUpdateModal() {},
    closeDonationBoardModal() {},
    closeDonationModal() {},
    closeThumbnailModal() { coreNodes.get('modal-thumbnail').classList.add('hidden'); },
    closePresetLoadModal() {},
    closePresetSaveModal() {},
    closeFilterColorPopover() {},
    closeRemoteVolumeDrawer() {},
    closeAllPopovers() { coreNodes.get('popover-chatmode').classList.add('hidden'); },
    toggleSettings() { coreNodes.get('settings-panel').classList.remove('open'); },
  };
  vm.createContext(coreCtx);
  vm.runInContext(read('ui/shared/core.js'), coreCtx);

  const sendEsc = () => coreListeners.keydown({ key: 'Escape', isComposing: false, preventDefault() {} });
  sendEsc();
  assert(coreNodes.get('modal-guide-image').classList.contains('hidden'));
  assert(!coreNodes.get('modal-custom-confirm').classList.contains('hidden'));
  sendEsc();
  assert(coreNodes.get('modal-custom-confirm').classList.contains('hidden'));
  assert(!coreNodes.get('modal-thumbnail').classList.contains('hidden'));
  sendEsc();
  assert(coreNodes.get('modal-thumbnail').classList.contains('hidden'));
  assert(!coreNodes.get('popover-chatmode').classList.contains('hidden'));
  sendEsc();
  assert(coreNodes.get('popover-chatmode').classList.contains('hidden'));
  assert(coreNodes.get('settings-panel').classList.contains('open'));
  sendEsc();
  assert(!coreNodes.get('settings-panel').classList.contains('open'));
  console.log('PASS: contextmenu copy/paste allowance & topmost-only Escape stack');
}

// 5. Modal live API fetch on open (openDonationModal syncs stale frontend state from live API)
async function testModalLiveApiSyncOnOpen() {
  const nodes = new Map([['modal-dsetting-chat', createEl('modal-dsetting-chat', ['hidden'])]]);
  let detailCalls = 0;
  const { ctx, document } = createDockTestEnv({
    nodes,
    globals: {
      userChannelId: 'chan-1',
      currentDonationState: { chatDonationActive: true, videoDonationActive: true, missionDonationActive: true, subAlertActive: true },
      checkLoginGuard: () => true,
    },
    apiFetch: async (url) => {
      if (url === '/chzzkdock/donations/details') {
        detailCalls++;
        return {
          ok: true,
          status: 200,
          json: async () => ({
            chat: { donationActive: false, minCurrencyPayAmount: 3000, exposureDonationAmount: 1000, useLargeDonationEnvelope: false },
            video: { donationActive: true, minCurrencyPayAmount: 2000, payAmountPerSecond: 50, maxDuration: 60 },
            mission: { donationActive: false, minCurrencyPayAmount: 5000, failSupportRatio: 20, exposureDonationAmount: 1000 },
            subscription: { alertActive: false, alertVolume: 35, ttsActive: true, ttsVolume: 80 },
          }),
        };
      }
      return { ok: false, status: 404, json: async () => ({}) };
    },
  });
  vm.runInContext(read('ui/main-dock/js/moderation.js'), ctx);

  document.getElementById('toggle-chat-donation').checked = true;
  document.getElementById('toggle-sub-alert').checked = true;
  await ctx.openDonationModal('chat');
  await flush();

  assert.equal(detailCalls, 1, 'openDonationModal must call live /chzzkdock/donations/details');
  assert.equal(document.getElementById('toggle-chat-donation').checked, false, 'stale toggle must sync to live false');
  assert.equal(document.getElementById('dsetting-chat-min-amount').value, '3,000');
  assert.equal(document.getElementById('toggle-sub-alert').checked, false);
  assert.equal(document.getElementById('dsetting-sub-alert-vol').value, 35);
  console.log('PASS: openDonationModal fetches live Chzzk API details & syncs stale UI toggles');
}

// 6. Shortcut settings theme parity (reuses .chzzk-input for dark/light base + active recording override)
function testShortcutTabThemeParity() {
  const mainCss = read('ui/main-dock/style.css');
  const themeCss = read('ui/shared/theme.css');
  const html = read('ui/main-dock/index.html');
  assert(!/\.shortcut-capture\s*\{/.test(mainCss),
    '.shortcut-capture must reuse .chzzk-input instead of duplicating base input styles');
  assert(html.includes('id="overlay-shortcut" class="chzzk-input shortcut-capture'),
    '#overlay-shortcut must apply .chzzk-input');
  assert(themeCss.includes('html[data-theme="light"] .shortcut-capture[aria-pressed="true"]'),
    'theme.css must style active capture recording state in light mode');
  assert(html.includes('id="tab-content-shortcuts"'), 'shortcuts tab content missing');
  console.log('PASS: shortcut settings tab reuses chzzk-input & matches dark/light theme parity');
}

// 7. Party popover always resets back to initial choose view after close (X, Esc, or outside click)
async function testPartyPopoverResetOnClose() {
  const nodes = new Map();
  for (const [id, cls] of [
    ['popover-party', ['hidden', 'w-[86px]', 'p-1.5']],
    ['party-step-choose', []],
    ['party-step-create', ['hidden']],
    ['party-step-join', ['hidden']],
    ['party-step-active', ['hidden']],
    ['btn-party-popover', []],
  ]) {
    const el = createEl(id, cls);
    el.focus = () => {};
    nodes.set(id, el);
  }
  const { ctx } = createDockTestEnv({
    nodes,
    autoCreate: false,
    globals: { userChannelId: 'chan-party-1', checkLoginGuard: () => true },
    chzzkApi: { getParty: async () => ({ ok: true, status: 200, json: async () => ({ participating: false }) }) },
  });
  vm.runInContext(read('ui/main-dock/js/moderation.js'), ctx);

  // Close via X button (togglePartyPopover)
  ctx.togglePartyPopover();
  ctx.showPartySubView('create');
  assert(!nodes.get('party-step-create').classList.contains('hidden'));
  ctx.togglePartyPopover();
  assert(nodes.get('popover-party').classList.contains('hidden'));
  assert(nodes.get('party-step-create').classList.contains('hidden'), 'closing party popover must hide create step');
  assert(!nodes.get('party-step-choose').classList.contains('hidden'), 'closing party popover must reset to choose step');

  // Reopen -> starts on choose, close via Esc (closeAllPopovers)
  ctx.togglePartyPopover();
  ctx.showPartySubView('join');
  ctx.closeAllPopovers();
  assert(nodes.get('popover-party').classList.contains('hidden'));
  assert(nodes.get('party-step-join').classList.contains('hidden'), 'closeAllPopovers must hide join step');
  assert(!nodes.get('party-step-choose').classList.contains('hidden'));

  // Reopen again -> simulate closing directly via closePartyPopover
  ctx.togglePartyPopover();
  ctx.showPartySubView('create');
  ctx.closePartyPopover();
  ctx.togglePartyPopover();
  await flush();
  assert(nodes.get('party-step-create').classList.contains('hidden'), 'reopening after close must not stay on create step');
  assert(!nodes.get('party-step-choose').classList.contains('hidden'), 'reopening after close must show choose step');
  console.log('PASS: party popover resets subview to choose on close and reopen');
}

// 8. Category dropdown: drops ongoing section (allows overlap) + persists across outside-webview blur
async function testCategoryDropdownDropsAndFocusPersistence() {
  const nodes = new Map();
  let focusedEl = null;
  let winHasFocus = true;
  const makeCatEl = (id) => {
    const el = createEl(id, []);
    el.listeners = {};
    el.addEventListener = (ev, fn) => { (el.listeners[ev] = el.listeners[ev] || []).push(fn); };
    el.focus = () => { focusedEl = el; (el.listeners.focus || []).forEach(fn => fn()); };
    el.blur = () => { if (focusedEl === el) focusedEl = null; (el.listeners.blur || []).forEach(fn => fn()); };
    el.contains = (target) => target === el || (id === 'category-wrapper' && (target === nodes.get('category-search') || target === nodes.get('category-dropdown')));
    return el;
  };
  for (const id of ['category-wrapper', 'category-search', 'category-dropdown', 'category-id', 'category-type', 'category-clear', 'category-favorite-toggle', 'category-error-msg', 'live-title']) {
    const el = makeCatEl(id);
    if (id === 'category-dropdown') el.style.display = 'none';
    nodes.set(id, el);
  }
  const catSearch = nodes.get('category-search');
  const dropdown = nodes.get('category-dropdown');

  const savedPatches = [];
  const noop = () => {};
  const { ctx, docListeners, winListeners } = createDockTestEnv({
    nodes,
    autoCreate: false,
    hasFocus: () => winHasFocus,
    getActiveElement: () => focusedEl,
    setTimeout: (fn, ms) => setTimeout(fn, ms > 50 ? 25 : ms),
    globals: {
      userChannelId: 'chan-cat-1',
      setBroadcastFormLocked: noop,
      initConnection: noop,
      checkObsScriptStatus: noop,
      loadBroadcastPresets: noop,
      loadCategoryFavorites: noop,
      checkForUpdate: noop,
      loadAllDockSettings: noop,
    },
    apiFetch: async (url, reqOpts = {}) => {
      if (url === '/app-settings' && reqOpts.method === 'PUT') {
        savedPatches.push(JSON.parse(reqOpts.body));
        return { ok: true, status: 200, json: async () => ({ code: 200 }) };
      }
      if (url === '/chzzkdock/categories/recommended') {
        return {
          ok: true, status: 200,
          json: async () => ({
            categories: [
              { categoryType: 'GAME', categoryId: 'League_of_Legends', categoryValue: '리그 오브 레전드', posterImageUrl: 'https://img/lol.png', dropsCampaignNo: 2097 },
              { categoryType: 'ETC', categoryId: 'talk', categoryValue: 'talk', posterImageUrl: 'https://img/talk.png', dropsCampaignNo: null },
            ],
          }),
        };
      }
      if (url === '/chzzkdock/drops/campaigns') {
        return {
          ok: true, status: 200,
          json: async () => ([
            { campaignNo: 2097, campaignName: '롤 드롭스', categoryType: 'GAME', categoryId: 'League_of_Legends', categoryValue: '리그 오브 레전드', posterImageUrl: 'https://img/lol.png' },
            { campaignNo: 2128, campaignName: '도깨비의세계 드롭스', categoryType: 'GAME', categoryId: 'dokkaebi', categoryValue: '도깨비의세계', posterImageUrl: 'https://img/dokkaebi.png' },
          ]),
        };
      }
      return { ok: false, status: 404, json: async () => ({}) };
    },
  });
  vm.runInContext(read('ui/shared/core.js'), ctx);
  vm.runInContext(read('ui/main-dock/js/moderation.js'), ctx);
  vm.runInContext(read('ui/main-dock/js/settings.js'), ctx);
  vm.runInContext(read('ui/main-dock/js/broadcast.js'), ctx);

  // Set LoL as favorite to confirm Drops section allows overlap with Favorites & Recommendations
  await flush();
  vm.runInContext(`userChannelId = 'chan-cat-1'; categoryFavoritesList = [{ category_id: 'League_of_Legends', category_name: '리그 오브 레전드', category_type: 'GAME', poster_image_url: 'https://img/lol.png' }];`, ctx);
  await vm.runInContext(`renderFavoritesDropdown()`, ctx);
  assert.equal(dropdown.style.display, 'block');
  assert(dropdown.innerHTML.includes('data-cat-section-toggle="favorites"'), 'favorites section toggle missing');
  assert(dropdown.innerHTML.includes('data-cat-section-toggle="recommended"'), 'recommended section toggle missing');
  assert(dropdown.innerHTML.includes('data-cat-section-toggle="drops"'), 'ongoing drops section toggle missing');
  const lolMatches = dropdown.innerHTML.match(/data-id="League_of_Legends"/g) || [];
  assert.equal(lolMatches.length, 2, 'overlap between favorites and ongoing drops must be allowed');
  assert(dropdown.innerHTML.includes('data-id="dokkaebi"'), 'non-favorite active drop must appear in ongoing drops section');

  // Collapsible category section persists via backend settings map and restores across applyDockSettings
  await vm.runInContext(`toggleCategorySection('recommended')`, ctx);
  await flush();
  assert.equal(savedPatches[savedPatches.length - 1].collapsed_category_sections.recommended, true);
  await vm.runInContext(`renderFavoritesDropdown()`, ctx);
  assert(dropdown.innerHTML.includes('data-cat-section-body="recommended" class="hidden"'), 'collapsed section body must render hidden');
  assert(dropdown.innerHTML.includes('data-cat-section-arrow="recommended" class="text-[9.5px] leading-none">▸</span>'), 'collapsed section arrow must show ▸');
  await vm.runInContext(`applyDockSettings({ collapsed_category_sections: { drops: true } }); renderFavoritesDropdown()`, ctx);
  assert(dropdown.innerHTML.includes('data-cat-section-body="drops" class="hidden"'), 'applyDockSettings must restore collapsed section from backend');
  await vm.runInContext(`toggleCategorySection('drops')`, ctx);

  // Favorite add/remove requires confirmation modal; cancel leaves list unchanged, confirm updates it
  let confirmCalls = 0;
  let confirmNext = false;
  ctx.showConfirmModal = async () => { confirmCalls++; return confirmNext; };
  await vm.runInContext(`toggleCategoryFavorite({ categoryId: 'dokkaebi', categoryValue: '도깨비의세계', categoryType: 'GAME', posterImageUrl: 'https://img/dokkaebi.png' })`, ctx);
  assert.equal(confirmCalls, 1, 'toggleCategoryFavorite must prompt confirmation modal');
  assert.equal(vm.runInContext(`categoryFavoritesList.length`, ctx), 1, 'cancelled confirmation must not add favorite');
  confirmNext = true;
  await vm.runInContext(`toggleCategoryFavorite({ categoryId: 'dokkaebi', categoryValue: '도깨비의세계', categoryType: 'GAME', posterImageUrl: 'https://img/dokkaebi.png' })`, ctx);
  assert.equal(confirmCalls, 2, 'second toggle must prompt confirmation modal');
  assert.equal(vm.runInContext(`categoryFavoritesList.length`, ctx), 2, 'approved confirmation must add favorite');

  // Clicking #modal-custom-confirm must NOT close #category-dropdown
  const modalConfirmTarget = { closest: (sel) => sel === '#modal-custom-confirm' ? {} : null };
  (docListeners.click || []).forEach(fn => fn({ target: modalConfirmTarget }));
  await flush();
  assert.equal(dropdown.style.display, 'block', 'clicking #modal-custom-confirm must keep dropdown open');

  // Focusing category-search blurs away due to outside webview click -> dropdown must stay open
  focusedEl = catSearch;
  winHasFocus = false;
  catSearch.blur();
  (winListeners.blur || []).forEach(fn => fn());
  await new Promise(r => setTimeout(r, 40));
  assert.equal(dropdown.style.display, 'block', 'dropdown must remain open after clicking outside webview');

  // Re-activating webview triggers focus on catSearch while dropdown is already open -> DOM must not reset
  dropdown.innerHTML = 'SENTINEL';
  winHasFocus = true;
  catSearch.focus();
  await flush();
  assert.equal(dropdown.innerHTML, 'SENTINEL', 're-focusing webview must not rebuild already-open dropdown');

  // Clicking outside #category-wrapper inside webview closes dropdown
  const outsideNode = { id: 'live-title' };
  (docListeners.click || []).forEach(fn => fn({ target: outsideNode }));
  await flush();
  assert.equal(dropdown.style.display, 'none', 'inside-webview outside click must close dropdown');

  // With dropdown closed, window blur drops stale focus on catSearch so returning to empty space never re-opens dropdown
  focusedEl = catSearch;
  winHasFocus = false;
  (winListeners.blur || []).forEach(fn => fn());
  assert.equal(focusedEl, null, 'window blur with closed dropdown must clear stale focus on category-search');

  // Verify shortcut binding guide & unified donation popover tone in index.html
  const html = read('ui/main-dock/index.html');
  assert(html.includes('단축키 지정 안내 및 주의사항'), 'shortcut binding guide box missing in index.html');
  const donPopoverStart = html.indexOf('id="popover-donations"');
  const donPopoverEnd = html.indexOf('<!-- Modal: Donation', donPopoverStart);
  const donPopoverHtml = html.slice(donPopoverStart, donPopoverEnd > 0 ? donPopoverEnd : donPopoverStart + 5000);
  assert(!donPopoverHtml.includes('bg-[#181524]') && !donPopoverHtml.includes('liquid-amber'), 'popover-donations must use unified dock palette instead of legacy purple/amber');
  assert(donPopoverHtml.includes('bg-[#0F1522]') && donPopoverHtml.includes('border-[#2B3E5E]'), 'popover-donations must use unified #0F1522 / #2B3E5E surface');
  console.log('PASS: category dropdown, collapsible sections, favorite confirm, shortcut guide, and donation tone verified');
}

// 9. Beta notice modal ("다시 보지 않음" via backend storage) & per-mode custom CSS tabs + backdrop close
function testBetaNoticeAndOverlayCssTabs() {
  const managerHtml = read('ui/overlay-settings/manager.html');
  assert(managerHtml.includes('data-css-tab="arrange"'), 'arrange custom CSS tab missing');
  assert(managerHtml.includes('data-css-tab="interact"'), 'interact custom CSS tab missing');
  assert(managerHtml.includes('data-css-tab="pass"'), 'pass custom CSS tab missing');
  assert(managerHtml.includes('data-css-help="arrange"'), 'arrange custom CSS helper missing');
  assert(managerHtml.includes('data-css-help="interact"'), 'interact custom CSS helper missing');
  assert(managerHtml.includes('data-css-help="pass"'), 'pass custom CSS helper missing');
  assert(managerHtml.includes('name="cssArrange"') && managerHtml.includes('예시:'), 'cssArrange textarea/example missing');
  assert(managerHtml.includes('name="cssInteract"') && managerHtml.includes('예시:'), 'cssInteract textarea/example missing');
  assert(managerHtml.includes('name="cssPass"') && managerHtml.includes('예시:'), 'pass custom CSS textarea/example missing');
  assert(managerHtml.includes('id="appearance-settings"') && managerHtml.includes('<summary>크기 · 배경 설정</summary>'), 'collapsible appearance settings missing');
  assert(managerHtml.includes('id="overlay-mode-desc"'), 'mode 1-line description missing');
  assert(managerHtml.includes('data-layout="1"') && managerHtml.includes('data-layout="2"'), '1-col/2-col source layout toggle missing');
  assert(managerHtml.includes('name="chatScale"') && managerHtml.includes('min="100"') && managerHtml.includes('max="170"'), 'chatScale 100%~170% slider missing');

  const overlaySettingsJs = read('ui/overlay-settings/settings.js');
  assert(overlaySettingsJs.includes('backdropDownOutside') && overlaySettingsJs.includes('closeEditor()'), 'overlay-editor backdrop click close missing');
  assert(!overlaySettingsJs.includes('showPopover'), 'add/edit dialogs must both be unified to showModal()');
  assert(overlaySettingsJs.includes('dataset.activeMode'), 'mode callout activeMode sync missing');
  assert(overlaySettingsJs.includes('소스 삭제') && overlaySettingsJs.includes('showConfirmModal'), 'source delete confirmation missing');
  const overlayCss = read('ui/overlay-settings/style.css');
  assert(overlayCss.includes('width: fit-content;'), 'mode-desc callout must hug text width');
  assert(!read('ui/main-dock/index.html').includes('header-reveal') && !read('ui/main-dock/style.css').includes('header-reveal'), 'header-reveal hiding must be removed');

  const savedBeta = [];
  const { ctx } = createDockTestEnv({
    autoCreate: true,
    globals: {
      saveAppSettingField: async (field, value) => {
        savedBeta.push({ field, value: JSON.parse(JSON.stringify(value)) });
        return true;
      },
    },
  });
  vm.runInContext(read('ui/shared/core.js'), ctx);
  let shownCount = 0;
  let lastOpts = null;
  ctx.showNoticeModal = (opts) => { shownCount++; lastOpts = opts; };

  vm.runInContext(`showBetaNoticeModal('revenue', { title: '테스트 안내' })`, ctx);
  assert.equal(shownCount, 1, 'beta notice must show when not dismissed');
  assert.equal(lastOpts.checkboxLabel, '다시 보지 않음', 'beta notice must include dismiss checkbox');
  lastOpts.onConfirm(false);
  vm.runInContext(`showBetaNoticeModal('revenue', { title: '테스트 안내' })`, ctx);
  assert.equal(shownCount, 2, 'unchecked confirm must not suppress future beta notice');
  lastOpts.onConfirm(true);
  assert.equal(savedBeta.length, 1, 'checked confirm must save to backend settings');
  assert.equal(savedBeta[0].field, 'dismissed_beta_notices');
  assert.equal(savedBeta[0].value.revenue, true);
  vm.runInContext(`showBetaNoticeModal('revenue', { title: '테스트 안내' })`, ctx);
  assert.equal(shownCount, 2, 'dismissed betaKey must stay suppressed');
  vm.runInContext(`showBetaNoticeModal('remote', { title: '리모컨 안내' })`, ctx);
  assert.equal(shownCount, 3, 'different betaKey must remain independent');
  console.log('PASS: backend beta notice persistence, per-mode custom CSS examples, & backdrop close verified');
}

(async () => {
  testStaticTailwindParity();
  testRemoteFilterColorContrast();
  testStatsDockBehaviors();
  testContextMenuAndEscStack();
  await testModalLiveApiSyncOnOpen();
  testShortcutTabThemeParity();
  await testPartyPopoverResetOnClose();
  await testCategoryDropdownDropsAndFocusPersistence();
  testBetaNoticeAndOverlayCssTabs();
})().catch((err) => {
  console.error(err);
  process.exit(1);
});
