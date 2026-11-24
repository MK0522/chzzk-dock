const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { pathToFileURL } = require('node:url');

// Popout DOM captured from the CHZZK chat component. Keep its nickname nesting
// and the button around the emote: both differ from older chat markup.
async function check() {
  const row = document.createElement('div');
  row.className = '_chatting_message_jbnq0_21';
  row.innerHTML = '<button class="_nickname_jbnq0_33" aria-haspopup="true">'
    + '<span class="_container_a6uv5_2"><span class="_nickname_a6uv5_43">Viewer</span></span></button>'
    + '<span class="_text_jbnq0_1">Hello<button class="_button_jbnq0_18"><img alt="wave"></button></span>';
  row.querySelector('img').src = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==';
  const list = document.querySelector('main');
  const pin = document.querySelector('aside');
  pin.append(row.cloneNode(true));
  list.append(row);
  await new Promise(resolve => setTimeout(resolve, 80));
  const visible = el => el.checkVisibility({ visibilityProperty: true, opacityProperty: true });
  const valid = () => visible(row.querySelector('button span span')) && visible(row.querySelector('img'))
    && visible(row.lastElementChild) && !visible(pin.querySelector('img'));
  const hidden = () => !visible(document.querySelector('header')) && !visible(document.querySelector('textarea'));
  const broadcast = valid() && hidden();
  window.chzzkDockOverlay.update(1, 0);
  const edit = valid() && hidden();
  window.chzzkDockOverlay.update(2, 0);
  const interact = visible(pin.querySelector('img')) && !hidden();
  window.chzzkDockOverlay.update(0, 0);
  document.documentElement.dataset.result = String(broadcast && edit && interact && valid());
}

const browser = process.env.CHROME_PATH || path.join(process.env.ProgramFiles, 'Google/Chrome/Application/chrome.exe');
const folder = fs.mkdtempSync(path.join(os.tmpdir(), 'dock-chat-'));
try {
  const file = path.join(folder, 'test.html');
  const overlay = fs.readFileSync(path.join(__dirname, '../core/chat_overlay.js'), 'utf8');
  fs.writeFileSync(file, '<!doctype html><meta charset="utf-8"><body><div id="root">'
    + '<header>Ranking</header><aside aria-label="고정 메시지"></aside><main role="log" class="_container_1li40_1 _exist_fixed_message_1li40_12"></main><textarea></textarea></div>'
    + '<script>(' + overlay + ')(true,"",[0,0]);(' + check + ')();</script>');
  const dom = execFileSync(browser, ['--headless=new', '--no-sandbox', '--disable-gpu',
    '--disable-background-networking', '--user-data-dir=' + path.join(folder, 'profile'),
    '--virtual-time-budget=1500', '--dump-dom', pathToFileURL(file).href],
  { encoding: 'utf8', windowsHide: true, timeout: 30000 });
  assert.match(dom, /<html\b[^>]* data-result="true"/);
  console.log('PASS: first incoming popout chat, inline emote, pinned row exclusion, and mode changes');
} finally {
  fs.rmSync(folder, { recursive: true, force: true, maxRetries: 5 });
}
