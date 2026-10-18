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
var currentPartySummary = null;
var cachedPartyInviteToken = '';
var isCurrentPartyHost = true;
var cachedSubscriptionSetting = null;
var isRemoteAlertPaused = false;
var activeFeedFilter = 'ALL';
var lastPlayedVideo = null;
var currentMissionSort = 'LATEST';
var locallySettledMissions = {};
var userSubscriptionAvailable = false;

var remoteVolumes = {
  tts: 70,
  video: 70,
  chatAlert: 70,
  videoAlert: 70,
  missionAlert: 70
};

var sendVolumeTimer = null;
var saveRemoteVolumeTimer = null;

// ===== 팝오버 공통 제어 (채널, 채팅모드, 후원) =====
function closeAllPopovers() {
  ['channel-popover-menu', 'popover-chatmode', 'popover-donations'].forEach(id => document.getElementById(id)?.classList.add('hidden'));
}

function toggleChatModePopover() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('실시간 채팅 모드')) return;
  const popover = document.getElementById('popover-chatmode');
  const popoverDonations = document.getElementById('popover-donations');
  const popoverChannel = document.getElementById('channel-popover-menu');
  if (popoverDonations) popoverDonations.classList.add('hidden');
  if (popoverChannel) popoverChannel.classList.add('hidden');
  if (!popover) return;
  const isHidden = popover.classList.contains('hidden');
  if (isHidden) {
    popover.classList.remove('hidden');
    fetchChatSettingsOnly();
  } else {
    popover.classList.add('hidden');
  }
}

function toggleDonationsPopover() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('후원 및 수익 요약')) return;
  const popover = document.getElementById('popover-donations');
  const popoverChatMode = document.getElementById('popover-chatmode');
  const popoverChannel = document.getElementById('channel-popover-menu');
  if (popoverChatMode) popoverChatMode.classList.add('hidden');
  if (popoverChannel) popoverChannel.classList.add('hidden');
  if (!popover) return;
  const isHidden = popover.classList.contains('hidden');
  if (isHidden) {
    popover.classList.remove('hidden');
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

  const activePill = 'flex-1 py-1 px-1 rounded-md text-[10.5px] font-bold transition-all flex items-center justify-center gap-1 bg-[#251B38] text-amber-300 border border-amber-400/40 shadow-sm cursor-pointer select-none';
  const inactivePill = 'flex-1 py-1 px-1 rounded-md text-[10.5px] font-medium transition-all flex items-center justify-center gap-1 text-slate-400 hover:text-slate-200 border border-transparent cursor-pointer select-none';

  if (tab === 'settings') {
    if (tabSettings) {
      tabSettings.classList.remove('hidden');
      tabSettings.classList.remove('tab-pane-animate');
      void tabSettings.offsetWidth;
      tabSettings.classList.add('tab-pane-animate');
    }
    if (tabRev) tabRev.classList.add('hidden');
    if (btnSettings) btnSettings.className = activePill;
    if (btnRev) btnRev.className = inactivePill;
  } else {
    if (tabSettings) tabSettings.classList.add('hidden');
    if (tabRev) {
      tabRev.classList.remove('hidden');
      tabRev.classList.remove('tab-pane-animate');
      void tabRev.offsetWidth;
      tabRev.classList.add('tab-pane-animate');
    }
    if (btnSettings) btnSettings.className = inactivePill;
    if (btnRev) btnRev.className = activePill;
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

async function fetchChatSettingsOnly() {
  const icon = document.getElementById('chatmode-refresh-icon');
  if (icon) icon.classList.add('animate-spin');
  try {
    const modeRes = await chzzkApi.getChatMode();
    if (modeRes.ok) {
      const chat = await modeRes.json();
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

async function openChannelInBrowser() {
  if (!userChannelId) {
    showToast('채널 정보 없음', 'warn', '치지직 로그인이 완료된 후 클릭하세요.');
    return;
  }
  const url = `https://chzzk.naver.com/live/${encodeURIComponent(userChannelId)}`;
  await openExternalUrl(url, `${currentChannelName || '내 채널'} 라이브 화면`);
}

async function openChatWebview() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('채팅창 열기')) return;
  try {
    if (!userChannelId) await resolveChannelId();
    const res = await chzzkApi.openChatWebview(userChannelId);
    if (res.ok) {
      const data = await res.json();
      if (data.status === 'started') {
        showToast('💬 채팅창 실행됨', 'success', '독립 채팅창이 열렸습니다.');
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

// ===== 치지직 공식 규격 리모컨 (Official Remote Controller) 로직 =====
function openRemoteNoticeModal() {
  showNoticeModal({
    icon: '🚧',
    title: '기능 준비 중 안내',
    message: 'OBS 독 내부 리모컨(영도/미션/피드 제어)은 치지직 스튜디오 공식 연동을 위해 기능 검증 및 개발 진행 중인 미완성 기능입니다.\n영상 재생·스킵, 볼륨 조절 및 TTS 제어가 완벽히 지원되는 네이버 공식 리모컨 창을 바로 실행할 수 있습니다.',
    submessage: '✨ 치지직 공식 리모컨 독립창 이용 권장',
    buttons: [
      {
        text: '공식 리모컨 열기',
        className: 'px-3.5 py-1.5 rounded-lg bg-[#5C3BFF] hover:bg-[#7B5FFF] text-white text-xs font-bold cursor-pointer transition-colors shadow-md',
        onClick: () => { window.__closeConfirmModal(); openRemoteWebview(); }
      },
      {
        text: '확인',
        className: 'px-3.5 py-1.5 rounded-lg bg-[#161F2E] hover:bg-[#1E2738] text-[#00FFA3] text-xs font-bold cursor-pointer transition-colors',
        onClick: () => window.__closeConfirmModal()
      }
    ]
  });
}

function toggleRemotePanel() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('치지직 공식 리모컨')) return;
  openActualRemotePanel();
}

function openActualRemotePanel() {
  closeAllPopovers();
  const panel = document.getElementById('remote-panel');
  const settingsPanel = document.getElementById('settings-panel');
  const urlsPanel = document.getElementById('urls-panel');
  if (settingsPanel && settingsPanel.classList.contains('open')) settingsPanel.classList.remove('open');
  if (urlsPanel && urlsPanel.classList.contains('open')) urlsPanel.classList.remove('open');
  if (panel) {
    panel.classList.toggle('open');
    if (panel.classList.contains('open')) {
      openRemoteNoticeModal();
      switchRemoteTab('all');
      loadRemoteVolumes();
      fetchNewsFeed();
      refreshPartyStatus();
    }
  }
  if (typeof updatePanelScrollLock === 'function') updatePanelScrollLock();
}

function switchRemoteMode(mode) {
  if (mode === 'official') {
    openRemoteWebview();
  }
}

function sendVolumeCommandDebounced(targetType, volumeVal) {
  clearTimeout(saveRemoteVolumeTimer);
  saveRemoteVolumeTimer = setTimeout(() => {
    try {
      localStorage.setItem('chzzk_remote_volumes', JSON.stringify(remoteVolumes));
    } catch (_) {}
  }, 300);

  clearTimeout(sendVolumeTimer);
  sendVolumeTimer = setTimeout(async () => {
    if (!userChannelId) return;
    try {
      await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/donations/command`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          command: 'VOLUME',
          volume: volumeVal,
          target: targetType
        })
      }).catch(() => null);

      const labels = {
        TTS: '채팅 후원 TTS',
        VIDEO: '영상 후원',
        CHAT: '채팅후원 알림음',
        MISSION: '미션후원 알림음'
      };
      const targetName = labels[targetType] || targetType;
      const existing = document.querySelectorAll('#toast-container .toast');
      existing.forEach(t => t.remove());
      showToast('🔊 볼륨 설정 반영', 'info', `${targetName}: ${volumeVal}%`);
    } catch (e) {
      console.warn('[Remote Volume] 볼륨 전송 실패:', e);
    }
  }, 350);
}

function onTTSVolumeChange(val) {
  remoteVolumes.tts = Number(val);
  const lbl = document.getElementById('remote-tts-vol-label');
  if (lbl) lbl.textContent = val + '%';
  sendVolumeCommandDebounced('TTS', Number(val));
}

function onVideoVolumeChange(val) {
  remoteVolumes.video = Number(val);
  const lbl = document.getElementById('remote-video-vol-label');
  if (lbl) lbl.textContent = val + '%';
  sendVolumeCommandDebounced('VIDEO', Number(val));
}

function onAlertVolumeChange(type, val) {
  const num = Number(val);
  if (type === 'chat') {
    remoteVolumes.chatAlert = num;
    const lbl = document.getElementById('remote-alert-chat-label');
    if (lbl) lbl.textContent = val + '%';
  } else if (type === 'video') {
    remoteVolumes.videoAlert = num;
    const lbl = document.getElementById('remote-alert-video-label');
    if (lbl) lbl.textContent = val + '%';
  } else if (type === 'mission') {
    remoteVolumes.missionAlert = num;
    const lbl = document.getElementById('remote-alert-mission-label');
    if (lbl) lbl.textContent = val + '%';
  }
  sendVolumeCommandDebounced(type.toUpperCase(), num);
}

function loadRemoteVolumes() {
  try {
    const saved = localStorage.getItem('chzzk_remote_volumes');
    if (saved) {
      const parsed = JSON.parse(saved);
      Object.assign(remoteVolumes, parsed);
    }
  } catch (_) {}
  const ttsSlider = document.getElementById('remote-tts-volume');
  const ttsLbl = document.getElementById('remote-tts-vol-label');
  if (ttsSlider) ttsSlider.value = remoteVolumes.tts;
  if (ttsLbl) ttsLbl.textContent = remoteVolumes.tts + '%';

  const vidSlider = document.getElementById('remote-video-volume');
  const vidLbl = document.getElementById('remote-video-vol-label');
  if (vidSlider) vidSlider.value = remoteVolumes.video;
  if (vidLbl) vidLbl.textContent = remoteVolumes.video + '%';

  const alertChatSlider = document.getElementById('remote-alert-chat-vol');
  const alertChatLbl = document.getElementById('remote-alert-chat-label');
  if (alertChatSlider) alertChatSlider.value = remoteVolumes.chatAlert;
  if (alertChatLbl) alertChatLbl.textContent = remoteVolumes.chatAlert + '%';

  const alertVideoSlider = document.getElementById('remote-alert-video-vol');
  const alertVideoLbl = document.getElementById('remote-alert-video-label');
  if (alertVideoSlider) alertVideoSlider.value = remoteVolumes.videoAlert;
  if (alertVideoLbl) alertVideoLbl.textContent = remoteVolumes.videoAlert + '%';

  const alertMissionSlider = document.getElementById('remote-alert-mission-vol');
  const alertMissionLbl = document.getElementById('remote-alert-mission-label');
  if (alertMissionSlider) alertMissionSlider.value = remoteVolumes.missionAlert;
  if (alertMissionLbl) alertMissionLbl.textContent = remoteVolumes.missionAlert + '%';
}

async function postponeMidrollAd(newsFeedNo, adId) {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('중간 광고 미루기')) return;
  if (!userChannelId) return;

  try {
    await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/ad/delay`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ newsFeedNo, adId, delayMinutes: 10 })
    }).catch(() => null);

    if (cachedNewsFeeds && cachedNewsFeeds.length > 0) {
      const item = cachedNewsFeeds.find(f => f.newsFeedNo === newsFeedNo || (adId && f.adId === adId));
      if (item) {
        item.adDelayCount = Math.min((item.adDelayCount || 0) + 1, item.adMaxDelayCount || 2);
      }
      renderRemoteFeed(cachedNewsFeeds);
    }

    const existing = document.querySelectorAll('#toast-container .toast');
    existing.forEach(t => t.remove());
    showToast('⏰ 광고 10분 미루기 완료', 'success', '중간 광고 시작 시간을 10분 연장했습니다.');
  } catch (e) {
    showToast('광고 미루기 실패', 'error', e.message);
  }
}

async function skipTTS() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('TTS 넘기기')) return;
  if (!userChannelId) {
    showToast('채널 정보 없음', 'warn', '채널 ID를 확인 중입니다.');
    return;
  }
  try {
    const res = await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/donations/command`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ command: 'SKIP' })
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
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
  if (!userChannelId) {
    showToast('채널 정보 없음', 'warn', '채널 ID를 확인 중입니다.');
    return;
  }
  isRemoteAlertPaused = !isRemoteAlertPaused;
  const iconEl = document.getElementById('icon-remote-pause');
  const textEl = document.getElementById('text-remote-pause');
  const btnEl = document.getElementById('btn-remote-pause-alert');
  const cmd = isRemoteAlertPaused ? 'PAUSE' : 'RESUME';

  try {
    const res = await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/donations/command`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ command: cmd })
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());

    if (res.ok) {
      if (isRemoteAlertPaused) {
        if (iconEl) iconEl.textContent = '▶️';
        if (textEl) textEl.textContent = '알림 재개';
        if (btnEl) btnEl.className = 'py-2.5 px-2 rounded-xl bg-amber-950/60 hover:bg-amber-900/80 border border-amber-600/50 text-amber-300 font-bold text-xs transition-colors cursor-pointer flex items-center justify-center gap-1.5 shadow-md';
        showToast('⏸️ 방송 알림 일시정지', 'info', 'OBS 화면 후원 알림 송출이 일시 중단되었습니다.');
      } else {
        if (iconEl) iconEl.textContent = '⏸️';
        if (textEl) textEl.textContent = '알림 정지';
        if (btnEl) btnEl.className = 'py-2.5 px-2 rounded-xl bg-[#1E1936] hover:bg-[#28214A] border border-[#5C3BFF]/40 text-[#C4B5FD] font-bold text-xs transition-colors cursor-pointer flex items-center justify-center gap-1.5 shadow-md';
        showToast('▶️ 방송 알림 재개', 'success', 'OBS 화면 후원 알림 송출이 재개되었습니다.');
      }
    } else {
      isRemoteAlertPaused = !isRemoteAlertPaused;
      const err = await res.json().catch(() => ({}));
      showToast('알림 제어 실패', 'error', err.message || '네이버 스튜디오 명령 전달 실패');
    }
  } catch (e) {
    isRemoteAlertPaused = !isRemoteAlertPaused;
    showToast('오류 발생', 'error', e.message);
  }
}

function switchRemoteTab(tab) {
  const tabs = ['all', 'video', 'mission', 'party'];
  tabs.forEach(t => {
    const btn = document.getElementById(`remote-tab-btn-${t}`);
    if (btn) {
      if (t === tab) {
        btn.className = 'pb-2 font-bold text-[#00FFA3] border-b-2 border-[#00FFA3] transition-all cursor-pointer';
      } else {
        btn.className = 'pb-2 font-medium text-slate-400 hover:text-slate-200 border-b-2 border-transparent transition-all cursor-pointer';
      }
    }
  });

  const feedContent = document.getElementById('remote-tab-content-feed');
  const missionContent = document.getElementById('remote-tab-content-mission');
  const partyContent = document.getElementById('remote-tab-content-party');

  if (tab === 'all') {
    if (feedContent) feedContent.style.display = 'block';
    if (missionContent) missionContent.style.display = 'none';
    if (partyContent) partyContent.style.display = 'none';
    filterFeed('ALL');
  } else if (tab === 'video') {
    if (feedContent) feedContent.style.display = 'block';
    if (missionContent) missionContent.style.display = 'none';
    if (partyContent) partyContent.style.display = 'none';
    filterFeed('VIDEO');
  } else if (tab === 'mission') {
    if (feedContent) feedContent.style.display = 'none';
    if (missionContent) missionContent.style.display = 'block';
    if (partyContent) partyContent.style.display = 'none';
    fetchNewsFeed();
  } else if (tab === 'party') {
    if (feedContent) feedContent.style.display = 'none';
    if (missionContent) missionContent.style.display = 'none';
    if (partyContent) partyContent.style.display = 'block';
    refreshPartyStatus();
  }
}

function replayVideoDonation(donationId, newsFeedNo, title) {
  showToast('🎬 영상 후원 재생', 'info', `"${title || '영상'}" 재생 요청 (치지직 스튜디오 공식 연동)`);
}

function sendDonationTest(type, btn) {
  showToast('🎉 후원 테스트 알림', 'info', `${type} 테스트 전송 (준비 중)`);
}

// ===== 실시간 후원 피드 & 대기열 & 미션 판정 로직 =====
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
  const diffDay = Math.floor(diffHour / 24);
  return `${diffDay}일 전`;
}

function filterFeed(type) {
  activeFeedFilter = type;
  const filters = ['all', 'cheese', 'video', 'mission', 'sub', 'follow'];
  filters.forEach(f => {
    const btn = document.getElementById(`feed-filter-${f}`);
    if (!btn) return;
    const isActive = (f === 'sub' && type === 'SUB') || (f === 'follow' && type === 'FOLLOW') || (f.toUpperCase() === type);
    if (isActive) {
      btn.className = 'px-2 py-0.5 rounded bg-[#161F2E] text-[#00FFA3] font-bold border border-[#00FFA3]/40 cursor-pointer';
    } else {
      btn.className = 'px-2 py-0.5 rounded bg-[#0F141C] text-slate-300 border border-[#1E2738] cursor-pointer';
    }
  });
  renderRemoteFeed(cachedNewsFeeds);
}

async function fetchNewsFeed() {
  if (!userChannelId) return;
  try {
    const res = await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/news-feeds?size=30`);
    if (res.ok) {
      const data = await res.json();
      const items = data.content?.data || data.content || [];
      cachedNewsFeeds = Array.isArray(items) ? items : [];
      renderRemoteFeed(cachedNewsFeeds);
      renderVideoQueue(cachedNewsFeeds);
      renderMissionList(cachedNewsFeeds);

      if (!lastPlayedVideo) {
        const vid = cachedNewsFeeds.find(f => f.newsFeedType === 'DONATION_VIDEO_CURRENCY');
        if (vid) {
          lastPlayedVideo = { donationId: vid.donationId, newsFeedNo: vid.newsFeedNo, title: vid.title };
        }
      }
    }
  } catch (e) {
    console.error('fetchNewsFeed failed:', e);
  }
}

function renderRemoteFeed(items) {
  const container = document.getElementById('remote-feed-list');
  if (!container) return;

  let filtered = items || [];
  if (activeFeedFilter === 'CHEESE') {
    filtered = filtered.filter(i => i.newsFeedType && (i.newsFeedType.includes('CHAT') || i.newsFeedType.includes('CHEESE')));
  } else if (activeFeedFilter === 'VIDEO') {
    filtered = filtered.filter(i => i.newsFeedType && i.newsFeedType.includes('VIDEO'));
  } else if (activeFeedFilter === 'MISSION') {
    filtered = filtered.filter(i => i.newsFeedType && i.newsFeedType.includes('MISSION'));
  } else if (activeFeedFilter === 'SUB') {
    filtered = filtered.filter(i => i.newsFeedType && i.newsFeedType.includes('SUBSCRIPTION'));
  } else if (activeFeedFilter === 'FOLLOW') {
    filtered = filtered.filter(i => i.newsFeedType && i.newsFeedType.includes('FOLLOW'));
  }

  if (filtered.length === 0) {
    container.innerHTML = `
      <div class="text-center py-10 text-[11px] text-slate-400 bg-[#090D13] rounded-lg border border-[#1A2333]/60">
        최근 방송 후원 알림 내역이 없습니다
      </div>`;
    return;
  }

  const typeMeta = {
    DONATION_CHAT_CURRENCY: { icon: '🧀', label: '치즈', color: 'text-amber-400' },
    DONATION_VIDEO_CURRENCY: { icon: '🎬', label: '영도', color: 'text-indigo-400' },
    DONATION_MISSION_SUGGESTION: { icon: '🎯', label: '미션', color: 'text-emerald-400' },
    DONATION_MISSION_PARTICIPATION: { icon: '🎯', label: '미션참여', color: 'text-emerald-400' },
    PARTY_DONATION: { icon: '🎉', label: '파티', color: 'text-pink-400' },
    SUBSCRIPTION: { icon: '⭐', label: '구독', color: 'text-cyan-400' },
    SUBSCRIPTION_GIFT: { icon: '🎁', label: '구독선물', color: 'text-cyan-400' },
    FOLLOW_CHANNEL: { icon: '❤️', label: '팔로우', color: 'text-rose-400' }
  };

  container.innerHTML = filtered.map(item => {
    let itemTime = item.createTime || item.donationTime || item.publishDate || item.createdTime || item.registeredDate || item.registeredDateTime || item.createdDate || item.date || item.timestamp || item.regDate || item.time;
    if (!itemTime && item.extraJson) {
      try {
        const ex = JSON.parse(item.extraJson);
        itemTime = ex.createTime || ex.donationTime || ex.createdDate || ex.publishDate;
      } catch (_) {}
    }
    const timeStr = formatTimeAgo(itemTime);
    const safeNewsFeedNo = item.newsFeedNo != null ? item.newsFeedNo : 'null';

    const isAd = item.newsFeedType && (item.newsFeedType.includes('AD') || item.newsFeedType === 'MIDROLL_AD');
    if (isAd) {
      const adTitle = item.title || '5분 뒤 중간 광고가 시작됩니다.';
      const delayCount = item.adDelayCount || 0;
      const maxDelay = item.adMaxDelayCount || 2;
      return `
        <div class="p-2.5 rounded-lg bg-[#140E26] border border-[#3B2874]/60 hover:border-[#5C3BFF]/50 transition-colors text-xs space-y-1.5">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1.5 min-w-0">
              <span class="w-4 h-4 rounded-full bg-[#7C3AED] text-white text-[9px] font-black flex items-center justify-center flex-shrink-0">AD</span>
              <span class="font-bold text-white truncate text-[11.5px]">${escapeHtml(adTitle)}</span>
            </div>
            ${timeStr ? `<span class="text-[10px] text-slate-400 font-medium flex-shrink-0">${timeStr}</span>` : ''}
          </div>
          <div class="text-[11px] text-slate-300 leading-snug">
            타이밍이 좋지 않다면, 광고 시작을 10분 미룰 수 있어요. 방송 당 2번 미루기 가능합니다.
          </div>
          <div class="pt-0.5">
            <button type="button" onclick="postponeMidrollAd(${safeNewsFeedNo}, '${item.adId || ''}')" class="px-2.5 py-1 rounded-md bg-[#2D1B69] hover:bg-[#3E258E] text-[#DDD6FE] border border-[#5C3BFF]/40 text-[10.5px] font-bold transition-all cursor-pointer flex items-center gap-1">
              <span>10분 미루기 (${delayCount}/${maxDelay})</span>
            </button>
          </div>
        </div>`;
    }

    const isFollow = item.newsFeedType && item.newsFeedType.includes('FOLLOW');
    if (isFollow) {
      const nickname = item.user?.nickname || '시청자';
      return `
        <div class="p-2 rounded-lg bg-[#090D13] border border-[#1A2333] hover:border-[#1E2738] transition-colors text-xs flex items-center justify-between">
          <div class="flex items-center gap-1.5 min-w-0">
            <span>❤️</span>
            <span class="font-bold text-slate-200 truncate max-w-[130px]">${escapeHtml(nickname)}</span>
            <span class="text-slate-300 text-[11.5px]">님이 팔로우하였습니다.</span>
          </div>
          ${timeStr ? `<span class="text-[10px] text-slate-400 font-medium flex-shrink-0">${timeStr}</span>` : ''}
        </div>`;
    }

    const meta = typeMeta[item.newsFeedType] || { icon: '💬', label: '알림', color: 'text-slate-300' };
    const nickname = item.user?.nickname || (item.anonymous ? '익명의 후원자' : '익명');
    const amount = item.payAmount ? Number(item.payAmount).toLocaleString() + ' 치즈' : '';
    const title = item.title || item.message || '';
    const isVideo = item.newsFeedType === 'DONATION_VIDEO_CURRENCY';

    const safeTitle = (title || '').replace(/'/g, "\\'").replace(/"/g, '&quot;');
    const safeDonationId = item.donationId || '';

    return `
      <div class="p-2 rounded-lg bg-[#090D13] border border-[#1A2333] hover:border-[#1E2738] transition-colors text-xs space-y-1">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-1.5 min-w-0">
            <span>${meta.icon}</span>
            <span class="font-bold text-slate-200 truncate max-w-[120px]">${escapeHtml(nickname)}</span>
            <span class="text-[9.5px] px-1 py-0.2 rounded font-medium bg-[#161F2E] ${meta.color}">${meta.label}</span>
          </div>
          <div class="flex items-center gap-2 flex-shrink-0">
            ${amount ? `<span class="font-mono font-bold text-[#00FFA3] text-[11px]">${amount}</span>` : ''}
            ${timeStr ? `<span class="text-[10px] text-slate-400 font-medium">${timeStr}</span>` : ''}
          </div>
        </div>
        ${title ? `<div class="text-[11px] text-slate-300 break-words leading-tight bg-[#05080C]/70 p-1.5 rounded">${escapeHtml(title)}</div>` : ''}
        ${isVideo ? `
          <div class="flex justify-end pt-1">
            <button type="button" onclick="replayVideoDonation('${safeDonationId}', ${safeNewsFeedNo}, '${safeTitle}')" class="px-2 py-0.5 rounded bg-[#161F2E] hover:bg-[#00FFA3]/20 text-[#00FFA3] border border-[#00FFA3]/40 text-[10px] font-bold flex items-center gap-1 transition-all cursor-pointer">
              <span>▶</span> <span>다시 재생</span>
            </button>
          </div>` : ''}
      </div>`;
  }).join('');
}

function renderVideoQueue(items) {
  const container = document.getElementById('remote-queue-list');
  const countBadge = document.getElementById('remote-queue-count');
  if (!container) return;

  const videoItems = (items || []).filter(i => i.newsFeedType === 'DONATION_VIDEO_CURRENCY');
  if (countBadge) countBadge.textContent = `대기 ${videoItems.length}건`;

  if (videoItems.length === 0) {
    container.innerHTML = `
      <div class="text-center py-4 text-[11px] text-slate-400 bg-[#090D13] rounded-lg border border-[#1A2333]/60">
        현재 대기열에 등록된 영상 후원이 없습니다
      </div>`;
    return;
  }

  container.innerHTML = videoItems.map((item, idx) => {
    const nickname = item.user?.nickname || '익명 후원자';
    const amount = item.payAmount ? Number(item.payAmount).toLocaleString() + ' 치즈' : '';
    const title = item.title || `영상 후원 #${idx + 1}`;
    const safeTitle = title.replace(/'/g, "\\'").replace(/"/g, '&quot;');
    const safeDonationId = item.donationId || '';
    const safeNewsFeedNo = item.newsFeedNo != null ? item.newsFeedNo : 'null';

    return `
      <div class="p-2 rounded-lg bg-[#090D13] border border-[#1A2333] hover:border-[#1E2738] flex items-center justify-between gap-2 text-xs">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-1.5">
            <span class="text-[10px] text-slate-500 font-mono">#${idx + 1}</span>
            <span class="font-bold text-slate-200 truncate text-[11.5px]">${escapeHtml(title)}</span>
          </div>
          <div class="flex items-center gap-2 text-[10px] text-slate-400 mt-0.5">
            <span>${escapeHtml(nickname)}</span>
            <span class="font-mono text-[#00FFA3] font-semibold">${amount}</span>
          </div>
        </div>
        <div class="flex items-center gap-1 flex-shrink-0">
          <button type="button" onclick="replayVideoDonation('${safeDonationId}', ${safeNewsFeedNo}, '${safeTitle}')" class="px-2 py-1 rounded bg-[#161F2E] hover:bg-[#00FFA3]/20 text-[#00FFA3] border border-[#00FFA3]/40 text-[10.5px] font-bold flex items-center gap-1 cursor-pointer transition-all" title="이 영상 다시 재생">
            <span>▶</span> <span>재생</span>
          </button>
        </div>
      </div>`;
  }).join('');
}

function setMissionSort(sortType) {
  currentMissionSort = sortType;
  const btnLatest = document.getElementById('mission-sort-latest');
  const btnAmount = document.getElementById('mission-sort-amount');
  if (btnLatest && btnAmount) {
    if (sortType === 'LATEST') {
      btnLatest.className = 'px-2 py-0.5 rounded text-[10px] font-bold bg-[#161F2E] text-[#00FFA3] border border-[#00FFA3]/40 transition-all cursor-pointer';
      btnAmount.className = 'px-2 py-0.5 rounded text-[10px] font-medium text-slate-400 hover:text-slate-200 transition-all cursor-pointer';
    } else {
      btnAmount.className = 'px-2 py-0.5 rounded text-[10px] font-bold bg-[#161F2E] text-[#00FFA3] border border-[#00FFA3]/40 transition-all cursor-pointer';
      btnLatest.className = 'px-2 py-0.5 rounded text-[10px] font-medium text-slate-400 hover:text-slate-200 transition-all cursor-pointer';
    }
  }
  renderMissionList(cachedNewsFeeds);
}

function renderMissionList(items) {
  const activeContainer = document.getElementById('remote-mission-active-list');
  const completedContainer = document.getElementById('remote-mission-completed-list');
  const activeCountBadge = document.getElementById('remote-mission-active-count');
  const completedCountBadge = document.getElementById('remote-mission-completed-count');
  if (!activeContainer || !completedContainer) return;

  const missionItems = (items || []).filter(i => i.newsFeedType && i.newsFeedType.includes('MISSION')).map(item => {
    let missionId = item.missionDonationId || item.donationId || '';
    let extra = null;
    if (item.extraJson) {
      try {
        extra = JSON.parse(item.extraJson);
        if (!missionId) missionId = extra?.missionDonation?.relatedMissionDonationId || '';
      } catch (_) {}
    }
    return { item, missionId, extra };
  });

  missionItems.sort((a, b) => {
    if (currentMissionSort === 'AMOUNT') {
      const amtA = Number(a.item.payAmount) || 0;
      const amtB = Number(b.item.payAmount) || 0;
      return amtB - amtA;
    }
    const aTime = a.item.donationTime || a.item.createTime || a.item.newsFeedNo || 0;
    const bTime = b.item.donationTime || b.item.createTime || b.item.newsFeedNo || 0;
    return bTime > aTime ? 1 : bTime < aTime ? -1 : 0;
  });

  const activeList = [];
  const completedList = [];

  missionItems.forEach(entry => {
    const { item, missionId, extra } = entry;
    const localStatus = missionId ? locallySettledMissions[missionId] : null;

    let isCompleted = false;
    let outcome = localStatus || null;

    if (localStatus) {
      isCompleted = true;
    } else {
      const rawStatus = (item.status || extra?.missionDonation?.status || extra?.status || '').toUpperCase();
      if (['SUCCESS', 'SUCCEED'].includes(rawStatus) || (item.newsFeedType && item.newsFeedType.includes('SUCCEED'))) {
        isCompleted = true;
        outcome = 'succeed';
      } else if (['FAIL', 'FAILED', 'REJECT', 'REJECTED'].includes(rawStatus) || (item.newsFeedType && item.newsFeedType.includes('FAIL'))) {
        isCompleted = true;
        outcome = 'fail';
      } else if (['COMPLETED', 'SETTLED', 'CANCELED'].includes(rawStatus) || (item.newsFeedType && item.newsFeedType.includes('SETTLED'))) {
        isCompleted = true;
        outcome = 'completed';
      }
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
      const { item, missionId } = entry;
      const nickname = item.user?.nickname || '후원자';
      const amount = item.payAmount ? Number(item.payAmount).toLocaleString() + ' 치즈' : '';
      const missionText = item.title || item.message || '미션 내용 없음';

      return `
        <div class="p-2 rounded-lg bg-[#090D13] border border-[#1A2333] space-y-1.5 text-xs">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1.5 min-w-0">
              <span>🎯</span>
              <span class="font-bold text-slate-200 truncate">${escapeHtml(nickname)}</span>
            </div>
            <span class="font-mono text-[#00FFA3] font-bold text-[11px] flex-shrink-0">${amount}</span>
          </div>
          <div class="text-[11px] text-slate-300 bg-[#05080C] p-2 rounded border border-[#1E2738]/50 break-words leading-normal">
            ${escapeHtml(missionText)}
          </div>
          ${missionId ? `
            <div class="flex items-center justify-end gap-1.5 pt-0.5">
              <button type="button" onclick="settleMission('${missionId}', 'succeed')" class="px-2.5 py-1 rounded bg-emerald-950/80 hover:bg-emerald-900 border border-emerald-700/60 text-[#00FFA3] font-bold text-[10.5px] cursor-pointer transition-colors flex items-center gap-1">
                <span>✅</span> <span>성공 판정</span>
              </button>
              <button type="button" onclick="settleMission('${missionId}', 'fail')" class="px-2.5 py-1 rounded bg-rose-950/80 hover:bg-rose-900 border border-rose-700/60 text-rose-300 font-bold text-[10.5px] cursor-pointer transition-colors flex items-center gap-1">
                <span>❌</span> <span>실패 판정</span>
              </button>
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
      const { item, outcome } = entry;
      const nickname = item.user?.nickname || '후원자';
      const amount = item.payAmount ? Number(item.payAmount).toLocaleString() + ' 치즈' : '';
      const missionText = item.title || item.message || '미션 내용 없음';

      let statusBadge = '<span class="text-[9.5px] px-1.5 py-0.2 rounded font-bold bg-slate-800 text-slate-300 border border-slate-700">🏁 완료</span>';
      if (outcome === 'succeed') {
        statusBadge = '<span class="text-[9.5px] px-1.5 py-0.2 rounded font-bold bg-emerald-950 text-emerald-400 border border-emerald-800/60">✅ 성공</span>';
      } else if (outcome === 'fail') {
        statusBadge = '<span class="text-[9.5px] px-1.5 py-0.2 rounded font-bold bg-rose-950 text-rose-400 border border-rose-800/60">❌ 실패</span>';
      }

      return `
        <div class="p-2 rounded-lg bg-[#070A0F] border border-[#161F2E] space-y-1.5 text-xs opacity-80 hover:opacity-100 transition-opacity">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1.5 min-w-0">
              <span class="text-slate-400">🎯</span>
              <span class="font-medium text-slate-300 truncate">${escapeHtml(nickname)}</span>
              ${statusBadge}
            </div>
            <span class="font-mono text-slate-400 text-[10.5px] flex-shrink-0">${amount}</span>
          </div>
          <div class="text-[10.5px] text-slate-400 bg-[#05080C]/80 p-1.5 rounded border border-[#1A2333]/40 break-words leading-tight">
            ${escapeHtml(missionText)}
          </div>
        </div>`;
    }).join('');
  }
}

async function settleMission(missionDonationId, outcome) {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('미션 판정')) return;
  if (!userChannelId) return;
  try {
    locallySettledMissions[missionDonationId] = outcome;
    const res = await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/donations/mission/${outcome}?missionDonationId=${encodeURIComponent(missionDonationId)}`, {
      method: 'PUT'
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());

    if (res.ok) {
      showToast(outcome === 'succeed' ? '🎉 미션 성공 판정 완료' : '⚠️ 미션 실패 처리 완료', 'success', '미션 결과가 스트리머/시청자에게 즉시 반영되었습니다.');
      fetchNewsFeed();
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

// ===== 치지직 방송 파티 관리 로직 =====
async function refreshPartyStatus(showFeedback = false) {
  if (!userChannelId) return;
  try {
    const [sumRes, partyRes] = await Promise.all([
      apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/party/summary`).catch(() => null),
      apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/party`).catch(() => null)
    ]);

    const viewCreate = document.getElementById('party-view-create');
    const viewManage = document.getElementById('party-view-manage');
    const activeCheese = document.getElementById('party-active-cheese');
    const badge = document.getElementById('party-status-badge');
    const editInput = document.getElementById('input-party-edit-name');
    const editLen = document.getElementById('party-edit-len');
    const btnRename = document.getElementById('btn-party-rename');
    const btnReissue = document.getElementById('btn-party-reissue-token');
    const roleBadge = document.getElementById('party-role-badge');
    const hostActions = document.getElementById('party-host-actions');
    const guestActions = document.getElementById('party-guest-actions');
    const countEl = document.getElementById('party-member-count');
    const listEl = document.getElementById('party-member-list');

    let summary = null;
    if (sumRes && sumRes.ok) {
      const sData = await sumRes.json();
      summary = sData.content;
      currentPartySummary = summary;
    }

    let party = null;
    if (partyRes && partyRes.ok) {
      const pData = await partyRes.json();
      party = pData.content;
    }

    const isPartyActive = summary && summary.partyId && summary.status !== 'FINISHED';

    if (isPartyActive) {
      if (viewCreate) viewCreate.style.display = 'none';
      if (viewManage) viewManage.style.display = 'block';

      const partyName = summary.partyName || party?.partyName || '치지직 방송 파티';
      const totalCheese = summary.totalDonationAmount || summary.cheese || 0;

      if (activeCheese) activeCheese.textContent = Number(totalCheese).toLocaleString() + ' 치즈';
      if (badge) {
        badge.textContent = '진행 중';
        badge.className = 'text-[10px] px-2 py-0.5 rounded font-bold bg-emerald-950/80 text-[#00FFA3] border border-emerald-800/60';
      }

      const isHost = party?.info?.isHost ?? summary?.isHost ?? (party?.members?.some(m => m.channelId === userChannelId && m.isHost) ?? true);
      isCurrentPartyHost = isHost;

      if (roleBadge) {
        if (isHost) {
          roleBadge.textContent = '👑 방장';
          roleBadge.className = 'text-[9px] px-1.5 py-0.2 rounded font-bold bg-[#161F2E] text-[#9E8BFF] border border-[#5C3BFF]/30';
        } else {
          roleBadge.textContent = '👤 게스트';
          roleBadge.className = 'text-[9px] px-1.5 py-0.2 rounded font-bold bg-[#161F2E] text-cyan-400 border border-cyan-500/30';
        }
      }

      if (editInput) {
        editInput.readOnly = !isHost;
        if (document.activeElement !== editInput) {
          editInput.value = partyName;
          if (editLen) editLen.textContent = `${partyName.length}/60`;
        }
      }
      if (btnRename) btnRename.style.display = isHost ? 'inline-block' : 'none';
      if (btnReissue) btnReissue.style.display = isHost ? 'inline-block' : 'none';
      if (hostActions) hostActions.style.display = isHost ? 'grid' : 'none';
      if (guestActions) guestActions.style.display = isHost ? 'none' : 'grid';

      fetchPartyInviteToken(summary.partyId);

      const members = party?.members || summary?.members || [];
      if (countEl) countEl.textContent = members.length;
      if (listEl) {
        if (members.length === 0) {
          listEl.innerHTML = `
            <div class="text-center py-3 text-[11px] text-slate-400 bg-[#090D13] rounded-lg border border-[#1A2333]/60">
              게스트가 들어오면 이곳에 보여요.
            </div>`;
        } else {
          listEl.innerHTML = members.map(m => `
            <div class="flex items-center justify-between p-1.5 rounded bg-[#090D13] border border-[#1A2333] text-xs">
              <div class="flex items-center gap-1.5 min-w-0">
                <span class="w-1.5 h-1.5 rounded-full bg-[#00FFA3] flex-shrink-0"></span>
                <span class="font-medium text-slate-200 truncate max-w-[140px]">${escapeHtml(m.nickname || m.channelName || '멤버')}</span>
              </div>
              <span class="text-[10px] ${m.isHost ? 'text-[#9E8BFF] font-bold' : 'text-slate-400'} flex-shrink-0">${m.isHost ? '👑 방장' : '게스트'}</span>
            </div>`).join('');
        }
      }
    } else {
      if (viewCreate) viewCreate.style.display = 'block';
      if (viewManage) viewManage.style.display = 'none';
      currentPartySummary = null;
      cachedPartyInviteToken = '';
      isCurrentPartyHost = true;
    }

    if (showFeedback) {
      const existingToasts = document.querySelectorAll('#toast-container .toast');
      existingToasts.forEach(t => t.remove());
      showToast('🎉 파티 상태 갱신', 'info', '최신 파티 정보를 동기화했습니다.');
    }
  } catch (e) {
    console.error('refreshPartyStatus error:', e);
  }
}

function extractPartyInviteToken(raw) {
  if (!raw || typeof raw !== 'string') return '';
  let str = raw.trim();
  const matchInvite = str.match(/party-invite\/([a-zA-Z0-9_-]+)/i);
  if (matchInvite && matchInvite[1]) return matchInvite[1];
  const matchTokenParam = str.match(/[?&]token=([a-zA-Z0-9_-]+)/i);
  if (matchTokenParam && matchTokenParam[1]) return matchTokenParam[1];
  str = str.replace(/^["'<(\[]+|["'>)\]]+$/g, '');
  return str;
}

async function joinPartyByInviteLink(inputId) {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('파티 합류')) return;
  if (!userChannelId) return;

  const input = document.getElementById(inputId);
  const raw = input ? input.value : '';
  const token = extractPartyInviteToken(raw);

  if (!token) {
    showToast('초대 링크 확인', 'warn', '유효한 파티 초대 링크나 코드를 입력해 주세요.');
    if (input) input.focus();
    return;
  }

  if (currentPartySummary && currentPartySummary.partyId && currentPartySummary.status !== 'FINISHED') {
    const currentName = currentPartySummary.partyName || '현재 파티';
    const ok = await showConfirmModal({
      title: '새 파티 합류',
      message: `현재 참여 중인 "${currentName}"에서 퇴장하고 새 파티에 합류하시겠습니까?`,
      confirmText: '파티 교체',
      cancelText: '취소',
      danger: false,
      icon: '👥'
    });
    if (!ok) return;
    try {
      if (isCurrentPartyHost) {
        await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/party/finish`, { method: 'PUT' });
      } else {
        await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/parties/${currentPartySummary.partyId}/member/leave`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({})
        });
      }
    } catch (_) {}
  }

  try {
    let partyTitle = '파티';
    try {
      const sumRes = await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/party/invitation/${token}/summary`);
      if (sumRes.ok) {
        const sumData = await sumRes.json();
        partyTitle = sumData.content?.partyInfo?.partyName || sumData.content?.partyName || partyTitle;
      }
    } catch (_) {}

    const res = await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/party/invitation/${token}/accept`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({})
    });

    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());

    if (res.ok) {
      showToast(`🎉 "${partyTitle}" 합류 완료!`, 'success', '파티 게스트로 성공적으로 참여했습니다.');
      if (input) input.value = '';
      const switchBox = document.getElementById('party-switch-box');
      if (switchBox) switchBox.classList.add('hidden');
      await refreshPartyStatus();
    } else {
      const data = await res.json().catch(() => ({}));
      showToast('파티 합류 실패', 'error', data.message || '초대 링크가 만료되었거나 이미 종료된 파티입니다.');
    }
  } catch (e) {
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    showToast('합류 오류 발생', 'error', e.message);
  }
}

async function leavePartySubmit() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('파티 나가기')) return;
  if (!userChannelId || !currentPartySummary?.partyId) return;
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
    const res = await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/parties/${currentPartySummary.partyId}/member/leave`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({})
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());

    if (res.ok) {
      showToast('🚪 파티 퇴장 완료', 'info', '파티에서 정상적으로 퇴장했습니다.');
      cachedPartyInviteToken = '';
      currentPartySummary = null;
      isCurrentPartyHost = true;
      await refreshPartyStatus();
    } else {
      const data = await res.json().catch(() => ({}));
      showToast('파티 퇴장 실패', 'error', data.message || '파티 나가기 실패');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

function toggleJoinPartyBox() {
  const box = document.getElementById('party-switch-box');
  if (!box) return;
  box.classList.toggle('hidden');
  if (!box.classList.contains('hidden')) {
    const input = document.getElementById('input-party-manage-join-link');
    if (input) input.focus();
  }
}

async function fetchPartyInviteToken(partyId) {
  if (!userChannelId || !partyId) return;
  try {
    const res = await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/parties/${partyId}/invitation/token`);
    if (res.ok) {
      const data = await res.json();
      const token = typeof data.content === 'string' ? data.content : data.content?.token;
      if (token) {
        cachedPartyInviteToken = token;
        const input = document.getElementById('field-party-invite-url');
        if (input) input.value = `https://chzzk.naver.com/party-invite/${token}`;
      }
    }
  } catch (e) {
    console.error('fetchPartyInviteToken error:', e);
  }
}

function togglePartyLinkMask() {
  const input = document.getElementById('field-party-invite-url');
  if (!input) return;
  input.type = input.type === 'password' ? 'text' : 'password';
}

// [보류 / v0.6.4 이후 검토]: navigator.clipboard 실패 시 execCommand('copy') 이중 폴백 처리.
// 차기 버전에서 공통 클립보드 헬퍼로 통합하거나 레거시 execCommand 완전 삭제 검토 필요.
function copyPartyInviteUrl() {
  const input = document.getElementById('field-party-invite-url');
  if (!input || !input.value) {
    showToast('복사 실패', 'warn', '복사할 초대 링크가 없습니다.');
    return;
  }
  navigator.clipboard.writeText(input.value).then(() => {
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    showToast('🔗 초대 링크 복사 완료', 'success', '클립보드에 파티 초대 링크가 복사되었습니다.');
  }).catch(() => {
    input.select();
    document.execCommand('copy');
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    showToast('🔗 초대 링크 복사 완료', 'success');
  });
}

async function reissuePartyInviteToken() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('초대 링크 재발급')) return;
  if (!userChannelId || !currentPartySummary?.partyId) {
    showToast('재발급 불가', 'warn', '활성화된 파티 정보를 찾을 수 없습니다.');
    return;
  }
  try {
    const res = await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/parties/${currentPartySummary.partyId}/invitation/token`, {
      method: 'PATCH'
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());

    if (res.ok) {
      showToast('🔄 초대 링크 재발급 완료', 'success', '새로운 파티 초대 링크가 발급되었습니다.');
      await fetchPartyInviteToken(currentPartySummary.partyId);
    } else {
      const data = await res.json().catch(() => ({}));
      showToast('재발급 실패', 'error', data.message || '초대 링크 재발급에 실패했습니다.');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function createPartySubmit() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('파티 만들기')) return;
  if (!userChannelId) return;
  const input = document.getElementById('input-party-create-name');
  const partyName = input ? input.value.trim() : '';
  if (!partyName) {
    showToast('입력 확인', 'warn', '생성할 파티명을 입력해 주세요.');
    if (input) input.focus();
    return;
  }

  try {
    const res = await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/party?partyName=${encodeURIComponent(partyName)}`, {
      method: 'POST'
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());

    if (res.ok) {
      showToast('🎉 파티 생성 완료!', 'success', `"${partyName}" 파티가 성공적으로 생성되었습니다.`);
      if (input) input.value = '';
      const createLen = document.getElementById('party-create-len');
      if (createLen) createLen.textContent = '0/60';
      await refreshPartyStatus();
    } else {
      const data = await res.json().catch(() => ({}));
      showToast('파티 생성 실패', 'error', data.message || '파티 생성 요청 실패');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function renamePartySubmit() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('파티명 변경')) return;
  if (!userChannelId) return;
  const input = document.getElementById('input-party-edit-name');
  const partyName = input ? input.value.trim() : '';
  if (!partyName) {
    showToast('입력 확인', 'warn', '수정할 파티명을 입력해 주세요.');
    if (input) input.focus();
    return;
  }

  try {
    const res = await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/party/name?partyName=${encodeURIComponent(partyName)}`, {
      method: 'PUT'
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());

    if (res.ok) {
      showToast('💾 파티명 저장 완료', 'success', `"${partyName}"(으)로 파티명이 변경되었습니다.`);
      await refreshPartyStatus();
    } else {
      const data = await res.json().catch(() => ({}));
      showToast('파티명 변경 실패', 'error', data.message || '파티명 수정 요청 실패');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function renewPartySubmit() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('파티 새로 만들기')) return;
  if (!userChannelId) return;
  const ok = await showConfirmModal({
    title: '새 파티 생성',
    message: '현재 파티를 정리하고 새로운 파티를 생성하시겠습니까?',
    confirmText: '새로 만들기',
    cancelText: '취소',
    danger: false,
    icon: '🔄'
  });
  if (!ok) return;

  try {
    const res = await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/party/renew`, {
      method: 'POST'
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());

    if (res.ok) {
      showToast('🔄 파티 새로 만들기 완료', 'success', '새로운 파티 준비 상태로 전환되었습니다.');
      await refreshPartyStatus();
    } else {
      const data = await res.json().catch(() => ({}));
      showToast('파티 갱신 실패', 'error', data.message || '파티 새로 만들기 실패');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function finishPartySubmit() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('파티 종료')) return;
  if (!userChannelId) return;
  const ok = await showConfirmModal({
    title: '파티 종료',
    message: '정말 현재 파티를 종료하시겠습니까?',
    submessage: '모든 참여자의 파티 연결이 종료됩니다.',
    confirmText: '파티 종료',
    cancelText: '취소',
    danger: true,
    icon: '🛑'
  });
  if (!ok) return;

  try {
    const res = await apiFetch(`/unofficial/manage/v1/channels/${userChannelId}/party/finish`, {
      method: 'PUT'
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());

    if (res.ok) {
      showToast('⏹️ 파티 종료 완료', 'info', '진행 중이던 파티가 정상적으로 종료되었습니다.');
      cachedPartyInviteToken = '';
      currentPartySummary = null;
      isCurrentPartyHost = true;
      await refreshPartyStatus();
    } else {
      const data = await res.json().catch(() => ({}));
      showToast('파티 종료 실패', 'error', data.message || '파티 종료 실패');
    }
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

function sendPartyDonationTest(btn) {
  sendDonationTest('party', btn);
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

function toggleSpamFilterInfo(e) {
  if (e) e.stopPropagation();
  const card = document.getElementById('dsetting-spam-info-card');
  if (card) card.classList.toggle('hidden');
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
  loadAllDonationSettings();
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
      const chatActive = document.getElementById('dsetting-chat-active');
      const chatMin = document.getElementById('dsetting-chat-min-amount');
      const chatExposure = document.getElementById('dsetting-chat-exposure');
      const chatEnvelope = document.getElementById('dsetting-chat-envelope');
      if (chatActive) chatActive.checked = d.donationActive !== false;
      const minVal = d.minCurrencyPayAmount || d.chatMinCurrencyPayAmount || 1000;
      if (chatMin) chatMin.value = Number(minVal).toLocaleString();
      if (chatExposure) chatExposure.checked = Number(d.exposureDonationAmount) > 0;
      if (chatEnvelope) chatEnvelope.checked = d.useLargeDonationEnvelope !== false && (d.useLargeDonationEnvelope === true || d.allowHighDonationEnvelope === true);
    }

    if (details.video) {
      const d = details.video;
      const vidActive = document.getElementById('dsetting-video-active');
      const vidMin = document.getElementById('dsetting-video-min-cheese');
      const vidSec = document.getElementById('dsetting-video-sec-cheese');
      const vidDur = document.getElementById('dsetting-video-max-duration');
      const vidAuto = document.getElementById('dsetting-video-autoplay');
      const vidTitle = document.getElementById('dsetting-video-title');
      const vidSpam = document.getElementById('dsetting-video-spam');

      if (vidActive) vidActive.checked = d.donationActive !== false;
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
      const subAlertActive = document.getElementById('dsetting-sub-alert-active');
      const subAlertVol = document.getElementById('dsetting-sub-alert-vol');
      const subAlertVolLbl = document.getElementById('dsetting-sub-alert-vol-label');
      const subTtsActive = document.getElementById('dsetting-sub-tts-active');
      const subTtsVol = document.getElementById('dsetting-sub-tts-vol');
      const subTtsVolLbl = document.getElementById('dsetting-sub-tts-vol-label');

      if (subAlertActive) subAlertActive.checked = d.alertActive !== false;
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
  } catch (e) {
    console.error('loadAllDonationSettings failed:', e);
  }
}

async function toggleChatActive(checked) {
  if (!userChannelId) return;
  try {
    await apiFetch('/chzzkdock/donations/chat', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: checked })
    });
    const mainEl = document.getElementById('toggle-chat-donation');
    if (mainEl) mainEl.checked = checked;
    currentDonationState.chatDonationActive = checked;
    updateDonationDetailButtonsState();

    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    showToast(checked ? '💬 채팅 후원 켜짐' : '💬 채팅 후원 꺼짐', 'info', `채팅 후원 기능이 ${checked ? '활성화' : '비활성화'}되었습니다.`);
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function saveChatDonationCondition() {
  if (!userChannelId) return;
  const minEl = document.getElementById('dsetting-chat-min-amount');
  const minVal = parseFormattedAmount(minEl?.value) || 1000;
  const isChatActive = document.getElementById('toggle-chat-donation') ? document.getElementById('toggle-chat-donation').checked : (currentDonationState.chatDonationActive !== false);

  try {
    await apiFetch('/chzzkdock/donations/details/chat', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        donationActive: isChatActive,
        minCurrencyPayAmount: minVal
      })
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    showToast('💾 후원 조건 설정 저장', 'success', `최소 후원 금액: ${minVal.toLocaleString()} 치즈`);
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function toggleChatExposure(checked) {
  if (!userChannelId) return;
  try {
    await apiFetch('/chzzkdock/donations/details/chat', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ exposureDonationAmount: checked ? 1000 : 0 })
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    showToast('후원 금액 표시 설정', 'info', checked ? '채팅창에 후원 금액을 표시합니다.' : '채팅창 후원 금액을 숨깁니다.');
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function toggleEnvelope(checked) {
  if (!userChannelId) return;
  const isChatActive = document.getElementById('toggle-chat-donation') ? document.getElementById('toggle-chat-donation').checked : (currentDonationState.chatDonationActive !== false);
  try {
    await apiFetch('/chzzkdock/donations/details/chat', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        donationActive: isChatActive,
        useLargeDonationEnvelope: checked
      })
    });
    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    showToast('고액 후원 봉투', 'info', checked ? '고액 후원 봉투 기능을 허용합니다.' : '고액 후원 봉투 기능을 차단합니다.');
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function toggleVideoActive(checked) {
  if (!userChannelId) return;
  try {
    await apiFetch('/chzzkdock/donations/video', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: checked })
    });
    const mainEl = document.getElementById('toggle-video-donation');
    if (mainEl) mainEl.checked = checked;
    currentDonationState.videoDonationActive = checked;
    updateDonationDetailButtonsState();

    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    showToast(checked ? '🎬 영상 후원 켜짐' : '🎬 영상 후원 꺼짐', 'info', `영상 후원 기능이 ${checked ? '활성화' : '비활성화'}되었습니다.`);
  } catch (e) {
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
    await apiFetch('/chzzkdock/donations/details/video', {
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
    showToast('💾 영상 후원 설정 저장 완료', 'success', '영상 후원 규칙이 성공적으로 반영되었습니다.');
  } catch (e) {
    showToast('오류 발생', 'error', e.message);
  }
}

async function toggleMissionActive(checked) {
  if (!userChannelId) return;
  try {
    await apiFetch('/chzzkdock/donations/mission', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: checked })
    });
    const mainEl = document.getElementById('toggle-mission-donation');
    if (mainEl) mainEl.checked = checked;
    currentDonationState.missionDonationActive = checked;
    updateDonationDetailButtonsState();

    const existingToasts = document.querySelectorAll('#toast-container .toast');
    existingToasts.forEach(t => t.remove());
    showToast(checked ? '🎯 미션 후원 켜짐' : '🎯 미션 후원 꺼짐', 'info', `미션 후원 기능이 ${checked ? '활성화' : '비활성화'}되었습니다.`);
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
  const alertActive = !!document.getElementById('dsetting-sub-alert-active')?.checked;
  const alertVol = Number(document.getElementById('dsetting-sub-alert-vol')?.value) || 70;
  const ttsActive = !!document.getElementById('dsetting-sub-tts-active')?.checked;
  const ttsVol = Number(document.getElementById('dsetting-sub-tts-vol')?.value) || 70;

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
  const iconBottom = document.getElementById('icon-refresh-bottom');
  if (iconBottom) iconBottom.classList.add('animate-spin');

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
  } finally {
    if (iconBottom) setTimeout(() => iconBottom.classList.remove('animate-spin'), 350);
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
    if (t === tab) {
      if (btn) {
        btn.className = 'flex-1 py-1 px-1 rounded-md text-[10.5px] font-bold transition-all flex items-center justify-center gap-1 bg-[#1A2436] text-[#00FFA3] border border-[#00FFA3]/40 shadow-sm cursor-pointer select-none';
      }
      if (pane) {
        pane.classList.remove('hidden');
        pane.classList.remove('tab-pane-animate');
        void pane.offsetWidth;
        pane.classList.add('tab-pane-animate');
      }
    } else {
      if (btn) {
        btn.className = 'flex-1 py-1 px-1 rounded-md text-[10.5px] font-medium transition-all flex items-center justify-center gap-1 text-slate-400 hover:text-slate-200 border border-transparent cursor-pointer select-none';
      }
      if (pane) pane.classList.add('hidden');
    }
  });
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
  const placeholder = document.getElementById('follower-options-placeholder');
  if (val === 'FOLLOWER') {
    if (opts) opts.style.display = 'flex';
    if (placeholder) placeholder.style.display = 'none';
  } else {
    if (opts) opts.style.display = 'none';
    if (placeholder) placeholder.style.display = 'none';
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
function toggleUrlsPanel() {
  if (typeof checkLoginGuard === 'function' && !checkLoginGuard('방송 URL 및 스트림 키 확인')) return;
  closeAllPopovers();
  const panel = document.getElementById('urls-panel');
  const settingsPanel = document.getElementById('settings-panel');
  const remotePanel = document.getElementById('remote-panel');
  if (settingsPanel && settingsPanel.classList.contains('open')) settingsPanel.classList.remove('open');
  if (remotePanel && remotePanel.classList.contains('open')) remotePanel.classList.remove('open');
  if (panel) {
    panel.classList.toggle('open');
    const elMainDock = document.getElementById('field-main-dock-url');
    if (elMainDock) elMainDock.value = `${window.location.origin}/`;
    const elMini = document.getElementById('field-mini-dock-url');
    if (elMini) elMini.value = `${window.location.origin}/mini`;
    const elStats = document.getElementById('field-stats-dock-url');
    if (elStats) elStats.value = `${window.location.origin}/stats`;
    if (panel.classList.contains('open')) {
      fetchStreamingInfo();
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

async function openUrlField(fieldId) {
  const input = document.getElementById(fieldId);
  if (!input || !input.value || !input.value.startsWith('http')) {
    showToast('열기 실패', 'warn', '유효한 URL이 아닙니다.');
    return;
  }
  await openExternalUrl(input.value, '해당 URL');
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

// 초기 로드 시 로컬 독 URL 즉시 사전 바인딩
document.addEventListener('DOMContentLoaded', () => {
  const elMainDock = document.getElementById('field-main-dock-url');
  if (elMainDock) elMainDock.value = `${window.location.origin}/`;
  const elMini = document.getElementById('field-mini-dock-url');
  if (elMini) elMini.value = `${window.location.origin}/mini`;
  const elStats = document.getElementById('field-stats-dock-url');
  if (elStats) elStats.value = `${window.location.origin}/stats`;
});

