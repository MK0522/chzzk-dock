const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { pathToFileURL } = require('node:url');

// Popout DOM captured from the CHZZK chat component. Keep its nickname nesting
// and the button around the emote: both differ from older chat markup.
async function check() {
  const aside = document.getElementById('aside-chatting');
  const topTitle = aside.querySelector('._title_17o7b_12');
  const ranking = aside.querySelector('._container_wl8bq_2');
  const rankingBtn = aside.querySelector('._ranking_button_wl8bq_141');
  const list = aside.querySelector('main[role="log"]');
  const pin = aside.querySelector('aside[aria-label="고정 메시지"]');
  let moreClicks = 0;
  aside.querySelector('button[aria-label="더보기 메뉴"]').addEventListener('click', () => { moreClicks++; });
  rankingBtn.addEventListener('click', () => {
    const next = rankingBtn.getAttribute('aria-expanded') !== 'true';
    rankingBtn.setAttribute('aria-expanded', String(next));
    if (next) {
      const contents = document.createElement('div');
      contents.className = '_contents_wl8bq_84';
      contents.style.height = '180px';
      ranking.append(contents);
    } else {
      ranking.querySelector('._contents_wl8bq_84')?.remove();
    }
  });

  const row = document.createElement('div');
  row.className = '_chatting_message_jbnq0_21';
  row.innerHTML = '<button class="_nickname_jbnq0_33" aria-haspopup="true">'
    + '<span class="_container_a6uv5_2"><span class="_nickname_a6uv5_43">Viewer</span></span></button>'
    + '<span class="_text_jbnq0_1">Hello<button class="_button_jbnq0_18"><img alt="wave"></button></span>';
  row.querySelector('img').src = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==';

  const welcomeItem = document.createElement('div');
  welcomeItem.className = '_item_1li40_7 _big_padding_1li40_44';
  welcomeItem.innerHTML = '<div class="_container_ik55w_1 _welcome_ik55w_18">채팅방에 오신 것을 환영합니다!</div>';
  const dropsItem = document.createElement('div');
  dropsItem.className = '_item_1li40_7 _big_padding_1li40_44';
  dropsItem.innerHTML = '<div class="_container_ik55w_1 _default_ik55w_10"><div class="_inner_ik55w_15"><p class="_title_ik55w_39">드롭스 캠페인이 진행 중인 방송입니다!</p><p class="_description_ik55w_93">방송을 시청하고 특별한 리워드를 받아보세요.</p></div></div>';

  pin.append(row.cloneNode(true));
  list.append(welcomeItem, dropsItem, row);
  await new Promise(resolve => setTimeout(resolve, 100));
  const visible = el => el.checkVisibility({ visibilityProperty: true, opacityProperty: true });
  const valid = () => visible(row.querySelector('button span span')) && visible(row.querySelector('img'))
    && visible(row.lastElementChild) && !visible(pin.querySelector('img'))
    && !visible(welcomeItem) && !visible(dropsItem);
  const hiddenChrome = () => !visible(topTitle) && !visible(document.querySelector('textarea'));
  const broadcast = valid() && hiddenChrome();
  window.chzzkDockOverlay.update(1, 0);
  const edit = valid() && hiddenChrome();

  window.chzzkDockOverlay.update(2, 0, [0, 0, 0, 0], '#141517', 100, 140);
  await new Promise(resolve => setTimeout(resolve, 100));
  const emptyRankingHidden = !visible(ranking);
  const rankItem = document.createElement('span');
  rankItem.className = '_box_19i63_15';
  rankItem.textContent = '1등 Elasia 3,000';
  rankingBtn.append(rankItem);
  await new Promise(resolve => setTimeout(resolve, 100));
  const bottomTools = document.getElementById('chzzk-dock-bottom-tools');
  const sendBtn = document.getElementById('send_chat_or_donate');
  bottomTools?.querySelector('button[aria-label="더보기 메뉴"]')?.click();
  const hasScaleButtons = Boolean(bottomTools?.querySelector('button[aria-label*="마이너스"], button[aria-label*="플러스"]'));
  const logHeightBeforeExpand = list.getBoundingClientRect().height;
  const rankingHeightCollapsed = ranking.getBoundingClientRect().height;
  rankingBtn.click();
  await new Promise(resolve => setTimeout(resolve, 100));
  const logHeightAfterExpand = list.getBoundingClientRect().height;
  const interact = visible(pin.querySelector('img'))
    && visible(document.querySelector('textarea'))
    && !visible(topTitle)
    && !visible(welcomeItem)
    && !visible(dropsItem)
    && emptyRankingHidden
    && visible(ranking)
    && Boolean(bottomTools) && bottomTools.nextElementSibling === sendBtn
    && !hasScaleButtons
    && moreClicks === 1
    && document.documentElement.style.getPropertyValue('--dock-chat-scale') === '1.4'
    && rankingHeightCollapsed >= 30
    && getComputedStyle(ranking).position === 'absolute'
    && Math.abs(logHeightAfterExpand - logHeightBeforeExpand) < 1;

  window.chzzkDockOverlay.update(0, 0);
  document.documentElement.dataset.result = String(broadcast && edit && interact && valid());
}

const browser = process.env.CHROME_PATH || path.join(process.env.ProgramFiles, 'Google/Chrome/Application/chrome.exe');
const folder = fs.mkdtempSync(path.join(os.tmpdir(), 'dock-chat-'));
try {
  const file = path.join(folder, 'test.html');
  const overlay = fs.readFileSync(path.join(__dirname, '../core/chat_overlay.js'), 'utf8');
  fs.writeFileSync(file, '<!doctype html><meta charset="utf-8"><body style="margin:0"><div id="root">'
    + '<aside id="aside-chatting" style="display:flex;flex-direction:column;height:100vh;position:relative">'
    + '<div class="_container_17o7b_2" style="height:44px;position:relative"><h2 class="_title_17o7b_12">채팅</h2>'
    + '<div class="_wrapper_17o7b_28 _scale_17o7b_39"><button type="button" aria-label="채팅 크기 마이너스">-</button><span class="_scale_text_17o7b_46" aria-live="polite">100%</span><button type="button" aria-label="채팅 크기 플러스">+</button></div>'
    + '<div class="_wrapper_17o7b_28 _menu_17o7b_36"><button type="button" aria-haspopup="true" aria-expanded="false" aria-label="더보기 메뉴">⋮</button></div></div>'
    + '<div class="_container_wl8bq_2"><button type="button" class="_ranking_button_wl8bq_141 _is_shrunk_wl8bq_148" style="height:35px" aria-expanded="false">Rank1 Rank2</button></div>'
    + '<aside aria-label="고정 메시지"></aside>'
    + '<main role="log" class="_container_1li40_1 _exist_fixed_message_1li40_12" style="flex:1;overflow:hidden"></main>'
    + '<div class="_area_nu1j9_49"><textarea></textarea><div class="_tools_19u4u_125" style="display:flex"><div class="_donation_19u4u_132"><span class="_donation_text_19u4u_137">후원하기</span></div><button id="send_chat_or_donate" type="button">채팅</button></div></div>'
    + '</aside></div>'
    + '<script>(' + overlay + ')(true,"",[0,0]);(' + check + ')();</script>');
  const dom = execFileSync(browser, ['--headless=new', '--no-sandbox', '--disable-gpu',
    '--disable-background-networking', '--user-data-dir=' + path.join(folder, 'profile'),
    '--virtual-time-budget=2000', '--dump-dom', pathToFileURL(file).href],
  { encoding: 'utf8', windowsHide: true, timeout: 30000 });
  assert.match(dom, /<html\b[^>]* data-result="true"/);
  console.log('PASS: first incoming popout chat, inline emote, drops/welcome hide, bottom tools, & zero-reflow ranking verified');
} finally {
  fs.rmSync(folder, { recursive: true, force: true, maxRetries: 5 });
}
