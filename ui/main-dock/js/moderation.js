// ============================================================
//  CHZZK OBS Dock - Moderation, Donations & Remote Controller
//  ui/main-dock/js/moderation.js
// ============================================================

var currentDonationState = {
  chatDonationActive: false,
  videoDonationActive: false,
  missionDonationActive: false
};

var currentChatModeState = {
  chatEmojiMode: false,
  chatSlowModeSec: 0
};

var streamingInfoLoaded = false;
var donationsLoaded = false;
var currentRevenueDate = new Date();
var isRevenuePrivate = true;
var revenueCache = {};
var cachedNewsFeeds = [];
var currentPartyInfo = null;
var cachedSubscriptionSetting = null;
var isRemoteAlertPaused = false;
var activeRemoteTab = 'all';
var activeMissionSort = 'LATEST';
var locallySettledMissions = {};
var userSubscriptionAvailable = false;
var remotePollTimer = null;

var remoteFilterChips = {
  FOLLOW: true,
  SUB: true,
  DONATION: true,
  AD: true,
  SHOP: true
};

var customFilterColors = {};
var activeFilterColorKey = null;
const REMOTE_FILTER_LABELS = {
  FOLLOW: '팔로우',
  SUB: '구독',
  DONATION: '후원',
  AD: '광고',
  SHOP: '상품 구매'
};

var remoteVolumes = {
  tts: 70,
  video: 70,
  chatAlert: 70,
  videoAlert: 70,
  missionAlert: 70
};

var remoteVolumeTimers = {};

// ===== 공통 물방울 슬라이딩 알약 탭 헬퍼 =====
function moveLiquidPill(groupOrId, activeBtn, animate = true) {
  const group = typeof groupOrId === 'string' ? document.getElementById(groupOrId) : groupOrId;
  if (!group || !activeBtn) return;
  const pill = group.querySelector(':scope > .liquid-pill');
  if (!pill) return;
  if (!activeBtn.offsetWidth || !activeBtn.offsetHeight) return;
  if (!animate) pill.style.transition = 'none';
  pill.style.transform = 'none';
  pill.style.left = `${activeBtn.offsetLeft}px`;
  pill.style.top = `${activeBtn.offsetTop}px`;
  pill.style.width = `${activeBtn.offsetWidth}px`;
  pill.style.height = `${activeBtn.offsetHeight}px`;
  if (!animate) {
    void pill.offsetWidth;
    pill.style.transition = '';
  }
}

// ===== 팝오버 공통 제어 (채널, 채팅모드, 후원, 파티) =====
const ALL_POPOVER_IDS = ['channel-popover-menu', 'popover-chatmode', 'popover-donations', 'popover-party'];

function closeAllPopovers(exceptId = '') {
  ALL_POPOVER_IDS.forEach(id => {
    if (id !== exceptId) document.getElementById(id)?.classList.add('hidden');
  });
  if (exceptId !== 'channel-popover-menu' && typeof stopChannelPopoverPolling === 'function') {
    stopChannelPopoverPolling();
  }
}

function toggleChatModePopover() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('실시간 채팅 모드')) return;
  closeAllPopovers('popover-chatmode');
  const popover = document.getElementById('popover-chatmode');
  if (!popover) return;
  if (popover.classList.contains('hidden')) {
    popover.classList.remove('hidden');
    fetchChatSettingsOnly();
  } else {
    popover.classList.add('hidden');
  }
}

function toggleDonationsPopover() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('후원 및 수익 요약')) return;
  closeAllPopovers('popover-donations');
  const popover = document.getElementById('popover-donations');
  if (!popover) return;
  if (popover.classList.contains('hidden')) {
    popover.classList.remove('hidden');
    requestAnimationFrame(() => {
      const activeBtn = document.querySelector('#donations-liquid-tabs .liquid-tab-btn.active');
      moveLiquidPill('donations-liquid-tabs', activeBtn, false);
    });
    refreshDonationsAndRevenue(true);
  } else {
    popover.classList.add('hidden');
  }
}

function switchDonationsPopoverTab(tab) {
  const tabSettings = document.getElementById('donations-popover-content-settings');
  const tabRev = document.getElementById('donations-popover-content-revenue');
  const btnSettings = document.getElementById('donations-popover-tab-settings');
  const btnRev = document.getElementById('donations-popover-tab-revenue');

  if (tab === 'settings') {
    if (tabSettings) {
      tabSettings.classList.remove('hidden');
      tabSettings.classList.remove('tab-pane-animate');
      void tabSettings.offsetWidth;
      tabSettings.classList.add('tab-pane-animate');
    }
    if (tabRev) tabRev.classList.add('hidden');
    btnSettings?.classList.add('active');
    btnRev?.classList.remove('active');
    moveLiquidPill('donations-liquid-tabs', btnSettings, true);
  } else {
    if (tabSettings) tabSettings.classList.add('hidden');
    if (tabRev) {
      tabRev.classList.remove('hidden');
      tabRev.classList.remove('tab-pane-animate');
      void tabRev.offsetWidth;
      tabRev.classList.add('tab-pane-animate');
    }
    btnSettings?.classList.remove('active');
    btnRev?.classList.add('active');
    moveLiquidPill('donations-liquid-tabs', btnRev, true);
    openRevenueNoticeModal();
    fetchRevenueSummary();
  }
}

async function refreshDonationsAndRevenue(silent = false) {
  const icon = document.getElementById('donations-popover-refresh-icon');
  if (icon) icon.classList.add('animate-spin');
  try {
    await Promise.all([
      fetchDonationsAndChatMode(silent),
      fetchRevenueSummary()
    ]);
    if (!silent) showToast('후원 및 수익 데이터 갱신 완료', 'success');
  } finally {
    if (icon) setTimeout(() => icon.classList.remove('animate-spin'), 350);
  }
}

function applyChatModeUI(chat) {
  currentChatModeState = {
    chatEmojiMode: !!chat.emojiOnly,
    chatSlowModeSec: Number(chat.slowSec) || 0
  };
  const elEmoji = document.getElementById('toggle-emoji-mode');
  const elSlow = document.getElementById('select-slow-mode');
  const elSlowStatus = document.getElementById('slow-mode-status');
  if (elEmoji) elEmoji.checked = currentChatModeState.chatEmojiMode;
  if (elSlow) elSlow.value = String(currentChatModeState.chatSlowModeSec);
  if (elSlowStatus) {
    if (currentChatModeState.chatSlowModeSec > 0) {
      elSlowStatus.textContent = formatSlowSec(currentChatModeState.chatSlowModeSec);
      elSlowStatus.className = 'text-[10px] text-[#00FFA3] font-bold font-mono';
    } else {
      elSlowStatus.textContent = 'OFF';
      elSlowStatus.className = 'text-[10px] text-slate-400 font-mono font-semibold';
    }
  }
}

async function fetchChatSettingsOnly() {
  const icon = document.getElementById('chatmode-refresh-icon');
  if (icon) icon.classList.add('animate-spin');
  try {
    const [modeRes, ruleRes] = await Promise.all([
      chzzkApi.getChatMode(),
      chzzkApi.getChatRule().catch(() => null)
    ]);
    if (modeRes && modeRes.ok) {
      const chat = await modeRes.json();
      applyChatModeUI(chat);
    }
    if (ruleRes && ruleRes.ok) {
      const ruleData = await ruleRes.json();
      const chatContent = ruleData.content || ruleData;
      if (chatContent) {
        if (chatContent.chatAvailableGroup && typeof setSelectedChatGroup === 'function') {
          setSelectedChatGroup(chatContent.chatAvailableGroup);
        }
        const elMin = document.getElementById('min-follower-minute');
        if (elMin && chatContent.minFollowerMinute !== undefined) {
          elMin.value = chatContent.minFollowerMinute;
        }
        const elSubEx = document.getElementById('toggle-sub-exception');
        if (elSubEx && chatContent.allowSubscriberInFollowerMode !== undefined) {
          elSubEx.checked = chatContent.allowSubscriberInFollowerMode;
        }
      }
    }
  } catch (e) {
    console.warn('채팅 모드 조회 오류:', e);
  } finally {
    if (icon) setTimeout(() => icon.classList.remove('animate-spin'), 350);
  }
}



// ===== 외부 브라우저 및 웹뷰 창 제어 =====
async function openExternalUrl(url, label = '페이지') {
  if (!url || (!url.startsWith('https://') && !url.startsWith('http://'))) {
    showToast('열기 실패', 'warn', '유효한 웹 URL이 아닙니다.');
    return false;
  }
  try {
    const res = await apiFetch(`/open-browser?url=${encodeURIComponent(url)}`);
    if (res.ok) {
      showToast('기본 브라우저 열기', 'info', `${label}을(를) 브라우저로 열었습니다.`);
      return true;
    }
    const data = await res.json().catch(() => ({}));
    showToast('브라우저 열기 실패', 'error', data.message || '브라우저 실행에 실패했습니다.');
    return false;
  } catch (e) {
    console.error('브라우저 열기 요청 실패:', e);
    showToast('브라우저 열기 실패', 'error', '서버 통신 상태를 확인하세요.');
    return false;
  }
}

async function openChatWebview(overlay = false) {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('채팅창 열기')) return;
  try {
    if (!userChannelId) await resolveChannelId();
    const res = await chzzkApi.openChatWebview(userChannelId, overlay);
    if (res.ok) {
      const data = await res.json();
      if (data.status === 'started') {
        if (overlay) {
          showToast('채팅 오버레이', 'success', 'Ctrl+Alt+T로 모드를 바꿀 수 있습니다.');
        } else {
          showToast('💬 채팅창 실행됨', 'success', '독립 채팅창이 열렸습니다.');
        }
      } else {
        showToast('채팅창 알림', 'info', data.message || '채팅창이 실행되었습니다.');
      }
    } else {
      showToast('채팅창 실행 실패', 'error', '서버에서 채팅창을 열지 못했습니다.');
    }
  } catch (e) {
    showToast('실행 오류', 'error', e.message);
  }
}

async function openRemoteWebview() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('리모컨 미니 창')) return;
  try {
    if (!userChannelId) await resolveChannelId();
    const res = await chzzkApi.openRemoteWebview(userChannelId);
    if (res.ok) {
      const data = await res.json();
      if (data.status === 'started') {
        showToast('🎮 리모컨 창 실행됨', 'success', '독립 리모컨 미니 창이 열렸습니다.');
      } else {
        showToast('리모컨 창 알림', 'info', data.message || '리모컨 창이 실행되었습니다.');
      }
    } else {
      showToast('리모컨 실행 실패', 'error', '서버에서 리모컨 창을 열지 못했습니다.');
    }
  } catch (e) {
    showToast('실행 오류', 'error', e.message);
  }
}

// ===== 치지직 공식 규격 리모컨 (Verified BFF Controller) =====
const REMOTE_CHIP_TYPE_FILTERS = {
  FOLLOW: 'FOLLOW',
  SUB: 'SUBSCRIPTION,GIFT',
  DONATION: 'DONATION,MISSION,MISSION_PARTICIPATION,VIDEO,PARTY_DONATION',
  AD: 'LIVE_AD',
  SHOP: 'STREAMER_SHOP_PURCHASE'
};

function startRemotePolling() {
  stopRemotePolling();
  remotePollTimer = setInterval(() => {
    const panel = document.getElementById('remote-panel');
    if (!panel || !panel.classList.contains('open')) {
      stopRemotePolling();
      return;
    }
    fetchNewsFeed(true);
  }, 10000);
}

function stopRemotePolling() {
  if (remotePollTimer) {
    clearInterval(remotePollTimer);
    remotePollTimer = null;
  }
}

function toggleRemotePanel() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('치지직 리모컨')) return;
  closeAllPopovers();
  const panel = document.getElementById('remote-panel');
  const settingsPanel = document.getElementById('settings-panel');
  const urlsPanel = document.getElementById('urls-panel');
  if (settingsPanel && settingsPanel.classList.contains('open')) settingsPanel.classList.remove('open');
  if (urlsPanel && urlsPanel.classList.contains('open')) urlsPanel.classList.remove('open');
  if (panel) {
    panel.classList.toggle('open');
    if (panel.classList.contains('open')) {
      loadRemoteState();
      switchRemoteTab(activeRemoteTab || 'all');
      startRemotePolling();
      requestAnimationFrame(() => {
        const activeBtn = document.getElementById(`remote-tab-btn-${activeRemoteTab || 'all'}`);
        moveLiquidPill('remote-liquid-tabs', activeBtn, false);
      });
    } else {
      closeRemoteVolumeDrawer();
      stopRemotePolling();
    }
  }
  if (typeof updatePanelScrollLock === 'function') updatePanelScrollLock();
}

function openRemoteVolumeDrawer() {
  document.getElementById('remote-vol-backdrop')?.classList.add('open');
  document.getElementById('remote-vol-drawer')?.classList.add('open');
}

function closeRemoteVolumeDrawer() {
  document.getElementById('remote-vol-backdrop')?.classList.remove('open');
  document.getElementById('remote-vol-drawer')?.classList.remove('open');
}

function syncPauseAlertButtonUI() {
  const iconEl = document.getElementById('icon-remote-pause');
  const textEl = document.getElementById('text-remote-pause');
  const btnEl = document.getElementById('btn-remote-pause-alert');
  if (isRemoteAlertPaused) {
    if (iconEl) iconEl.textContent = '▶️';
    if (textEl) textEl.textContent = '알림 재개';
    if (btnEl) btnEl.className = 'py-2 px-2 rounded-xl bg-amber-950/60 hover:bg-amber-900/80 border border-amber-600/50 text-amber-300 font-bold text-xs transition-colors cursor-pointer flex items-center justify-center gap-1.5 shadow-md';
  } else {
    if (iconEl) iconEl.textContent = '⏸️';
    if (textEl) textEl.textContent = '알림 정지';
    if (btnEl) btnEl.className = 'py-2 px-2 rounded-xl bg-[#1E1936] hover:bg-[#28214A] border border-[#5C3BFF]/40 text-[#C4B5FD] font-bold text-xs transition-colors cursor-pointer flex items-center justify-center gap-1.5';
  }
}

function applyRemoteVolumesUI(vols, videoDonationActive = true) {
  const setSlider = (sliderId, labelId, val) => {
    const num = Math.max(0, Math.min(100, Number(val ?? 70)));
    const s = document.getElementById(sliderId);
    const l = document.getElementById(labelId);
    if (s) s.value = num;
    if (l) l.textContent = `${num}%`;
    return num;
  };
  remoteVolumes.tts = setSlider('remote-tts-volume', 'remote-tts-vol-label', vols.tts);
  remoteVolumes.video = setSlider('remote-video-volume', 'remote-video-vol-label', vols.video);
  remoteVolumes.chatAlert = setSlider('remote-alert-chat-vol', 'remote-alert-chat-label', vols.chatAlert);
  remoteVolumes.videoAlert = setSlider('remote-alert-video-vol', 'remote-alert-video-label', vols.videoAlert);
  remoteVolumes.missionAlert = setSlider('remote-alert-mission-vol', 'remote-alert-mission-label', vols.missionAlert);

  const vidSlider = document.getElementById('remote-video-volume');
  const vidRow = document.getElementById('remote-video-vol-row');
  const vidTitle = document.getElementById('remote-video-vol-title');
  const vidOffBadge = document.getElementById('remote-video-off-badge');
  const enabled = videoDonationActive !== false;
  if (vidSlider) vidSlider.disabled = false;
  if (vidRow) vidRow.classList.remove('opacity-45');
  if (vidTitle) vidTitle.textContent = '영상 후원 볼륨';
  if (vidOffBadge) vidOffBadge.classList.toggle('hidden', enabled);
}

async function loadRemoteState() {
  try {
    const res = await chzzkApi.getRemoteState();
    if (!res.ok) return;
    const data = await res.json();
    isRemoteAlertPaused = !!data.paused;
    syncPauseAlertButtonUI();
    applyRemoteVolumesUI(data.volumes || {}, data.videoDonationActive !== false);
  } catch (e) {
    console.warn('[Remote] 초기 상태 동기화 오류:', e);
  }
}

function sendVolumeCommandDebounced(targetType, volumeVal) {
  clearTimeout(remoteVolumeTimers[targetType]);
  remoteVolumeTimers[targetType] = setTimeout(async () => {
    try {
      const res = await chzzkApi.setRemoteVolume(targetType, volumeVal);
      if (res.ok) {
        const labels = {
          TTS: '채팅 후원 TTS',
          VIDEO: '영상 후원',
          CHAT: '채팅후원 알림음',
          VIDEO_ALERT: '영상후원 알림음',
          MISSION: '미션후원 알림음'
        };
        showToast('🔊 볼륨 저장 완료', 'info', `${labels[targetType] || targetType}: ${volumeVal}%`);
      } else {
        const err = await res.json().catch(() => ({}));
        showToast('볼륨 변경 실패', 'error', err.message || '서버 볼륨 설정을 반영하지 못했습니다.');
      }
    } catch (e) {
      showToast('볼륨 변경 오류', 'error', e.message);
    }
  }, 350);
}

function onTTSVolumeChange(val) {
  const num = Number(val);
  remoteVolumes.tts = num;
  const lbl = document.getElementById('remote-tts-vol-label');
  if (lbl) lbl.textContent = `${num}%`;
  sendVolumeCommandDebounced('TTS', num);
}

function onVideoVolumeChange(val) {
  const num = Number(val);
  remoteVolumes.video = num;
  const lbl = document.getElementById('remote-video-vol-label');
  if (lbl) lbl.textContent = `${num}%`;
  sendVolumeCommandDebounced('VIDEO', num);
}

function onAlertVolumeChange(type, val) {
  const num = Number(val);
  const map = {
    chat: { key: 'chatAlert', labelId: 'remote-alert-chat-label', target: 'CHAT' },
    video: { key: 'videoAlert', labelId: 'remote-alert-video-label', target: 'VIDEO_ALERT' },
    mission: { key: 'missionAlert', labelId: 'remote-alert-mission-label', target: 'MISSION' }
  };
  const cfg = map[type];
  if (!cfg) return;
  remoteVolumes[cfg.key] = num;
  const lbl = document.getElementById(cfg.labelId);
  if (lbl) lbl.textContent = `${num}%`;
  sendVolumeCommandDebounced(cfg.target, num);
}

async function postponeMidrollAd() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('중간 광고 미루기')) return;
  try {
    const res = await chzzkApi.postponeAd();
    if (res.ok) {
      showToast('⏰ 광고 10분 미루기 완료', 'success', '중간 광고 시작 시간을 10분 연장했습니다.');
      fetchNewsFeed(true);
    } else {
      const err = await res.json().catch(() => ({}));
      showToast('광고 미루기 실패', 'error', err.message || '중간 광고를 미룰 수 없습니다.');
    }
  } catch (e) {
    showToast('광고 미루기 오류', 'error', e.message);
  }
}

async function skipTTS() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('TTS 넘기기')) return;
  try {
    const res = await chzzkApi.sendRemoteCommand('SKIP');
    if (res.ok) {
      showToast('⏭️ TTS 넘기기 완료', 'success', '현재 재생 중인 TTS 후원을 건너뛰었습니다.');
    } else {
      const err = await res.json().catch(() => ({}));
      showToast('TTS 넘기기 실패', 'error', err.message || '네이버 스튜디오 명령 전달 실패');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function togglePauseAlert() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('알림 정지/재개')) return;
  const nextPaused = !isRemoteAlertPaused;
  try {
    const res = await chzzkApi.setRemotePause(nextPaused);
    if (res.ok) {
      isRemoteAlertPaused = nextPaused;
      syncPauseAlertButtonUI();
      showToast(
        nextPaused ? '⏸️ 방송 알림 일시정지' : '▶️ 방송 알림 재개',
        nextPaused ? 'info' : 'success',
        nextPaused ? '후원 알림 송출이 일시 중단되었습니다.' : '후원 알림 송출이 재개되었습니다.'
      );
    } else {
      const err = await res.json().catch(() => ({}));
      showToast('알림 제어 실패', 'error', err.message || '알림 정지/재개 상태를 변경할 수 없습니다.');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function replayVideoDonation(donationId, newsFeedNo, cmd = 'PLAY') {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('영상 후원 제어')) return;
  try {
    const res = await chzzkApi.sendRemoteCommand(cmd, donationId, newsFeedNo);
    if (res.ok) {
      const labels = { PLAY: '▶ 영상 재생 요청', STOP: '■ 영상 중지 요청', IGNORE: '영상 숨김 요청' };
      showToast(labels[cmd] || '영상 명령 완료', 'success');
      fetchNewsFeed(true);
    } else {
      const err = await res.json().catch(() => ({}));
      showToast('영상 제어 실패', 'error', err.message || '명령을 실행할 수 없습니다.');
    }
  } catch (e) {
    showToast('영상 제어 오류', 'error', e.message);
  }
}

async function replayTTSDonation(donationId, newsFeedNo, isSub = false) {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('TTS 다시 듣기')) return;
  const cmd = isSub ? 'SUBSCRIPTION_TTS_REPLAY' : 'DONATION_TTS_REPLAY';
  try {
    const res = await chzzkApi.sendRemoteCommand(cmd, donationId, newsFeedNo);
    if (res.ok) {
      showToast('🔊 TTS 다시 재생 요청', 'success');
    } else {
      const err = await res.json().catch(() => ({}));
      showToast('TTS 다시 재생 실패', 'error', err.message || 'TTS 재생을 요청할 수 없습니다.');
    }
  } catch (e) {
    showToast('TTS 재생 오류', 'error', e.message);
  }
}

function toggleRemoteFilterChip(key) {
  if (!(key in remoteFilterChips)) return;
  remoteFilterChips[key] = !remoteFilterChips[key];
  const btn = document.getElementById(`feed-filter-${key.toLowerCase()}`);
  if (btn) btn.classList.toggle('on', remoteFilterChips[key]);
  fetchNewsFeed();
}

function hexToRgba(hex, alpha) {
  const clean = String(hex || '').replace('#', '');
  if (clean.length !== 6) return '';
  const r = parseInt(clean.slice(0, 2), 16);
  const g = parseInt(clean.slice(2, 4), 16);
  const b = parseInt(clean.slice(4, 6), 16);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}

function parseHexRgb(hex) {
  const clean = String(hex || '').replace('#', '');
  if (clean.length !== 6) return [0, 255, 163];
  return [
    parseInt(clean.slice(0, 2), 16),
    parseInt(clean.slice(2, 4), 16),
    parseInt(clean.slice(4, 6), 16)
  ];
}

function rgbToHex(r, g, b) {
  const toHex = (n) => Math.max(0, Math.min(255, Math.round(n))).toString(16).padStart(2, '0');
  return `#${toHex(r)}${toHex(g)}${toHex(b)}`.toUpperCase();
}

function rgbToHsl(r, g, b) {
  const rn = r / 255, gn = g / 255, bn = b / 255;
  const max = Math.max(rn, gn, bn), min = Math.min(rn, gn, bn);
  let h = 0, s = 0;
  const l = (max + min) / 2;
  if (max !== min) {
    const d = max - min;
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
    switch (max) {
      case rn: h = (gn - bn) / d + (gn < bn ? 6 : 0); break;
      case gn: h = (bn - rn) / d + 2; break;
      case bn: h = (rn - gn) / d + 4; break;
    }
    h *= 60;
  }
  return [h, s, l];
}

function hslToRgb(h, s, l) {
  const c = (1 - Math.abs(2 * l - 1)) * s;
  const hp = (h % 360) / 60;
  const x = c * (1 - Math.abs((hp % 2) - 1));
  let r1 = 0, g1 = 0, b1 = 0;
  if (hp >= 0 && hp < 1) { r1 = c; g1 = x; }
  else if (hp < 2) { r1 = x; g1 = c; }
  else if (hp < 3) { g1 = c; b1 = x; }
  else if (hp < 4) { g1 = x; b1 = c; }
  else if (hp < 5) { r1 = x; b1 = c; }
  else { r1 = c; b1 = x; }
  const m = l - c / 2;
  return [Math.round((r1 + m) * 255), Math.round((g1 + m) * 255), Math.round((b1 + m) * 255)];
}

function relativeLuminance(rgb) {
  const lin = rgb.map((v) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  });
  return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2];
}

function contrastRatio(rgbA, rgbB) {
  const l1 = relativeLuminance(rgbA);
  const l2 = relativeLuminance(rgbB);
  return (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05);
}

function blendAlphaOver(fgRgb, alpha, baseRgb) {
  return [
    Math.round(fgRgb[0] * alpha + baseRgb[0] * (1 - alpha)),
    Math.round(fgRgb[1] * alpha + baseRgb[1] * (1 - alpha)),
    Math.round(fgRgb[2] * alpha + baseRgb[2] * (1 - alpha))
  ];
}

// 방법 A: 선택한 색조(Hue)·채도는 유지하면서 현재 테마 배경에서 WCAG AA(>=4.5:1)를 만족하도록 명도만 자동 보정
function getAccessibleFilterFgColor(hex, tintAlpha = 0.13, surfaceKind = 'chip') {
  const upper = String(hex || '').toUpperCase();
  if (!/^#[0-9A-F]{6}$/.test(upper)) return upper;

  const isLight = typeof document !== 'undefined' && document.documentElement?.getAttribute?.('data-theme') === 'light';
  const rawRgb = parseHexRgb(upper);

  let baseRgb;
  if (isLight) {
    baseRgb = surfaceKind === 'card' ? [255, 255, 255] : [248, 250, 252];
  } else {
    baseRgb = surfaceKind === 'card' ? [9, 13, 19] : [13, 17, 25];
  }
  const effectiveBg = tintAlpha > 0 ? blendAlphaOver(rawRgb, tintAlpha, baseRgb) : baseRgb;

  if (contrastRatio(rawRgb, effectiveBg) >= 4.5) {
    return upper;
  }

  const [h, s, origL] = rgbToHsl(rawRgb[0], rawRgb[1], rawRgb[2]);
  const bgLum = relativeLuminance(effectiveBg);
  const targetRatio = 4.65;

  let lo = bgLum > 0.5 ? 0 : origL;
  let hi = bgLum > 0.5 ? origL : 1;
  let bestRgb = bgLum > 0.5 ? [15, 23, 42] : [248, 250, 252];

  for (let i = 0; i < 16; i++) {
    const mid = (lo + hi) / 2;
    const candidate = hslToRgb(h, s, mid);
    const passes = contrastRatio(candidate, effectiveBg) >= targetRatio;
    if (bgLum > 0.5) {
      // 라이트 배경: 가능한 원래 색 밝기에 가까운 가장 높은 명도를 찾음
      if (passes) {
        bestRgb = candidate;
        lo = mid;
      } else {
        hi = mid;
      }
    } else {
      // 다크 배경: 가능한 원래 색 밝기에 가까운 가장 낮은 명도를 찾음
      if (passes) {
        bestRgb = candidate;
        hi = mid;
      } else {
        lo = mid;
      }
    }
  }
  return rgbToHex(bestRgb[0], bestRgb[1], bestRgb[2]);
}

function setChipCssColorVars(btn, hex) {
  if (!btn) return;
  if (typeof hex === 'string' && /^#[0-9a-fA-F]{6}$/.test(hex)) {
    const upper = hex.toUpperCase();
    const fgHex = getAccessibleFilterFgColor(upper, 0.13, 'chip');
    btn.style.setProperty('--chip-bg', hexToRgba(upper, 0.13));
    btn.style.setProperty('--chip-border', hexToRgba(upper, 0.48));
    btn.style.setProperty('--chip-fg', fgHex);
  } else {
    btn.style.removeProperty('--chip-bg');
    btn.style.removeProperty('--chip-border');
    btn.style.removeProperty('--chip-fg');
  }
}

function applyRemoteFilterChipColors(colorsMap) {
  const next = {};
  if (colorsMap && typeof colorsMap === 'object') {
    Object.keys(REMOTE_FILTER_LABELS).forEach(k => {
      const hex = colorsMap[k];
      if (typeof hex === 'string' && /^#[0-9a-fA-F]{6}$/.test(hex)) {
        next[k] = hex.toUpperCase();
      }
    });
  }
  customFilterColors = next;
  Object.keys(REMOTE_FILTER_LABELS).forEach(k => {
    const btn = document.getElementById(`feed-filter-${k.toLowerCase()}`);
    setChipCssColorVars(btn, customFilterColors[k] || '');
  });
  applyRemoteFilterColorsToFeedCards();
  if (activeFilterColorKey) {
    const label = document.getElementById('filter-color-target-label');
    if (label) {
      const curColor = customFilterColors[activeFilterColorKey] || '#00FFA3';
      label.style.color = getAccessibleFilterFgColor(curColor, 0, 'popover');
    }
  }
}

function refreshRemoteFilterChipContrast() {
  applyRemoteFilterChipColors(customFilterColors);
}

function getFeedCategoryKey(fType) {
  const upper = String(fType || '').toUpperCase();
  if (upper === 'LIVE_AD' || upper === 'MIDROLL_AD' || upper.includes('AD')) return 'AD';
  if (upper === 'FOLLOW' || upper === 'FOLLOW_CHANNEL') return 'FOLLOW';
  if (upper === 'STREAMER_SHOP_PURCHASE') return 'SHOP';
  if (upper === 'SUBSCRIPTION' || upper === 'GIFT' || upper === 'SUBSCRIPTION_GIFT') return 'SUB';
  return 'DONATION';
}

function applyRemoteFilterColorsToFeedCards(previewKey = null, previewHex = null) {
  const cards = document.querySelectorAll('#remote-feed-list .remote-feed-card');
  cards.forEach(card => {
    const cat = card.dataset.feedCat;
    const hex = (previewKey && cat === previewKey) ? previewHex : customFilterColors[cat];
    if (typeof hex === 'string' && /^#[0-9a-fA-F]{6}$/.test(hex)) {
      const upper = hex.toUpperCase();
      const fgHex = getAccessibleFilterFgColor(upper, 0.16, 'card');
      card.style.setProperty('--msg-bg', hexToRgba(upper, 0.10));
      card.style.setProperty('--msg-border', hexToRgba(upper, 0.48));
      card.style.setProperty('--msg-badge-bg', hexToRgba(upper, 0.16));
      card.style.setProperty('--msg-fg', fgHex);
      card.setAttribute('data-custom-colored', 'true');
    } else {
      card.style.removeProperty('--msg-bg');
      card.style.removeProperty('--msg-border');
      card.style.removeProperty('--msg-badge-bg');
      card.style.removeProperty('--msg-fg');
      card.removeAttribute('data-custom-colored');
    }
  });
}

function openFilterColorPopover(ev, key) {
  if (ev) {
    ev.preventDefault();
    ev.stopPropagation();
  }
  if (!REMOTE_FILTER_LABELS[key]) return;
  activeFilterColorKey = key;
  const pop = document.getElementById('filter-color-popover');
  const label = document.getElementById('filter-color-target-label');
  const picker = document.getElementById('filter-color-picker');
  const curColor = customFilterColors[key] || '#00FFA3';
  if (label) {
    label.textContent = REMOTE_FILTER_LABELS[key];
    label.style.color = getAccessibleFilterFgColor(curColor, 0, 'popover');
  }
  if (picker) picker.value = curColor;
  if (pop) pop.classList.remove('hidden');
}

function closeFilterColorPopover() {
  const pop = document.getElementById('filter-color-popover');
  if (pop && !pop.classList.contains('hidden')) {
    pop.classList.add('hidden');
  }
  activeFilterColorKey = null;
  applyRemoteFilterChipColors(customFilterColors);
}

function previewFilterChipColor(hex) {
  if (!activeFilterColorKey || !/^#[0-9a-fA-F]{6}$/.test(hex || '')) return;
  const btn = document.getElementById(`feed-filter-${activeFilterColorKey.toLowerCase()}`);
  setChipCssColorVars(btn, hex);
  applyRemoteFilterColorsToFeedCards(activeFilterColorKey, hex);
  const label = document.getElementById('filter-color-target-label');
  if (label) label.style.color = getAccessibleFilterFgColor(hex, 0, 'popover');
}

function setFilterChipColor(key, hex) {
  const targetKey = key || activeFilterColorKey;
  if (!targetKey || !REMOTE_FILTER_LABELS[targetKey]) return;
  if (typeof hex !== 'string' || !/^#[0-9a-fA-F]{6}$/.test(hex)) return;
  const upper = hex.toUpperCase();
  customFilterColors[targetKey] = upper;
  applyRemoteFilterChipColors(customFilterColors);
  const label = document.getElementById('filter-color-target-label');
  const picker = document.getElementById('filter-color-picker');
  if (label) label.style.color = getAccessibleFilterFgColor(upper, 0, 'popover');
  if (picker) picker.value = upper;
  if (typeof saveAppSettingField === 'function') {
    saveAppSettingField('remote_filter_colors', customFilterColors);
  }
}

function resetFilterChipColor(resetAll) {
  if (resetAll) {
    customFilterColors = {};
  } else if (activeFilterColorKey) {
    delete customFilterColors[activeFilterColorKey];
  }
  applyRemoteFilterChipColors(customFilterColors);
  if (typeof saveAppSettingField === 'function') {
    saveAppSettingField('remote_filter_colors', customFilterColors, resetAll ? '필터 색상이 기본값으로 초기화되었습니다.' : '선택한 필터 색상이 기본값으로 복원되었습니다.');
  }
  closeFilterColorPopover();
}

document.addEventListener('click', (e) => {
  const pop = document.getElementById('filter-color-popover');
  if (!pop || pop.classList.contains('hidden')) return;
  if (pop.contains(e.target) || e.target.closest('.remote-filter-chip')) return;
  closeFilterColorPopover();
});

function switchRemoteTab(tab) {
  activeRemoteTab = tab;
  if (tab !== 'all') closeFilterColorPopover();
  ['all', 'video', 'mission', 'party'].forEach(t => {
    document.getElementById(`remote-tab-btn-${t}`)?.classList.toggle('active', t === tab);
  });
  moveLiquidPill('remote-liquid-tabs', document.getElementById(`remote-tab-btn-${tab}`), true);

  const filterBar = document.getElementById('remote-filter-bar');
  const feedContent = document.getElementById('remote-tab-content-feed');
  const missionContent = document.getElementById('remote-tab-content-mission');

  if (filterBar) filterBar.classList.toggle('hidden', tab !== 'all');
  if (feedContent) feedContent.classList.toggle('hidden', tab === 'mission');
  if (missionContent) {
    missionContent.classList.toggle('hidden', tab !== 'mission');
    if (tab === 'mission') {
      requestAnimationFrame(() => {
        const activeSortBtn = document.getElementById(activeMissionSort === 'AMOUNT' ? 'mission-sort-amount' : 'mission-sort-latest');
        moveLiquidPill('mission-sort-group', activeSortBtn, false);
      });
    }
  }
  fetchNewsFeed();
}

function getActiveTypeFiltersForTab() {
  if (activeRemoteTab === 'video') return 'VIDEO';
  if (activeRemoteTab === 'mission') return 'MISSION, MISSION_PARTICIPATION';
  if (activeRemoteTab === 'party') return 'PARTY,PARTY_DONATION';
  const parts = Object.entries(remoteFilterChips)
    .filter(([, on]) => on)
    .map(([k]) => REMOTE_CHIP_TYPE_FILTERS[k]);
  return parts.join(',');
}

// ===== 실시간 후원 피드 & 미션 판정 로직 =====
function formatTimeAgo(timestamp) {
  if (!timestamp && timestamp !== 0) return '';
  let date;
  if (typeof timestamp === 'number') {
    date = new Date(timestamp < 10000000000 ? timestamp * 1000 : timestamp);
  } else if (typeof timestamp === 'string') {
    const clean = timestamp.trim().replace(' ', 'T');
    date = new Date(clean);
    if (isNaN(date.getTime())) date = new Date(timestamp);
  } else {
    date = new Date(timestamp);
  }
  if (isNaN(date.getTime())) return '';
  const diffSec = Math.floor((Date.now() - date.getTime()) / 1000);
  if (diffSec < 60) return '방금 전';
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}분 전`;
  const diffHour = Math.floor(diffMin / 60);
  if (diffHour < 24) return `${diffHour}시간 전`;
  return `${Math.floor(diffHour / 24)}일 전`;
}

function parseExtraJson(item) {
  if (!item?.extraJson || typeof item.extraJson !== 'string') return {};
  try {
    return JSON.parse(item.extraJson) || {};
  } catch (_) {
    return {};
  }
}

function extractFeedAmount(item, ex) {
  return Number(
    item.payAmount ??
    item.donationAmount ??
    item.missionDonation?.amount ??
    ex.payAmount ??
    ex.amount ??
    ex.donationAmount ??
    0
  ) || 0;
}

function extractFeedText(item, ex) {
  return (
    item.donationText ||
    item.donationVideoDescription ||
    item.missionDonation?.missionText ||
    item.title ||
    item.message ||
    ex.donationText ||
    ex.missionText ||
    ex.message ||
    ''
  );
}

async function fetchNewsFeed(silent = false) {
  if (!userChannelId) return;
  const typeFilters = getActiveTypeFiltersForTab();
  if (activeRemoteTab === 'all' && !typeFilters) {
    cachedNewsFeeds = [];
    renderRemoteFeed([]);
    return;
  }
  try {
    const res = await chzzkApi.getRemoteFeeds(typeFilters, 20);
    if (res.ok) {
      const data = await res.json();
      const items = data.feeds || [];
      cachedNewsFeeds = Array.isArray(items) ? items : [];
      if (activeRemoteTab === 'mission') {
        renderMissionList(cachedNewsFeeds);
      } else {
        renderRemoteFeed(cachedNewsFeeds);
      }
    } else if (!silent) {
      const err = await res.json().catch(() => ({}));
      showToast('피드 조회 실패', 'error', err.message || '피드 데이터를 불러오지 못했습니다.');
    }
  } catch (e) {
    console.error('fetchNewsFeed failed:', e);
    if (!silent) showToast('피드 통신 오류', 'error', e.message);
  }
}

function renderRemoteFeed(items) {
  const container = document.getElementById('remote-feed-list');
  if (!container) return;

  const list = items || [];
  if (list.length === 0) {
    const emptyMsg = activeRemoteTab === 'party'
      ? '최근 파티 후원 내역이 없습니다'
      : activeRemoteTab === 'video'
        ? '최근 영상 후원 내역이 없습니다'
        : '조건에 맞는 방송 알림 내역이 없습니다';
    container.innerHTML = `
      <div class="text-center py-10 text-[11px] text-slate-400 bg-[#090D13] rounded-lg border border-[#1A2333]/60">
        ${emptyMsg}
      </div>`;
    return;
  }

  const typeMeta = {
    DONATION: { icon: '🧀', label: '치즈', color: 'text-amber-400' },
    DONATION_CHAT_CURRENCY: { icon: '🧀', label: '치즈', color: 'text-amber-400' },
    VIDEO: { icon: '🎬', label: '영도', color: 'text-indigo-400' },
    DONATION_VIDEO_CURRENCY: { icon: '🎬', label: '영도', color: 'text-indigo-400' },
    MISSION: { icon: '🎯', label: '미션', color: 'text-emerald-400' },
    MISSION_PARTICIPATION: { icon: '🎯', label: '미션참여', color: 'text-emerald-400' },
    DONATION_MISSION_SUGGESTION: { icon: '🎯', label: '미션', color: 'text-emerald-400' },
    DONATION_MISSION_PARTICIPATION: { icon: '🎯', label: '미션참여', color: 'text-emerald-400' },
    PARTY: { icon: '🎉', label: '파티', color: 'text-pink-400' },
    PARTY_DONATION: { icon: '🎉', label: '파티후원', color: 'text-pink-400' },
    SUBSCRIPTION: { icon: '⭐', label: '구독', color: 'text-cyan-400' },
    GIFT: { icon: '🎁', label: '구독선물', color: 'text-cyan-400' },
    SUBSCRIPTION_GIFT: { icon: '🎁', label: '구독선물', color: 'text-cyan-400' },
    FOLLOW: { icon: '❤️', label: '팔로우', color: 'text-rose-400' },
    FOLLOW_CHANNEL: { icon: '❤️', label: '팔로우', color: 'text-rose-400' },
    STREAMER_SHOP_PURCHASE: { icon: '🛍️', label: '상품구매', color: 'text-emerald-300' }
  };

  container.innerHTML = list.map(item => {
    const ex = parseExtraJson(item);
    const fType = item.newsFeedType || '';
    const feedCat = getFeedCategoryKey(fType);
    const itemTime = item.createdDate || item.createTime || item.donationTime || ex.createdDate;
    const timeStr = formatTimeAgo(itemTime);
    const safeNewsFeedNo = item.newsFeedNo != null ? item.newsFeedNo : null;
    const nickname = item.user?.nickname || (item.anonymous ? '익명의 후원자' : '시청자');
    const payAmt = extractFeedAmount(item, ex);
    const amountStr = payAmt > 0 ? `${payAmt.toLocaleString()} 치즈` : '';
    const bodyText = extractFeedText(item, ex);
    const safeDonationId = escapeHtml(item.donationId || '');

    if (fType === 'LIVE_AD' || fType === 'MIDROLL_AD' || fType.includes('AD')) {
      const adTitle = bodyText || '5분 뒤 중간 광고가 시작됩니다.';
      return `
        <div class="remote-feed-card p-2.5 rounded-lg bg-[#090D13] border border-[#1A2333] hover:border-[#1E2738] transition-colors text-xs space-y-1.5" data-feed-cat="${feedCat}">
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-1.5 min-w-0">
              <span class="msg-badge-accent px-1.5 py-0.5 rounded bg-[#161F2E] text-slate-300 border border-[#243044] text-[9px] font-black flex-shrink-0">AD</span>
              <span class="font-bold text-slate-100 truncate text-[11.5px]">${escapeHtml(adTitle)}</span>
            </div>
            ${timeStr ? `<span class="text-[10px] text-slate-400 font-medium flex-shrink-0">${timeStr}</span>` : ''}
          </div>
          <div class="flex items-center justify-between gap-2">
            <span class="text-[10.5px] text-slate-400">방송 당 최대 2회 10분 미루기</span>
            <button type="button" onclick="postponeMidrollAd()" class="px-2 py-0.5 rounded bg-[#161F2E] hover:bg-[#1E2A3D] text-slate-200 border border-[#243044] text-[10.5px] font-bold transition-all cursor-pointer flex-shrink-0">
              ⏰ 10분 미루기
            </button>
          </div>
        </div>`;
    }

    if (fType === 'FOLLOW' || fType === 'FOLLOW_CHANNEL') {
      return `
        <div class="remote-feed-card p-2 rounded-lg bg-[#090D13] border border-[#1A2333] text-xs flex items-center justify-between gap-2" data-feed-cat="${feedCat}">
          <div class="flex items-center gap-1.5 min-w-0">
            <span>❤️</span>
            <span class="msg-tag-accent font-bold text-slate-200 truncate max-w-[140px]">${escapeHtml(nickname)}</span>
            <span class="text-slate-300 text-[11px] truncate">님이 팔로우했습니다.</span>
          </div>
          ${timeStr ? `<span class="text-[10px] text-slate-400 font-medium flex-shrink-0">${timeStr}</span>` : ''}
        </div>`;
    }

    if (fType === 'STREAMER_SHOP_PURCHASE') {
      return `
        <div class="remote-feed-card p-2 rounded-lg bg-[#090D13] border border-[#1A2333] text-xs space-y-1" data-feed-cat="${feedCat}">
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-1.5 min-w-0">
              <span>🛍️</span>
              <span class="font-bold text-slate-200 truncate">${escapeHtml(nickname)}</span>
              <span class="msg-badge-accent text-[9.5px] px-1 rounded bg-[#161F2E] border border-[#243044] text-emerald-300 font-medium">상품구매</span>
            </div>
            ${timeStr ? `<span class="text-[10px] text-slate-400 flex-shrink-0">${timeStr}</span>` : ''}
          </div>
          ${bodyText ? `<div class="text-[11px] text-slate-300 truncate">${escapeHtml(bodyText)}</div>` : ''}
        </div>`;
    }

    const meta = typeMeta[fType] || { icon: '💬', label: '알림', color: 'text-slate-300' };
    const isVideo = fType === 'VIDEO' || fType === 'DONATION_VIDEO_CURRENCY' || !!item.videoId;
    const isSub = fType === 'SUBSCRIPTION' || fType === 'GIFT' || fType === 'SUBSCRIPTION_GIFT';
    const isChatTTS = !isVideo && (fType === 'DONATION' || fType === 'DONATION_CHAT_CURRENCY' || isSub) && item.donationId;

    return `
      <div class="remote-feed-card p-2 rounded-lg bg-[#090D13] border border-[#1A2333] hover:border-[#1E2738] transition-colors text-xs space-y-1.5" data-feed-cat="${feedCat}">
        <div class="flex items-center justify-between gap-2">
          <div class="flex items-center gap-1.5 min-w-0">
            <span>${meta.icon}</span>
            <span class="font-bold text-slate-200 truncate max-w-[120px]">${escapeHtml(nickname)}</span>
            <span class="msg-badge-accent text-[9.5px] px-1 py-0.2 rounded border border-transparent font-medium bg-[#161F2E] ${meta.color}">${meta.label}</span>
          </div>
          <div class="flex items-center gap-2 flex-shrink-0">
            ${amountStr ? `<span class="msg-tag-accent font-mono font-bold text-[#00FFA3] text-[11px]">${amountStr}</span>` : ''}
            ${timeStr ? `<span class="text-[10px] text-slate-400 font-medium">${timeStr}</span>` : ''}
          </div>
        </div>
        ${bodyText ? `<div class="text-[11px] text-slate-300 break-words leading-snug bg-[#05080C]/70 p-1.5 rounded">${escapeHtml(bodyText)}</div>` : ''}
        ${isVideo && item.donationId ? `
          <div class="flex justify-end gap-1.5">
            <button type="button" onclick="replayVideoDonation('${safeDonationId}', ${safeNewsFeedNo}, 'PLAY')" class="px-2 py-0.5 rounded bg-[#161F2E] hover:bg-[#00FFA3]/20 text-[#00FFA3] border border-[#00FFA3]/40 text-[10px] font-bold cursor-pointer transition-all">▶ 재생</button>
            <button type="button" onclick="replayVideoDonation('${safeDonationId}', ${safeNewsFeedNo}, 'STOP')" class="px-2 py-0.5 rounded bg-[#121721] hover:bg-[#1E2738] text-slate-300 border border-[#1E2738] text-[10px] cursor-pointer transition-all">■ 중지</button>
          </div>` : ''}
        ${isChatTTS ? `
          <div class="flex justify-end">
            <button type="button" onclick="replayTTSDonation('${safeDonationId}', ${safeNewsFeedNo}, ${isSub})" class="px-2 py-0.5 rounded bg-[#161F2E] hover:bg-[#5C3BFF]/25 text-slate-200 hover:text-white border border-[#243044] text-[10px] font-medium cursor-pointer transition-all">🔊 다시 듣기</button>
          </div>` : ''}
      </div>`;
  }).join('');
  applyRemoteFilterColorsToFeedCards();
}

function setMissionSort(sortType) {
  activeMissionSort = sortType;
  const btnLatest = document.getElementById('mission-sort-latest');
  const btnAmount = document.getElementById('mission-sort-amount');
  btnLatest?.classList.toggle('active', sortType === 'LATEST');
  btnAmount?.classList.toggle('active', sortType === 'AMOUNT');
  moveLiquidPill('mission-sort-group', sortType === 'AMOUNT' ? btnAmount : btnLatest, true);
  renderMissionList(cachedNewsFeeds);
}

function renderMissionList(items) {
  const activeContainer = document.getElementById('remote-mission-active-list');
  const completedContainer = document.getElementById('remote-mission-completed-list');
  const activeCountBadge = document.getElementById('remote-mission-active-count');
  const completedCountBadge = document.getElementById('remote-mission-completed-count');
  if (!activeContainer || !completedContainer) return;

  const missionItems = (items || [])
    .filter(i => i.newsFeedType && i.newsFeedType.includes('MISSION'))
    .map(item => {
      const ex = parseExtraJson(item);
      const md = item.missionDonation || ex.missionDonation || {};
      const missionId = String(md.missionDonationId || item.missionDonationId || md.relatedMissionDonationId || item.donationId || '');
      const payAmount = extractFeedAmount(item, ex);
      const missionText = extractFeedText(item, ex) || '미션 내용 없음';
      const rawStatus = (md.status || item.status || ex.status || '').toUpperCase();
      return { item, md, missionId, payAmount, missionText, rawStatus };
    });

  missionItems.sort((a, b) => {
    if (activeMissionSort === 'AMOUNT') {
      return b.payAmount - a.payAmount;
    }
    const aTime = a.item.createdDate || a.item.newsFeedNo || 0;
    const bTime = b.item.createdDate || b.item.newsFeedNo || 0;
    return bTime > aTime ? 1 : bTime < aTime ? -1 : 0;
  });

  const activeList = [];
  const completedList = [];

  missionItems.forEach(entry => {
    const localStatus = entry.missionId ? locallySettledMissions[entry.missionId] : null;
    let isCompleted = false;
    let outcome = localStatus || null;

    if (localStatus) {
      isCompleted = true;
    } else if (['SUCCESS', 'SUCCEED', 'COMPLETED_SUCCESS'].includes(entry.rawStatus) || (entry.item.newsFeedType || '').includes('SUCCEED')) {
      isCompleted = true;
      outcome = 'succeed';
    } else if (['FAIL', 'FAILED', 'REJECT', 'REJECTED'].includes(entry.rawStatus) || (entry.item.newsFeedType || '').includes('FAIL')) {
      isCompleted = true;
      outcome = 'fail';
    } else if (['COMPLETED', 'SETTLED', 'CANCELED'].includes(entry.rawStatus) || (entry.item.newsFeedType || '').includes('SETTLED')) {
      isCompleted = true;
      outcome = 'completed';
    }

    if (isCompleted) {
      completedList.push({ ...entry, outcome });
    } else {
      activeList.push(entry);
    }
  });

  if (activeCountBadge) activeCountBadge.textContent = `${activeList.length}건`;
  if (completedCountBadge) completedCountBadge.textContent = `${completedList.length}건`;

  if (activeList.length === 0) {
    activeContainer.innerHTML = `
      <div class="text-center py-6 text-[10.5px] text-slate-400 bg-[#090D13] rounded-lg border border-[#1A2333]/60">
        현재 진행 중인 미션이 없습니다
      </div>`;
  } else {
    activeContainer.innerHTML = activeList.map(entry => {
      const nickname = entry.item.user?.nickname || '후원자';
      const amount = entry.payAmount > 0 ? `${entry.payAmount.toLocaleString()} 치즈` : '';
      return `
        <div class="p-2 rounded-lg bg-[#090D13] border border-[#1A2333] space-y-1.5 text-xs">
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-1.5 min-w-0">
              <span>🎯</span>
              <span class="font-bold text-slate-200 truncate">${escapeHtml(nickname)}</span>
            </div>
            <span class="font-mono text-[#00FFA3] font-bold text-[11px] flex-shrink-0">${amount}</span>
          </div>
          <div class="text-[11px] text-slate-300 bg-[#05080C] p-2 rounded border border-[#1E2738]/50 break-words leading-normal">
            ${escapeHtml(entry.missionText)}
          </div>
          ${entry.missionId ? `
            <div class="grid grid-cols-2 gap-1.5 pt-0.5">
              <button type="button" onclick="settleMission('${escapeHtml(entry.missionId)}', 'succeed')" class="py-1 rounded bg-emerald-950/80 hover:bg-emerald-900 border border-emerald-700/60 text-[#00FFA3] font-bold text-[10.5px] cursor-pointer transition-colors">✅ 성공 판정</button>
              <button type="button" onclick="settleMission('${escapeHtml(entry.missionId)}', 'fail')" class="py-1 rounded bg-rose-950/80 hover:bg-rose-900 border border-rose-700/60 text-rose-300 font-bold text-[10.5px] cursor-pointer transition-colors">❌ 실패 판정</button>
            </div>` : ''}
        </div>`;
    }).join('');
  }

  if (completedList.length === 0) {
    completedContainer.innerHTML = `
      <div class="text-center py-6 text-[10.5px] text-slate-500 bg-[#090D13] rounded-lg border border-[#1A2333]/60">
        완료된 미션 내역이 없습니다
      </div>`;
  } else {
    completedContainer.innerHTML = completedList.map(entry => {
      const nickname = entry.item.user?.nickname || '후원자';
      const amount = entry.payAmount > 0 ? `${entry.payAmount.toLocaleString()} 치즈` : '';
      let statusBadge = '<span class="text-[9.5px] px-1.5 py-0.2 rounded font-bold bg-slate-800 text-slate-300 border border-slate-700">🏁 완료</span>';
      if (entry.outcome === 'succeed') {
        statusBadge = '<span class="text-[9.5px] px-1.5 py-0.2 rounded font-bold bg-emerald-950 text-emerald-400 border border-emerald-800/60">✅ 성공</span>';
      } else if (entry.outcome === 'fail') {
        statusBadge = '<span class="text-[9.5px] px-1.5 py-0.2 rounded font-bold bg-rose-950 text-rose-400 border border-rose-800/60">❌ 실패</span>';
      }
      return `
        <div class="p-2 rounded-lg bg-[#070A0F] border border-[#161F2E] space-y-1.5 text-xs opacity-80">
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-1.5 min-w-0">
              <span class="text-slate-400">🎯</span>
              <span class="font-medium text-slate-300 truncate">${escapeHtml(nickname)}</span>
              ${statusBadge}
            </div>
            <span class="font-mono text-slate-400 text-[10.5px] flex-shrink-0">${amount}</span>
          </div>
          <div class="text-[10.5px] text-slate-400 bg-[#05080C]/80 p-1.5 rounded border border-[#1A2333]/40 break-words leading-tight">
            ${escapeHtml(entry.missionText)}
          </div>
        </div>`;
    }).join('');
  }
}

async function settleMission(missionDonationId, outcome) {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('미션 판정')) return;
  try {
    locallySettledMissions[missionDonationId] = outcome;
    const res = await chzzkApi.settleMission(outcome, missionDonationId);
    if (res.ok) {
      showToast(outcome === 'succeed' ? '🎉 미션 성공 판정 완료' : '⚠️ 미션 실패 처리 완료', 'success');
      fetchNewsFeed(true);
    } else {
      delete locallySettledMissions[missionDonationId];
      const data = await res.json().catch(() => ({}));
      showToast('미션 판정 실패', 'error', data.message || '판정 요청 전달 실패');
      renderMissionList(cachedNewsFeeds);
    }
  } catch (e) {
    delete locallySettledMissions[missionDonationId];
    showToast('오류 발생', 'error', e.message);
    renderMissionList(cachedNewsFeeds);
  }
}

// ===== 치지직 합방 파티 관리 (메인 독 팝오버) =====
function togglePartyPopover() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('합방 파티')) return;
  closeAllPopovers('popover-party');
  const popover = document.getElementById('popover-party');
  if (!popover) return;
  if (popover.classList.contains('hidden')) {
    popover.classList.remove('hidden');
    refreshPartyStatus(false);
  } else {
    popover.classList.add('hidden');
  }
}

function showPartySubView(step) {
  const pop = document.getElementById('popover-party');
  if (pop) {
    if (step === 'choose') {
      pop.classList.remove('left-0', 'p-2.5');
      pop.classList.add('w-[86px]', 'p-1.5');
    } else {
      pop.classList.remove('w-[86px]', 'p-1.5');
      pop.classList.add('left-0', 'p-2.5');
    }
  }
  ['choose', 'create', 'join', 'active'].forEach(s => {
    document.getElementById(`party-step-${s}`)?.classList.toggle('hidden', s !== step);
  });
  if (step === 'create') document.getElementById('input-party-create-name')?.focus();
  if (step === 'join') document.getElementById('input-party-join-link')?.focus();
}

async function pastePartyInviteClipboard() {
  const input = document.getElementById('input-party-join-link');
  try {
    const text = await navigator.clipboard.readText();
    if (text && input) {
      input.value = text.trim();
      showToast('📋 초대 링크 붙여넣기 완료', 'info');
    } else {
      showToast('클립보드 비어 있음', 'warn', '복사된 초대 링크가 없습니다.');
    }
  } catch (e) {
    showToast('붙여넣기 안내', 'warn', 'Ctrl+V로 초대 링크를 직접 붙여넣어 주세요.');
    input?.focus();
  }
}

function updatePartyToolbarButtonUI(data) {
  const btn = document.getElementById('btn-party-popover');
  const labelText = document.getElementById('party-btn-text');
  const badge = document.getElementById('party-btn-count-badge');
  const active = !!(data && data.participating);
  if (btn) btn.classList.toggle('party-btn-participating', active);
  if (labelText) labelText.textContent = active ? '파티 진행중' : '파티';
  if (badge) {
    badge.classList.toggle('hidden', !active);
    if (active) badge.textContent = String(data.memberCount || (data.members ? data.members.length : 1));
  }
}

async function openPartyMemberLive(channelId, channelName) {
  if (!channelId) return;
  try {
    const res = await chzzkApi.openChannelLive(channelId);
    if (res.ok) {
      showToast('기본 브라우저 열기', 'info', `${channelName || '멤버'}님의 방송을 열었습니다.`);
    } else {
      const err = await res.json().catch(() => ({}));
      showToast('방송 열기 실패', 'error', err.message || '채널을 열 수 없습니다.');
    }
  } catch (e) {
    showToast('방송 열기 오류', 'error', e.message);
  }
}

function renderActivePartyView(data) {
  const titleEl = document.getElementById('party-active-name-title');
  const countEl = document.getElementById('party-member-count');
  const hostControls = document.getElementById('party-host-controls');
  const editInput = document.getElementById('input-party-edit-name');
  const inviteInput = document.getElementById('field-party-invite-url');
  const listEl = document.getElementById('party-member-list');
  const exitBtn = document.getElementById('btn-party-exit');

  const isHost = !!data.isHost;
  const members = Array.isArray(data.members) ? data.members : [];
  const count = data.memberCount || members.length || 1;

  if (titleEl) titleEl.textContent = data.partyName || '파티';
  if (countEl) countEl.textContent = String(count);
  if (hostControls) hostControls.classList.toggle('hidden', !isHost);
  if (isHost) {
    if (editInput && document.activeElement !== editInput) {
      editInput.value = data.partyName || '';
    }
    if (inviteInput) {
      inviteInput.value = data.inviteUrl || (data.inviteToken ? `https://studio.chzzk.naver.com/party-invite/${data.inviteToken}` : '');
    }
  }

  if (listEl) {
    if (members.length === 0) {
      listEl.innerHTML = '<div class="text-center py-3 text-[11px] text-slate-400">참여자 정보를 불러오는 중...</div>';
    } else {
      listEl.innerHTML = members.map(m => {
        const name = m.channelName || '멤버';
        const rowClass = m.isMe
          ? 'border-[#00FFA3]/75 bg-[#00FFA3]/10 hover:bg-[#00FFA3]/18'
          : 'border-transparent hover:bg-[#141C2B]';
        const badgeText = m.isMe ? '나' : escapeHtml(name.slice(0, 1));
        const avatarClass = m.isMe
          ? 'bg-[#00FFA3]/25 border-[#00FFA3]/60 text-[#00FFA3]'
          : m.isHost
            ? 'bg-[#5C3BFF]/40 border-[#9E8BFF]/60 text-white'
            : 'bg-[#161F2E] border-[#2B3E5E] text-slate-300';
        const safeCid = escapeHtml(m.channelId || '');
        const safeName = (name || '').replace(/'/g, "\\'");
        return `
          <button type="button" onclick="openPartyMemberLive('${safeCid}', '${safeName}')" class="w-full px-2 py-1 rounded-md border ${rowClass} flex items-center justify-between cursor-pointer transition-colors" title="클릭하여 기본 브라우저에서 ${escapeHtml(name)} 방송 열기">
            <div class="flex items-center gap-2 min-w-0">
              <span class="w-5 h-5 rounded-full border ${avatarClass} flex items-center justify-center text-[9px] font-bold flex-shrink-0">${badgeText}</span>
              <span class="text-[11.5px] ${m.isMe ? 'font-bold text-[#00FFA3]' : 'font-medium text-slate-200'} truncate">${escapeHtml(name)}</span>
            </div>
            ${m.isHost ? '<span class="text-[10px] px-1.5 py-0.2 rounded bg-[#5C3BFF]/25 text-[#C4B5FD] font-bold flex-shrink-0">👑 호스트</span>' : ''}
          </button>`;
      }).join('');
    }
  }

  if (exitBtn) {
    if (isHost) {
      exitBtn.textContent = '파티 종료';
      exitBtn.onclick = endPartySubmit;
    } else {
      exitBtn.textContent = '🚪 파티 나가기';
      exitBtn.onclick = leavePartySubmit;
    }
  }
}

async function refreshPartyStatus(showFeedback = false) {
  if (!userChannelId) return;
  const refreshIcon = document.getElementById('party-refresh-icon');
  if (refreshIcon && showFeedback) refreshIcon.classList.add('animate-spin');
  try {
    const res = await chzzkApi.getParty();
    if (!res.ok) return;
    const data = await res.json();
    currentPartyInfo = data;
    updatePartyToolbarButtonUI(data);

    const popoverOpen = !document.getElementById('popover-party')?.classList.contains('hidden');
    if (popoverOpen) {
      if (data.participating) {
        renderActivePartyView(data);
        showPartySubView('active');
      } else {
        const creating = !document.getElementById('party-step-create')?.classList.contains('hidden');
        const joining = !document.getElementById('party-step-join')?.classList.contains('hidden');
        if (!creating && !joining) showPartySubView('choose');
      }
    }

    if (showFeedback) {
      showToast('🎉 파티 상태 동기화 완료', 'info');
    }
  } catch (e) {
    console.error('refreshPartyStatus error:', e);
  } finally {
    if (refreshIcon) setTimeout(() => refreshIcon.classList.remove('animate-spin'), 350);
  }
}

function extractPartyInviteToken(raw) {
  if (!raw || typeof raw !== 'string') return '';
  let str = raw.trim();
  const matchInvite = str.match(/party-invite\/([a-zA-Z0-9_-]+)/i);
  if (matchInvite && matchInvite[1]) return matchInvite[1];
  const matchTokenParam = str.match(/[?&]token=([a-zA-Z0-9_-]+)/i);
  if (matchTokenParam && matchTokenParam[1]) return matchTokenParam[1];
  return str.replace(/^["'<(\[]+|["'>)\]]+$/g, '');
}

async function joinPartyByInviteLink(inputId) {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('파티 합류')) return;
  const input = document.getElementById(inputId);
  const token = extractPartyInviteToken(input ? input.value : '');
  if (!token) {
    showToast('초대 링크 확인', 'warn', '유효한 파티 초대 링크나 코드를 입력해 주세요.');
    input?.focus();
    return;
  }
  try {
    const res = await chzzkApi.joinParty(token);
    if (res.ok) {
      const data = await res.json().catch(() => ({}));
      const title = data.partyName ? `"${data.partyName}"` : '파티';
      showToast(`🎉 ${title} 합류 완료!`, 'success');
      if (input) input.value = '';
      await refreshPartyStatus(false);
    } else {
      const err = await res.json().catch(() => ({}));
      showToast('파티 합류 실패', 'error', err.message || '초대 링크가 만료되었거나 이미 종료된 파티입니다.');
    }
  } catch (e) {
    showToast('합류 오류 발생', 'error', e.message);
  }
}

async function leavePartySubmit() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('파티 나가기')) return;
  const partyNo = currentPartyInfo?.partyNo;
  if (!partyNo) return;
  const ok = await showConfirmModal({
    title: '파티 퇴장',
    message: '정말 현재 파티에서 퇴장하시겠습니까?',
    confirmText: '퇴장하기',
    cancelText: '취소',
    danger: true,
    icon: '🏃'
  });
  if (!ok) return;

  try {
    const res = await chzzkApi.leaveParty(partyNo);
    if (res.ok) {
      showToast('🚪 파티 퇴장 완료', 'info');
      currentPartyInfo = null;
      showPartySubView('choose');
      await refreshPartyStatus(false);
    } else {
      const err = await res.json().catch(() => ({}));
      showToast('파티 퇴장 실패', 'error', err.message || '파티 나가기 실패');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

function togglePartyLinkMask(btn) {
  const input = document.getElementById('field-party-invite-url');
  if (!input) return;
  input.type = input.type === 'password' ? 'text' : 'password';
  if (btn) btn.textContent = input.type === 'password' ? '👁️' : '🙈';
}

function copyPartyInviteUrl() {
  const input = document.getElementById('field-party-invite-url');
  if (!input || !input.value) {
    showToast('복사 실패', 'warn', '복사할 초대 링크가 없습니다.');
    return;
  }
  navigator.clipboard.writeText(input.value).then(() => {
    showToast('🔗 초대 링크 복사 완료', 'success');
  }).catch(() => {
    const prevType = input.type;
    input.type = 'text';
    input.select();
    document.execCommand('copy');
    input.type = prevType;
    showToast('🔗 초대 링크 복사 완료', 'success');
  });
}

async function reissuePartyInviteToken() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('초대 링크 재발급')) return;
  const partyNo = currentPartyInfo?.partyNo;
  if (!partyNo) {
    showToast('재발급 불가', 'warn', '활성화된 파티 정보를 찾을 수 없습니다.');
    return;
  }
  try {
    const res = await chzzkApi.reissuePartyToken(partyNo);
    if (res.ok) {
      const data = await res.json();
      const input = document.getElementById('field-party-invite-url');
      if (input && data.inviteUrl) input.value = data.inviteUrl;
      showToast('🔄 초대 링크 재발급 완료', 'success');
    } else {
      const err = await res.json().catch(() => ({}));
      showToast('재발급 실패', 'error', err.message || '초대 링크 재발급 실패');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function createPartySubmit() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('파티 만들기')) return;
  const input = document.getElementById('input-party-create-name');
  const partyName = input ? input.value.trim() : '';
  if (!partyName) {
    showToast('입력 확인', 'warn', '생성할 파티명을 입력해 주세요.');
    input?.focus();
    return;
  }
  try {
    const res = await chzzkApi.createParty(partyName);
    if (res.ok) {
      showToast('🎉 파티 생성 완료!', 'success', `"${partyName}" 파티가 생성되었습니다.`);
      if (input) input.value = '';
      const createLen = document.getElementById('party-create-len');
      if (createLen) createLen.textContent = '0/60';
      await refreshPartyStatus(false);
    } else {
      const err = await res.json().catch(() => ({}));
      showToast('파티 생성 실패', 'error', err.message || '파티 생성 요청 실패');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function renamePartySubmit() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('파티명 변경')) return;
  const input = document.getElementById('input-party-edit-name');
  const partyName = input ? input.value.trim() : '';
  if (!partyName) {
    showToast('입력 확인', 'warn', '수정할 파티명을 입력해 주세요.');
    input?.focus();
    return;
  }
  try {
    const res = await chzzkApi.renameParty(partyName);
    if (res.ok) {
      showToast('💾 파티명 변경 완료', 'success', `"${partyName}"`);
      await refreshPartyStatus(false);
    } else {
      const err = await res.json().catch(() => ({}));
      showToast('파티명 변경 실패', 'error', err.message || '파티명 수정 요청 실패');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function endPartySubmit() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('파티 종료')) return;
  const ok = await showConfirmModal({
    title: '파티 종료',
    message: '현재 파티를 종료하시겠습니까?',
    submessage: '모든 참여자의 파티 연결이 종료됩니다.',
    confirmText: '파티 종료',
    cancelText: '취소',
    danger: true,
    icon: '🛑'
  });
  if (!ok) return;

  try {
    const status = currentPartyInfo?.status || '';
    const res = await chzzkApi.endParty(status);
    if (res.ok) {
      showToast('⏹️ 파티 종료 완료', 'info');
      currentPartyInfo = null;
      showPartySubView('choose');
      await refreshPartyStatus(false);
    } else {
      const err = await res.json().catch(() => ({}));
      showToast('파티 종료 실패', 'error', err.message || '파티 종료 실패');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

// ===== 금액 자동 콤마 포맷팅 헬퍼 함수 =====
function formatAmountInput(el) {
  if (!el) return;
  const raw = el.value.replace(/[^0-9]/g, '');
  if (!raw) {
    el.value = '';
    return;
  }
  el.value = Number(raw).toLocaleString();
}

function parseFormattedAmount(val) {
  if (!val) return 0;
  return Number(String(val).replace(/[^0-9]/g, '')) || 0;
}

// ===== 스팸 영상 필터 단계별 설명 및 UI 동기화 =====
const SPAM_LEVEL_DESCS = {
  0: '제한없음 (모든 유튜브 영상 가능)',
  1: '조회수 2,000 이상 & 좋아요 200개 이상',
  2: '조회수 30,000 이상 & 좋아요 5,000개 이상',
  3: '조회수 300,000 이상 & 좋아요 10,000개 이상'
};

function updateSpamFilterUI(val) {
  const level = Number(val) || 0;
  const lbl = document.getElementById('dsetting-spam-level-label');
  const desc = document.getElementById('dsetting-spam-desc-text');
  if (lbl) lbl.textContent = level + '레벨';
  if (desc) {
    desc.textContent = SPAM_LEVEL_DESCS[level] || '제한없음';
    if (level === 0) desc.className = 'font-medium text-slate-300';
    else if (level === 1) desc.className = 'font-medium text-emerald-400';
    else if (level === 2) desc.className = 'font-medium text-amber-400';
    else desc.className = 'font-medium text-rose-400';
  }
}

// ===== 구독 활성화 여부 확인 및 잠금 UI =====
async function checkChannelMonetization() {
  try {
    const res = await apiFetch('/chzzkdock/profile');
    if (res.ok) {
      const profile = await res.json();
      userSubscriptionAvailable = !!profile.subscriptionAvailable;
      return userSubscriptionAvailable;
    }
  } catch (e) {}
  userSubscriptionAvailable = false;
  return false;
}

function updateDonationDetailButtonsState() {
  const chatOn = !!document.getElementById('toggle-chat-donation')?.checked;
  const vidOn = !!document.getElementById('toggle-video-donation')?.checked;
  const misOn = !!document.getElementById('toggle-mission-donation')?.checked;
  const subOn = !!document.getElementById('toggle-sub-alert')?.checked;

  syncBtnState('btn-dsetting-chat', chatOn, '채팅 후원');
  syncBtnState('btn-dsetting-video', vidOn, '영상 후원');
  syncBtnState('btn-dsetting-mission', misOn, '미션 후원');
  syncBtnState('btn-dsetting-sub', subOn, '구독 알림');
}

function syncBtnState(btnId, enabled, name) {
  const btn = document.getElementById(btnId);
  if (!btn) return;
  btn.disabled = !enabled;
  if (enabled) {
    btn.className = "px-2 py-0.5 rounded text-[10.5px] font-semibold flex items-center gap-1 transition-all bg-[#0B0F15] border border-[#00FFA3]/40 text-[#00FFA3] hover:bg-[#161F2E] cursor-pointer";
    btn.title = `${name} 세부 설정 열기`;
  } else {
    btn.className = "px-2 py-0.5 rounded text-[10.5px] font-medium flex items-center gap-1 transition-all bg-[#0B0F15]/40 border border-[#1E2738] text-slate-500 opacity-40 cursor-not-allowed";
    btn.title = `${name} 활성화 시 설정 가능`;
  }
}

async function toggleSubAlertFromMain(checked) {
  const el = document.getElementById('toggle-sub-alert');

  if (checked) {
    const isQualified = await checkChannelMonetization();
    if (!isQualified) {
      if (el) el.checked = false;
      const existingToasts = document.querySelectorAll('#toast-container .toast');
      existingToasts.forEach(t => t.remove());
      showToast('구독 설정 잠김', 'warn', '현재 채널은 루키(ROOKIE) 회원입니다. 구독 알림 설정은 프로(PRO) 또는 파트너 등급 전용 기능입니다.');
      updateDonationDetailButtonsState();
      return;
    }
  }

  await toggleSubAlertActive(checked);
  updateDonationDetailButtonsState();
}

async function openDonationModal(type) {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('후원 설정')) return;
  if (type === 'sub') {
    const isQualified = await checkChannelMonetization();
    if (!isQualified) {
      showToast('구독 기능 잠김', 'warn', '현재 채널은 루키(ROOKIE) 회원입니다. 구독 설정은 프로 또는 파트너 등급 전용 기능입니다.');
      return;
    }
  }
  const modal = document.getElementById(`modal-dsetting-${type}`);
  if (!modal) return;
  modal.classList.remove('hidden');
  await loadAllDonationSettings();
}

function closeDonationModal(type) {
  const modal = document.getElementById(`modal-dsetting-${type}`);
  if (modal) modal.classList.add('hidden');
  fetchDonationsAndChatMode(true);
}

async function loadAllDonationSettings() {
  if (!userChannelId) return;
  try {
    const res = await apiFetch('/chzzkdock/donations/details');
    if (!res.ok) return;
    const details = await res.json();

    if (details.chat) {
      const d = details.chat;
      if (typeof d.donationActive === 'boolean') {
        currentDonationState.chatDonationActive = d.donationActive;
        const elChatToggle = document.getElementById('toggle-chat-donation');
        if (elChatToggle) elChatToggle.checked = d.donationActive;
      }
      const chatMin = document.getElementById('dsetting-chat-min-amount');
      const chatExposure = document.getElementById('dsetting-chat-exposure');
      const chatEnvelope = document.getElementById('dsetting-chat-envelope');
      const minVal = d.minCurrencyPayAmount || d.chatMinCurrencyPayAmount || 1000;
      if (chatMin) chatMin.value = Number(minVal).toLocaleString();
      if (chatExposure) chatExposure.checked = Number(d.exposureDonationAmount) > 0;
      if (chatEnvelope) chatEnvelope.checked = d.useLargeDonationEnvelope !== false && (d.useLargeDonationEnvelope === true || d.allowHighDonationEnvelope === true);
    }

    if (details.video) {
      const d = details.video;
      if (typeof d.donationActive === 'boolean') {
        currentDonationState.videoDonationActive = d.donationActive;
        const elVidToggle = document.getElementById('toggle-video-donation');
        if (elVidToggle) elVidToggle.checked = d.donationActive;
      }
      const vidMin = document.getElementById('dsetting-video-min-cheese');
      const vidSec = document.getElementById('dsetting-video-sec-cheese');
      const vidDur = document.getElementById('dsetting-video-max-duration');
      const vidAuto = document.getElementById('dsetting-video-autoplay');
      const vidTitle = document.getElementById('dsetting-video-title');
      const vidSpam = document.getElementById('dsetting-video-spam');

      if (vidMin) vidMin.value = Number(d.minCurrencyPayAmount || 1000).toLocaleString();
      if (vidSec) vidSec.value = d.payAmountPerSecond || 30;
      if (vidDur) vidDur.value = d.maxDurationLength || 60;
      if (vidAuto) vidAuto.checked = !!d.isAutoPlay;
      if (vidTitle) vidTitle.checked = !!d.isTitleExpose;
      if (vidSpam) {
        vidSpam.value = d.spamFilterLevel || 0;
        updateSpamFilterUI(vidSpam.value);
      }
    }

    if (details.mission) {
      const d = details.mission;
      if (typeof d.donationActive === 'boolean') {
        currentDonationState.missionDonationActive = d.donationActive;
        const elMisToggle = document.getElementById('toggle-mission-donation');
        if (elMisToggle) elMisToggle.checked = d.donationActive;
      }
      const misActive = document.getElementById('dsetting-mission-active');
      const misMin = document.getElementById('dsetting-mission-min-amount');
      const misFail = document.getElementById('dsetting-mission-fail-rate');
      const misExposure = document.getElementById('dsetting-mission-exposure');

      if (misActive) misActive.checked = d.donationActive !== false;
      if (misMin) misMin.value = Number(d.minCurrencyPayAmount || 1000).toLocaleString();
      if (misFail) misFail.value = d.failCheeringRate || 10;
      if (misExposure) misExposure.checked = d.exposureAmount !== false;
    }

    if (details.subscription) {
      const d = details.subscription;
      cachedSubscriptionSetting = d;
      if (typeof d.alertActive === 'boolean') {
        const elSubToggle = document.getElementById('toggle-sub-alert');
        if (elSubToggle) elSubToggle.checked = d.alertActive;
      }
      const subAlertVol = document.getElementById('dsetting-sub-alert-vol');
      const subAlertVolLbl = document.getElementById('dsetting-sub-alert-vol-label');
      const subTtsActive = document.getElementById('dsetting-sub-tts-active');
      const subTtsVol = document.getElementById('dsetting-sub-tts-vol');
      const subTtsVolLbl = document.getElementById('dsetting-sub-tts-vol-label');

      if (subAlertVol) {
        subAlertVol.value = d.alertVolume != null ? d.alertVolume : 70;
        if (subAlertVolLbl) subAlertVolLbl.textContent = subAlertVol.value + '%';
      }
      if (subTtsActive) subTtsActive.checked = !!d.ttsActive;
      if (subTtsVol) {
        subTtsVol.value = d.ttsVolume != null ? d.ttsVolume : 70;
        if (subTtsVolLbl) subTtsVolLbl.textContent = subTtsVol.value + '%';
      }
    }
    updateDonationDetailButtonsState();
  } catch (e) {
    console.error('loadAllDonationSettings failed:', e);
  }
}

async function saveChatDonationCondition() {
  if (!userChannelId) return;
  const minEl = document.getElementById('dsetting-chat-min-amount');
  const minVal = parseFormattedAmount(minEl?.value) || 1000;
  const isChatActive = document.getElementById('toggle-chat-donation') ? document.getElementById('toggle-chat-donation').checked : (currentDonationState.chatDonationActive !== false);

  try {
    const res = await apiFetch('/chzzkdock/donations/details/chat', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        donationActive: isChatActive,
        minCurrencyPayAmount: minVal
      })
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    if (!res.ok) {
      const errData = await res.json().catch(() => ({}));
      showToast('후원 조건 설정 저장 실패', 'error', errData.message || `서버 응답 오류 (HTTP ${res.status})`);
      return;
    }
    showToast('💾 후원 조건 설정 저장', 'success', `최소 후원 금액: ${minVal.toLocaleString()} 치즈`);
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function toggleChatExposure(checked) {
  if (!userChannelId) return;
  const el = document.getElementById('dsetting-chat-exposure');
  try {
    const res = await apiFetch('/chzzkdock/donations/details/chat', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ exposureDonationAmount: checked ? 1000 : 0 })
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    if (!res.ok) {
      if (el) el.checked = !checked;
      const errData = await res.json().catch(() => ({}));
      showToast('후원 금액 표시 설정 실패', 'error', errData.message || `서버 응답 오류 (HTTP ${res.status})`);
      return;
    }
    showToast('후원 금액 표시 설정', 'info', checked ? '채팅창에 후원 금액을 표시합니다.' : '채팅창 후원 금액을 숨깁니다.');
  } catch (e) {
    if (el) el.checked = !checked;
    showToast('오류 발생', 'error', e.message);
  }
}

async function toggleEnvelope(checked) {
  if (!userChannelId) return;
  const el = document.getElementById('dsetting-chat-envelope');
  const isChatActive = document.getElementById('toggle-chat-donation') ? document.getElementById('toggle-chat-donation').checked : (currentDonationState.chatDonationActive !== false);
  try {
    const res = await apiFetch('/chzzkdock/donations/details/chat', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        donationActive: isChatActive,
        useLargeDonationEnvelope: checked
      })
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    if (!res.ok) {
      if (el) el.checked = !checked;
      const errData = await res.json().catch(() => ({}));
      showToast('고액 후원 봉투 설정 실패', 'error', errData.message || `서버 응답 오류 (HTTP ${res.status})`);
      return;
    }
    showToast('고액 후원 봉투', 'info', checked ? '고액 후원 봉투 기능을 허용합니다.' : '고액 후원 봉투 기능을 차단합니다.');
  } catch (e) {
    if (el) el.checked = !checked;
    showToast('오류 발생', 'error', e.message);
  }
}

async function saveVideoDonationSetting() {
  if (!userChannelId) return;
  const minCheese = parseFormattedAmount(document.getElementById('dsetting-video-min-cheese')?.value) || 1000;
  const secCheese = Number(document.getElementById('dsetting-video-sec-cheese')?.value) || 30;
  const maxDur = Number(document.getElementById('dsetting-video-max-duration')?.value) || 60;
  const autoPlay = !!document.getElementById('dsetting-video-autoplay')?.checked;
  const titleExp = !!document.getElementById('dsetting-video-title')?.checked;
  const spamLevel = Number(document.getElementById('dsetting-video-spam')?.value) || 0;
  const isVideoActive = document.getElementById('toggle-video-donation') ? document.getElementById('toggle-video-donation').checked : (currentDonationState.videoDonationActive !== false);

  try {
    const res = await apiFetch('/chzzkdock/donations/details/video', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        donationActive: isVideoActive,
        minCurrencyPayAmount: minCheese,
        payAmountPerSecond: secCheese,
        maxDurationLength: maxDur,
        isAutoPlay: autoPlay,
        isTitleExpose: titleExp,
        spamFilterLevel: spamLevel
      })
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    if (!res.ok) {
      const errData = await res.json().catch(() => ({}));
      showToast('영상 후원 설정 저장 실패', 'error', errData.message || `서버 응답 오류 (HTTP ${res.status})`);
      return;
    }
    showToast('💾 영상 후원 설정 저장 완료', 'success', '영상 후원 규칙이 성공적으로 반영되었습니다.');
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function saveMissionDonationSetting() {
  if (!userChannelId) return;
  const minAmount = parseFormattedAmount(document.getElementById('dsetting-mission-min-amount')?.value) || 1000;
  let failRate = Number(document.getElementById('dsetting-mission-fail-rate')?.value);
  if (isNaN(failRate) || failRate < 10) failRate = 10;
  if (failRate > 90) failRate = 90;
  const exposure = !!document.getElementById('dsetting-mission-exposure')?.checked;
  const misActiveEl = document.getElementById('dsetting-mission-active');
  const isMissionActive = misActiveEl ? !!misActiveEl.checked : (document.getElementById('toggle-mission-donation') ? document.getElementById('toggle-mission-donation').checked : (currentDonationState.missionDonationActive !== false));

  try {
    const res = await apiFetch('/chzzkdock/donations/details/mission', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        donationActive: isMissionActive,
        minCurrencyPayAmount: minAmount,
        failCheeringRate: failRate,
        exposureAmount: exposure
      })
    });

    if (!res.ok) {
      const errData = await res.json().catch(() => ({}));
      throw new Error(errData.message || `치지직 서버 응답 오류 (HTTP ${res.status})`);
    }

    const mainToggle = document.getElementById('toggle-mission-donation');
    if (mainToggle) mainToggle.checked = isMissionActive;
    currentDonationState.missionDonationActive = isMissionActive;
    updateDonationDetailButtonsState();

    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    showToast('💾 미션 후원 설정 저장 완료', 'success', '미션 후원 규칙이 성공적으로 반영되었습니다.');
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function toggleSubAlertActive(checked) {
  try {
    const res = await apiFetch('/chzzkdock/donations/subscription', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: checked })
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());

    if (res.ok) {
      const don = await res.json();
      if (cachedSubscriptionSetting) cachedSubscriptionSetting.alertActive = don.subscription;
      const mainEl = document.getElementById('toggle-sub-alert');
      if (mainEl) mainEl.checked = !!don.subscription;
      updateDonationDetailButtonsState();
      showToast(don.subscription ? '⭐ 구독 알림 켜짐' : '⭐ 구독 알림 꺼짐', 'info', `구독 알림이 ${don.subscription ? '활성화' : '비활성화'}되었습니다.`);
    } else {
      const data = await res.json().catch(() => ({}));
      showToast('구독 알림 설정 실패', 'error', data.message || '상태 변경 실패');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function saveSubscriptionSetting() {
  if (!userChannelId) return;
  const mainSubToggle = document.getElementById('toggle-sub-alert');
  const alertActive = mainSubToggle ? !!mainSubToggle.checked : (cachedSubscriptionSetting?.alertActive !== false);
  const rawAlertVol = document.getElementById('dsetting-sub-alert-vol')?.value;
  const parsedAlertVol = Number(rawAlertVol);
  const alertVol = (rawAlertVol !== undefined && rawAlertVol !== '' && Number.isFinite(parsedAlertVol)) ? parsedAlertVol : 70;
  const ttsActive = !!document.getElementById('dsetting-sub-tts-active')?.checked;
  const rawTtsVol = document.getElementById('dsetting-sub-tts-vol')?.value;
  const parsedTtsVol = Number(rawTtsVol);
  const ttsVol = (rawTtsVol !== undefined && rawTtsVol !== '' && Number.isFinite(parsedTtsVol)) ? parsedTtsVol : 70;

  const payload = Object.assign({}, cachedSubscriptionSetting || {}, {
    alertActive,
    alertVolume: alertVol,
    ttsActive,
    ttsVolume: ttsVol
  });

  try {
    const res = await apiFetch('/chzzkdock/donations/details/subscription', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());

    if (res.ok) {
      cachedSubscriptionSetting = payload;
      showToast('💾 구독 알림 설정 저장 완료', 'success', '구독 알림 및 TTS 음성 볼륨 규칙이 성공적으로 반영되었습니다.');
    } else {
      const data = await res.json().catch(() => ({}));
      showToast('구독 알림 설정 저장 실패', 'error', data.message || '설정 저장 실패');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function fetchDonationsAndChatMode(silent = false) {
  try {
    let loaded = false;

    const [chatRes, donRes] = await Promise.all([
      chzzkApi.getChatMode(),
      chzzkApi.getDonations()
    ]);

    if (donRes.ok) {
      const d = await donRes.json();
      currentDonationState = {
        chatDonationActive: !!d.chat,
        videoDonationActive: !!d.video,
        missionDonationActive: !!d.mission
      };
      const elChat = document.getElementById('toggle-chat-donation');
      const elVid = document.getElementById('toggle-video-donation');
      const elMis = document.getElementById('toggle-mission-donation');
      const elSub = document.getElementById('toggle-sub-alert');
      if (elChat) elChat.checked = currentDonationState.chatDonationActive;
      if (elVid) elVid.checked = currentDonationState.videoDonationActive;
      if (elMis) elMis.checked = currentDonationState.missionDonationActive;
      if (elSub) elSub.checked = !!d.subscription;
      if (cachedSubscriptionSetting) cachedSubscriptionSetting.alertActive = !!d.subscription;
      loaded = true;
    }

    if (chatRes.ok) {
      const chat = await chatRes.json();
      applyChatModeUI(chat);
      loaded = true;
    }

    if (loaded) {
      donationsLoaded = true;
      if (!silent) showToast('후원 및 실시간 채팅 설정을 불러왔습니다', 'success');
    } else {
      if (!silent) showToast('후원/채팅 설정 불러오기 실패', 'error', '네이버 로그인 쿠키 상태를 확인해 주세요.');
    }
    updateDonationDetailButtonsState();
  } catch (e) {
    if (!silent) showToast('통신 오류', 'error', e.message);
  }
}

async function updateDonationSetting(type, silent = false) {
  const MAP = {
    CHAT: { id: 'toggle-chat-donation', key: 'chatDonationActive', target: 'chat' },
    VIDEO: { id: 'toggle-video-donation', key: 'videoDonationActive', target: 'video' },
    MISSION: { id: 'toggle-mission-donation', key: 'missionDonationActive', target: 'mission' }
  };

  const cfg = MAP[type];
  if (!cfg) return;

  const el = document.getElementById(cfg.id);
  const targetActive = el ? el.checked : false;

  try {
    const res = await chzzkApi.toggleDonation(cfg.target, targetActive);

    if (res.ok) {
      const don = await res.json();
      currentDonationState[cfg.key] = !!don[cfg.target];
      if (el) el.checked = currentDonationState[cfg.key];
      updateDonationDetailButtonsState();
      if (!silent) showToast('🎉 후원 옵션 변경 성공!', 'success');
    } else {
      if (el) el.checked = currentDonationState[cfg.key];
      updateDonationDetailButtonsState();
      if (!silent) showToast('후원 옵션 변경 실패', 'error', '쿠키 로그인 상태를 확인하세요.');
    }
  } catch (e) {
    if (el) el.checked = currentDonationState[cfg.key];
    updateDonationDetailButtonsState();
    if (!silent) showToast('통신 오류', 'error', e.message);
  }
}

function formatSlowSec(sec) {
  if (sec >= 60) return `${sec / 60}분`;
  return `${sec}초`;
}

async function updateChatEmojiMode(enabled) {
  const el = document.getElementById('toggle-emoji-mode');
  try {
    const res = await chzzkApi.setEmojiMode(enabled);
    if (res.ok) {
      const chat = await res.json();
      currentChatModeState.chatEmojiMode = !!chat.emojiOnly;
      if (el) el.checked = currentChatModeState.chatEmojiMode;
      showToast(currentChatModeState.chatEmojiMode ? '😀 이모티콘 전용 모드 ON' : '💬 일반 채팅 모드로 전환', 'success');
    } else {
      if (el) el.checked = currentChatModeState.chatEmojiMode;
      showToast('이모티콘 모드 변경 실패', 'error', '쿠키 로그인 상태를 확인하세요.');
    }
  } catch (e) {
    if (el) el.checked = currentChatModeState.chatEmojiMode;
    showToast('통신 오류', 'error', e.message);
  }
}

async function updateChatSlowMode(sec) {
  const el = document.getElementById('select-slow-mode');
  const elStatus = document.getElementById('slow-mode-status');
  try {
    const res = await chzzkApi.setSlowMode(sec);
    if (res.ok) {
      const chat = await res.json();
      currentChatModeState.chatSlowModeSec = Number(chat.slowSec) || 0;
      if (el) el.value = String(currentChatModeState.chatSlowModeSec);
      if (elStatus) {
        if (currentChatModeState.chatSlowModeSec > 0) {
          elStatus.textContent = formatSlowSec(currentChatModeState.chatSlowModeSec);
          elStatus.className = 'text-[10px] text-[#00FFA3] font-bold font-mono';
          showToast(`⏱️ 저속 모드 (${formatSlowSec(currentChatModeState.chatSlowModeSec)}) 설정 완료`, 'success');
        } else {
          elStatus.textContent = 'OFF';
          elStatus.className = 'text-[10px] text-slate-400 font-mono font-semibold';
          showToast('⏱️ 저속 모드 해제 완료', 'success');
        }
      }
    } else {
      if (el) el.value = String(currentChatModeState.chatSlowModeSec);
      showToast('저속 모드 변경 실패', 'error', '쿠키 로그인 상태를 확인하세요.');
    }
  } catch (e) {
    if (el) el.value = String(currentChatModeState.chatSlowModeSec);
    showToast('통신 오류', 'error', e.message);
  }
}

function switchStreamOptionTab(tab) {
  const tabs = ['replay', 'stream'];
  tabs.forEach(t => {
    const btn = document.getElementById(`tab-btn-${t}`);
    const pane = document.getElementById(`tab-pane-${t}`);
    const isActive = (t === tab);
    btn?.classList.toggle('active', isActive);
    if (pane) {
      if (isActive) {
        pane.classList.remove('hidden');
        pane.classList.remove('tab-pane-animate');
        void pane.offsetWidth;
        pane.classList.add('tab-pane-animate');
      } else {
        pane.classList.add('hidden');
      }
    }
  });
  moveLiquidPill('stream-option-liquid-tabs', document.getElementById(`tab-btn-${tab}`), true);
  checkBroadcastStateDirty();
}

async function applyChatRuleInstant() {
  const group = getSelectedChatGroup();
  const minMin = group === 'FOLLOWER' ? (parseInt(document.getElementById('min-follower-minute')?.value) || 0) : 0;
  const subEx = !!document.getElementById('toggle-sub-exception')?.checked;

  const payloadChat = {
    chatAvailableGroup: group,
    minFollowerMinute: minMin,
    allowSubscriberInFollowerMode: subEx
  };

  try {
    const res = await apiFetch('/chzzkdock/chat-rule', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payloadChat)
    });
    if (res.ok) {
      const groupNames = { ALL: '모두', FOLLOWER: '팔로워 전용', MANAGER: '운영자 전용' };
      let msg = `채팅 권한: ${groupNames[group] || group}`;
      if (group === 'FOLLOWER') {
        msg += minMin > 0 ? ` (${minMin}분 이상)` : ' (제한 없음)';
      }
      showToast(msg, 'success');
    } else {
      let chatErrorMsg = '채팅 권한 변경에 실패했습니다.';
      try {
        const errChat = await res.json();
        if (errChat && errChat.message) chatErrorMsg = errChat.message;
      } catch (e) {}
      showToast('채팅 권한 변경 실패', 'error', chatErrorMsg);
    }
  } catch (err) {
    showToast('채팅 권한 변경 오류', 'error', err.message);
  }
}

function onChatGroupChange(val, skipApi = false) {
  const opts = document.getElementById('follower-options');
  if (opts) {
    opts.style.display = val === 'FOLLOWER' ? 'flex' : 'none';
  }
  if (!skipApi) {
    applyChatRuleInstant();
  }
}

function getSelectedChatGroup() {
  const selected = document.querySelector('input[name="chatAvailableGroup"]:checked');
  return selected ? selected.value : 'ALL';
}

function setSelectedChatGroup(val) {
  const el = document.querySelector(`input[name="chatAvailableGroup"][value="${val}"]`);
  if (el) {
    el.checked = true;
  } else {
    const defaultEl = document.querySelector('input[name="chatAvailableGroup"][value="ALL"]');
    if (defaultEl) defaultEl.checked = true;
  }
  onChatGroupChange(val, true);
}

// ===== 방송 URL, 스트림 키 & 수익 요약 =====
function switchUrlsPanelTab(tab, animate = true) {
  const layers = tab === 'layers';
  if (!layers && !checkLoginGuard('방송 URL 및 스트림 키 확인')) return;
  const active = document.getElementById(layers ? 'layers-tab' : 'urls-tab');
  for (const button of active.parentElement.querySelectorAll('[role="tab"]')) {
    button.classList.toggle('active', button === active);
    button.setAttribute('aria-selected', String(button === active));
  }
  document.getElementById('tab-content-urls').hidden = layers;
  const layersPanel = document.getElementById('tab-content-layers');
  layersPanel.hidden = !layers;
  document.getElementById('urls-panel-refresh').hidden = layers;
  moveLiquidPill('urls-liquid-tabs', active, animate);
  if (layers) layersPanel.querySelector('.overlay-manager').dispatchEvent(new Event('overlay-activated'));
  else fetchStreamingInfo();
}

function toggleUrlsPanel() {
  closeAllPopovers();
  const panel = document.getElementById('urls-panel');
  const settingsPanel = document.getElementById('settings-panel');
  const remotePanel = document.getElementById('remote-panel');
  if (settingsPanel && settingsPanel.classList.contains('open')) settingsPanel.classList.remove('open');
  if (remotePanel && remotePanel.classList.contains('open')) {
    remotePanel.classList.remove('open');
    closeRemoteVolumeDrawer();
    stopRemotePolling();
  }
  if (panel) {
    panel.classList.toggle('open');
    const elMainDock = document.getElementById('field-main-dock-url');
    if (elMainDock) elMainDock.value = `${window.location.origin}/`;
    const elMini = document.getElementById('field-mini-dock-url');
    if (elMini) elMini.value = `${window.location.origin}/mini`;
    const elStats = document.getElementById('field-stats-dock-url');
    if (elStats) elStats.value = `${window.location.origin}/stats`;
    if (panel.classList.contains('open')) {
      const hasLogin = userChannelId || (serverConfig.nid_aut && serverConfig.nid_ses);
      const tab = !hasLogin || document.getElementById('layers-tab').classList.contains('active') ? 'layers' : 'urls';
      switchUrlsPanelTab(tab, false);
    }
  }
  if (typeof updatePanelScrollLock === 'function') updatePanelScrollLock();
}

async function reissueStreamKey() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('스트림 키 재발급')) return;
  if (!userChannelId) await resolveChannelId();
  if (!userChannelId) {
    showToast('로그인 필요', 'warn', '채널 정보를 찾을 수 없습니다.');
    return;
  }

  const ok = await showConfirmModal({
    title: '스트림 키 재발급 안내',
    message: '방송을 시작하려면 새로 발급된 스트림 키로 스트리밍 소프트웨어에 업데이트 해야 합니다.',
    submessage: '스트림 키를 재발급하려면 재발급을 클릭해주세요.',
    confirmText: '재발급',
    cancelText: '취소',
    danger: true,
    icon: '🔑'
  });

  if (!ok) return;

  try {
    const res = await apiFetch('/chzzkdock/streamkey', { method: 'PUT' });
    if (res.ok) {
      const data = await res.json();
      const newKey = data?.streamKey;
      const elKey = document.getElementById('field-stream-key');
      if (elKey && newKey) {
        elKey.value = newKey;
      }
      showToast('🔑 스트림 키 재발급 완료!', 'success', 'OBS 방송 설정에 새 스트림 키를 업데이트하세요.');
    } else {
      showToast('재발급 실패', 'error', '스트림 키를 재발급하지 못했습니다.');
    }
  } catch (e) {
    showToast('통신 오류', 'error', e.message);
  }
}

function toggleFieldMask(fieldId, btn) {
  const input = document.getElementById(fieldId);
  if (!input) return;
  if (input.type === 'password') {
    input.type = 'text';
    if (btn) btn.textContent = '🙈';
  } else {
    input.type = 'password';
    if (btn) btn.textContent = '👁️';
  }
}

// [보류 / v0.6.4 이후 검토]: navigator.clipboard 실패 시 execCommand('copy') 이중 폴백 처리.
// 차기 버전에서 공통 클립보드 헬퍼로 통합하거나 레거시 execCommand 완전 삭제 검토 필요.
function copyFieldValue(fieldId, label) {
  const input = document.getElementById(fieldId);
  if (!input || !input.value) {
    showToast('복사 실패', 'warn', '복사할 값이 없습니다.');
    return;
  }
  navigator.clipboard.writeText(input.value).then(() => {
    showToast(`✅ ${label} 복사 완료!`, 'success');
  }).catch(() => {
    input.select();
    document.execCommand('copy');
    showToast(`✅ ${label} 복사 완료!`, 'success');
  });
}

async function fetchStreamingInfo() {
  if (!userChannelId) await resolveChannelId();
  if (!userChannelId) {
    showToast('로그인 필요', 'warn', '치지직 로그인 설정을 먼저 완료해 주세요.');
    return;
  }

  try {
    const keyPromise = apiFetch('/chzzkdock/streamkey').then(r => r.ok ? r.json() : null).catch(() => null);
    const urlsPromise = apiFetch('/chzzkdock/streaming-urls').then(r => r.ok ? r.json() : null).catch(() => null);

    const [keyData, urlsData] = await Promise.all([keyPromise, urlsPromise]);

    if (keyData) {
      const elKey = document.getElementById('field-stream-key');
      if (elKey) elKey.value = keyData.streamKey || '';
    }

    if (urlsData) {
      const elChat = document.getElementById('field-chat-url');
      const elDon = document.getElementById('field-alert-donation');
      const elVid = document.getElementById('field-alert-video');
      const elMis = document.getElementById('field-alert-mission');
      const elPartyAmount = document.getElementById('field-alert-party-amount');
      const elPartyTicker = document.getElementById('field-alert-party-ticker');
      const elGift = document.getElementById('field-alert-gift');
      const elShop = document.getElementById('field-alert-shop');

      if (elChat) elChat.value = urlsData.chatUrl || '';
      if (elDon) elDon.value = urlsData.donationUrl || '';
      if (elVid) elVid.value = urlsData.videoUrl || '';
      if (elMis) elMis.value = urlsData.missionUrl || '';
      if (elPartyAmount) elPartyAmount.value = urlsData.partyAmountUrl || '';
      if (elPartyTicker) elPartyTicker.value = urlsData.partyTickerUrl || '';
      if (elGift) elGift.value = urlsData.giftUrl || '';
      if (elShop) elShop.value = urlsData.shopUrl || '';
    }

    const elMainDock = document.getElementById('field-main-dock-url');
    if (elMainDock) elMainDock.value = `${window.location.origin}/`;

    const elMini = document.getElementById('field-mini-dock-url');
    if (elMini) elMini.value = `${window.location.origin}/mini`;

    const elStats = document.getElementById('field-stats-dock-url');
    if (elStats) elStats.value = `${window.location.origin}/stats`;

    streamingInfoLoaded = true;
    return true;
  } catch (e) {
    showToast('스트리밍 정보 조회 실패', 'error', e.message);
  }
  return false;
}

function openRevenueNoticeModal() {
  showNoticeModal({
    icon: '🚧',
    title: '기능 준비 중 안내',
    message: '후원 수익 요약 기능은 현재 개발 및 연동 테스트 진행 중인 미완성 기능입니다.',
    submessage: 'ℹ️ 정확한 후원 수익 집계 및 정산 내역은 치지직 스튜디오 공식 웹사이트의 정산 관리 메뉴에서 확인해 주시기 바랍니다.'
  });
}

function getFormattedYM(date) {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, '0');
  return `${y}${m}`;
}

function updateRevenueDisplayDate() {
  const disp = document.getElementById('revenue-month-display');
  if (disp) {
    const y = currentRevenueDate.getFullYear();
    const m = currentRevenueDate.getMonth() + 1;
    disp.textContent = `${y}년 ${m}월`;
  }
}

function changeRevenueMonth(delta) {
  currentRevenueDate.setMonth(currentRevenueDate.getMonth() + delta);
  updateRevenueDisplayDate();
  fetchRevenueSummary();
}

function toggleRevenuePrivacy() {
  isRevenuePrivate = !isRevenuePrivate;
  const elTotal = document.getElementById('revenue-total-amount');
  const elSettlement = document.getElementById('revenue-settlement-amount');
  const btn = document.getElementById('btn-toggle-privacy');

  if (isRevenuePrivate) {
    if (elTotal) elTotal.classList.add('blur-[5px]');
    if (elSettlement) elSettlement.classList.add('blur-[5px]');
    if (btn) btn.textContent = '👁️ 금액 표시';
  } else {
    if (elTotal) elTotal.classList.remove('blur-[5px]');
    if (elSettlement) elSettlement.classList.remove('blur-[5px]');
    if (btn) btn.textContent = '🙈 금액 숨김';
  }
}

async function fetchRevenueSummary() {
  if (!userChannelId) await resolveChannelId();
  if (!userChannelId) return;

  const ym = getFormattedYM(currentRevenueDate);
  updateRevenueDisplayDate();

  const spinner = document.getElementById('revenue-loading-spinner');
  if (spinner) spinner.style.display = 'inline-block';

  if (revenueCache[ym]) {
    renderRevenueData(revenueCache[ym]);
    if (spinner) spinner.style.display = 'none';
    return;
  }

  try {
    const res = await apiFetch(`/chzzkdock/revenue/summary?yearMonth=${ym}`);
    if (res.ok) {
      const data = await res.json();
      const content = data.content || data;
      revenueCache[ym] = content;
      renderRevenueData(content);
    } else if (res.status === 403) {
      renderRevenueError('수익 창출 계약(프로/파트너) 이후 확인 가능합니다.');
    } else {
      renderRevenueError('해당 월 수익 데이터를 불러올 수 없습니다.');
    }
  } catch (e) {
    renderRevenueError(e.message);
  } finally {
    if (spinner) spinner.style.display = 'none';
  }
}

function renderRevenueData(content) {
  const summaryList = content?.summaryList || [];
  const item = summaryList.length > 0 ? summaryList[0] : null;

  const elTotal = document.getElementById('revenue-total-amount');
  const elSettlement = document.getElementById('revenue-settlement-amount');
  const elCount = document.getElementById('revenue-donation-count');
  const elStatus = document.getElementById('revenue-settlement-status');
  const elPaidDate = document.getElementById('revenue-paid-date');

  if (!item) {
    if (elTotal) elTotal.textContent = '₩ 0';
    if (elSettlement) elSettlement.textContent = '₩ 0';
    if (elCount) elCount.textContent = '0건';
    if (elStatus) {
      elStatus.textContent = '내역 없음';
      elStatus.className = 'px-2 py-0.5 rounded text-[10px] font-bold bg-[#161F2E] text-slate-400';
    }
    if (elPaidDate) elPaidDate.textContent = '-';
    return;
  }

  const total = (item.totalRevenue || 0).toLocaleString();
  const settlement = (item.totalSettlement || 0).toLocaleString();
  const count = (item.donationCount || 0).toLocaleString();

  if (elTotal) elTotal.textContent = `₩ ${total}`;
  if (elSettlement) elSettlement.textContent = `₩ ${settlement}`;
  if (elCount) elCount.textContent = `${count}건`;

  const statusMap = {
    NONE: { label: '미정산 (NONE)', class: 'bg-[#161F2E] text-slate-400' },
    WAITING: { label: '정산 대기 중', class: 'bg-amber-950/80 text-amber-300 border border-amber-800/50' },
    IN_PROGRESS: { label: '정산 진행 중', class: 'bg-blue-950/80 text-blue-300 border border-blue-800/50' },
    SETTLED: { label: '정산 완료', class: 'bg-emerald-950/80 text-[#00FFA3] border border-emerald-800/50' }
  };

  const st = statusMap[item.settlementStatus] || { label: item.settlementStatus || 'NONE', class: 'bg-[#161F2E] text-slate-400' };
  if (elStatus) {
    elStatus.textContent = st.label;
    elStatus.className = `px-2 py-0.5 rounded text-[10px] font-bold ${st.class}`;
  }

  if (elPaidDate) {
    if (Array.isArray(item.paidSettlementDate)) {
      elPaidDate.textContent = `${item.paidSettlementDate[0]}년 ${item.paidSettlementDate[1]}월`;
    } else if (item.paidSettlementDate) {
      elPaidDate.textContent = String(item.paidSettlementDate);
    } else {
      elPaidDate.textContent = '-';
    }
  }
}

function renderRevenueError(msg) {
  const elTotal = document.getElementById('revenue-total-amount');
  const elSettlement = document.getElementById('revenue-settlement-amount');
  const elStatus = document.getElementById('revenue-settlement-status');
  if (elTotal) elTotal.textContent = '-';
  if (elSettlement) elSettlement.textContent = '-';
  if (elStatus) {
    elStatus.textContent = msg;
    elStatus.className = 'px-2 py-0.5 rounded text-[10px] font-medium bg-red-950/80 text-red-300 border border-red-800/50';
  }
}

// 초기 로드 시 로컬 독 URL 사전 바인딩 및 물방울 탭 알약 초기화
function syncAllVisibleLiquidPills() {
  [
    'stream-option-liquid-tabs',
    'donations-liquid-tabs',
    'urls-liquid-tabs',
    'remote-liquid-tabs',
    'mission-sort-group',
    'settings-liquid-nav'
  ].forEach(id => {
    const active = document.querySelector(`#${id} .liquid-tab-btn.active, #${id} .settings-nav-btn.active`);
    if (active) moveLiquidPill(id, active, false);
  });
}

function initRemoteVolumeHandleDrag() {
  const panel = document.getElementById('remote-panel');
  const handle = document.getElementById('remote-vol-handle');
  const drawer = document.getElementById('remote-vol-drawer');
  if (!panel || !handle || !drawer) return;

  function applySide(side, topPct) {
    const cleanSide = (side === 'left') ? 'left' : 'right';
    handle.dataset.side = cleanSide;
    drawer.dataset.side = cleanSide;
    if (typeof topPct === 'number' && isFinite(topPct)) {
      handle.style.top = `${Math.max(12, Math.min(82, topPct))}%`;
    }
  }

  try {
    const saved = JSON.parse(localStorage.getItem('chzzk_remote_vol_pos') || 'null');
    if (saved) applySide(saved.side, saved.topPct);
  } catch (_) {}

  let dragging = false;
  let startX = 0;
  let startY = 0;
  let moved = false;

  handle.addEventListener('mousedown', (e) => {
    if (e.button !== 0) return;
    dragging = true;
    moved = false;
    startX = e.clientX;
    startY = e.clientY;
    handle.classList.add('is-dragging');
    e.preventDefault();
  });

  document.addEventListener('mousemove', (e) => {
    if (!dragging) return;
    if (Math.abs(e.clientX - startX) > 4 || Math.abs(e.clientY - startY) > 4) {
      moved = true;
    }
    if (!moved) return;

    const rect = panel.getBoundingClientRect();
    const minY = 52;
    const maxY = Math.max(minY + 10, panel.clientHeight - handle.offsetHeight - 58);
    const clampedPx = Math.max(minY, Math.min(maxY, e.clientY - rect.top - handle.offsetHeight / 2));
    const topPct = rect.height > 0 ? (clampedPx / rect.height) * 100 : 52;
    const nextSide = (e.clientX - rect.left) < rect.width / 2 ? 'left' : 'right';
    applySide(nextSide, topPct);
  });

  document.addEventListener('mouseup', () => {
    if (!dragging) return;
    dragging = false;
    handle.classList.remove('is-dragging');
    if (!moved) {
      openRemoteVolumeDrawer();
      return;
    }
    const topPct = parseFloat(handle.style.top) || 52;
    try {
      localStorage.setItem('chzzk_remote_vol_pos', JSON.stringify({
        side: handle.dataset.side || 'right',
        topPct: Math.round(topPct * 10) / 10
      }));
    } catch (_) {}
  });
}

document.addEventListener('DOMContentLoaded', () => {
  const elMainDock = document.getElementById('field-main-dock-url');
  if (elMainDock) elMainDock.value = `${window.location.origin}/`;
  const elMini = document.getElementById('field-mini-dock-url');
  if (elMini) elMini.value = `${window.location.origin}/mini`;
  const elStats = document.getElementById('field-stats-dock-url');
  if (elStats) elStats.value = `${window.location.origin}/stats`;
  initRemoteVolumeHandleDrag();
  requestAnimationFrame(syncAllVisibleLiquidPills);
});

window.addEventListener('resize', syncAllVisibleLiquidPills);
if (document.fonts && document.fonts.ready) {
  document.fonts.ready.then(syncAllVisibleLiquidPills).catch(() => {});
}

