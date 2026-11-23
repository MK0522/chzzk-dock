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
  const nodes = new Map();
  const cards = [createEl('card-1', ['remote-feed-card'])];
  cards[0].dataset.feedCat = 'FOLLOW';

  const document = {
    documentElement: { getAttribute: (k) => (k === 'data-theme' ? theme : null) },
    addEventListener() {},
    getElementById(id) {
      if (!nodes.has(id)) nodes.set(id, createEl(id, id === 'filter-color-popover' ? ['hidden'] : []));
      return nodes.get(id);
    },
    querySelectorAll(sel) {
      return sel === '#remote-feed-list .remote-feed-card' ? cards : [];
    },
  };

  const ctx = {
    document,
    window: { addEventListener() {} },
    navigator: {},
    console,
    setTimeout,
    clearTimeout,
    EventSource: undefined,
    localStorage: { getItem: () => null, setItem() {} },
    chzzkApi: {},
    apiFetch: async () => ({ ok: false, status: 500, json: async () => ({}) }),
    showToast() {},
    saveAppSettingField() {},
  };
  vm.createContext(ctx);
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
  const nodes = new Map();
  const document = {
    documentElement: { getAttribute: () => 'dark' },
    addEventListener() {},
    getElementById(id) {
      if (!nodes.has(id)) nodes.set(id, createEl(id, id === 'modal-dsetting-chat' ? ['hidden'] : []));
      return nodes.get(id);
    },
    querySelectorAll: () => [],
  };
  let detailCalls = 0;
  const ctx = {
    document,
    window: { addEventListener() {} },
    navigator: {},
    console,
    setTimeout,
    clearTimeout,
    userChannelId: 'chan-1',
    currentDonationState: { chatDonationActive: true, videoDonationActive: true, missionDonationActive: true, subAlertActive: true },
    checkLoginGuard: () => true,
    showToast() {},
    localStorage: { getItem: () => null, setItem() {} },
    chzzkApi: {},
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
  };
  vm.createContext(ctx);
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

(async () => {
  testStaticTailwindParity();
  testRemoteFilterColorContrast();
  testStatsDockBehaviors();
  testContextMenuAndEscStack();
  await testModalLiveApiSyncOnOpen();
})().catch((err) => {
  console.error(err);
  process.exit(1);
});
