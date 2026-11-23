// ============================================================
//  CHZZK OBS Dock - Unified Backend API Client
//  ui/shared/api.js
// ============================================================

// 빈 공간 우클릭 시 기본 브라우저 컨텍스트 메뉴 차단 (드래그 선택 텍스트 복사 및 입력창 복사/붙여넣기 우클릭은 허용)
if (typeof window !== 'undefined' && typeof document !== 'undefined' && typeof document.addEventListener === 'function') {
  document.addEventListener('contextmenu', (e) => {
    const target = e.target;
    const isEditable = target && typeof target.closest === 'function' && target.closest('input, textarea, [contenteditable="true"]');
    const hasSelection = typeof window.getSelection === 'function' && window.getSelection().toString().trim().length > 0;
    if (isEditable || hasSelection) {
      return;
    }
    e.preventDefault();
  });
}

/**
 * Core Unified API Fetch
 * - 'X-Requested-With': 'ChzzkDock' 보안 헤더 기본 주입
 * - JSON body 전달 시 'Content-Type': 'application/json' 자동 설정
 * - 401/403 세션 만료 시 handleSessionExpired() 자동 위임
 * - AbortSignal 기반 타임아웃 가드 (기본 15초)
 */
async function apiFetch(url, options = {}) {
  const defaultHeaders = {
    'X-Requested-With': 'ChzzkDock'
  };
  if (options.body && typeof options.body === 'string' && !options.headers?.['Content-Type']) {
    defaultHeaders['Content-Type'] = 'application/json';
  }
  const mergedHeaders = Object.assign({}, defaultHeaders, options.headers || {});
  const mergedOptions = Object.assign({}, options, { headers: mergedHeaders });

  // 무한 대기 방지 네이티브 타임아웃 (기본 15초, /login-wait는 예외)
  if (!mergedOptions.signal && !url.includes('/login-wait')) {
    mergedOptions.signal = AbortSignal.timeout(options.timeout || 15000);
  }

  try {
    const res = await fetch(url, mergedOptions);
    if ((res.status === 401 || res.status === 403) && typeof handleSessionExpired === 'function') {
      handleSessionExpired();
    }
    return res;
  } catch (err) {
    if (err.name === 'TimeoutError' || err.name === 'AbortError') {
      console.warn(`[API] 요청 시간 초과 중단: ${url}`);
      throw new Error('서버 응답 시간 초과 (네트워크 지연)');
    }
    throw err;
  }
}

/**
 * 치지직 독 전용 백엔드 BFF API 클라이언트
 * 각 버튼/토글마다 개별 엔드포인트로 1:1 매핑되어 경량화된 단일 요청을 전송합니다.
 */
const chzzkApi = {
  // --- [1. 실시간 채팅 모드] (단일 토글 / 셀렉트 분리) ---
  getChatMode: () => apiFetch('/chzzkdock/chat-mode'),
  setEmojiMode: (enabled) => apiFetch('/chzzkdock/chat-mode/emoji', {
    method: 'PUT',
    body: JSON.stringify({ enabled: !!enabled })
  }),
  setSlowMode: (slowSec) => apiFetch('/chzzkdock/chat-mode/slow', {
    method: 'PUT',
    body: JSON.stringify({ slowSec: Number(slowSec) || 0 })
  }),

  // --- [2. 후원 제어 4종] (각 스위치마다 독립 엔드포인트 전송) ---
  getDonations: () => apiFetch('/chzzkdock/donations'),
  // 범용 후원 토글 디스패처
  toggleDonation: (target, enabled) => {
    const t = target === 'sub' ? 'subscription' : target.toLowerCase();
    return apiFetch(`/chzzkdock/donations/${t}`, {
      method: 'PUT',
      body: JSON.stringify({ enabled: !!enabled })
    });
  },

  // --- [3. 윈도우 웹뷰 제어 & 시스템 기능] ---
  openChatWebview: (channelId = '') => apiFetch(`/chat-webview?channelId=${encodeURIComponent(channelId || '')}`),
  openRemoteWebview: (channelId = '') => apiFetch(`/remote-webview?channelId=${encodeURIComponent(channelId || '')}`),
  openFolder: (path) => apiFetch(`/api/open-folder?path=${encodeURIComponent(path)}`),
  getDiskTotal: (drive = 'C') => apiFetch(`/api/disk-total?drive=${encodeURIComponent(drive)}`),
  getObsBitrateTarget: (profile = '') => apiFetch(`/api/obs-bitrate-target?profile=${encodeURIComponent(profile)}`),

  // --- [4. 방송 설정 & 치지직 상태] ---
  getLiveStatus: (channelId = '') => apiFetch(`/chzzkdock/live-status${channelId ? '?channelId=' + encodeURIComponent(channelId) : ''}`),
  getBroadcast: () => apiFetch('/chzzkdock/broadcast'),
  saveBroadcast: (payload) => apiFetch('/chzzkdock/broadcast', {
    method: 'PUT',
    body: JSON.stringify(payload)
  }),
  getChatRule: () => apiFetch('/chzzkdock/chat-rule'),

  // --- [5. 리모컨 & 파티 관리] ---
  getRemoteState: () => apiFetch('/chzzkdock/remote/state'),
  getRemoteFeeds: (typeFilters = '', size = 20) => apiFetch(
    `/chzzkdock/remote/feeds?size=${encodeURIComponent(size)}${typeFilters ? '&typeFilters=' + encodeURIComponent(typeFilters) : ''}`
  ),
  setRemotePause: (active) => apiFetch('/chzzkdock/remote/pause', {
    method: 'PUT',
    body: JSON.stringify({ active: !!active })
  }),
  setRemoteVolume: (target, volume) => apiFetch('/chzzkdock/remote/volume', {
    method: 'PATCH',
    body: JSON.stringify({ target, volume: Number(volume) })
  }),
  sendRemoteCommand: (command, donationId = '', newsFeedNo = null) => apiFetch('/chzzkdock/remote/command', {
    method: 'POST',
    body: JSON.stringify({ command, donationId, newsFeedNo })
  }),
  postponeAd: () => apiFetch('/chzzkdock/remote/ad/postpone', {
    method: 'POST',
    body: JSON.stringify({})
  }),
  settleMission: (outcome, missionDonationId) => apiFetch(
    `/chzzkdock/remote/mission/${encodeURIComponent(outcome)}?missionDonationId=${encodeURIComponent(missionDonationId)}`,
    { method: 'PUT' }
  ),
  getParty: () => apiFetch('/chzzkdock/remote/party'),
  createParty: (partyName) => apiFetch('/chzzkdock/remote/party', {
    method: 'POST',
    body: JSON.stringify({ partyName })
  }),
  renameParty: (partyName) => apiFetch('/chzzkdock/remote/party/name', {
    method: 'PUT',
    body: JSON.stringify({ partyName })
  }),
  reissuePartyToken: (partyNo) => apiFetch(`/chzzkdock/remote/party/token?partyNo=${encodeURIComponent(partyNo)}`, {
    method: 'PATCH'
  }),
  joinParty: (token) => apiFetch('/chzzkdock/remote/party/join', {
    method: 'POST',
    body: JSON.stringify({ token })
  }),
  leaveParty: (partyNo) => apiFetch('/chzzkdock/remote/party/leave', {
    method: 'POST',
    body: JSON.stringify({ partyNo })
  }),
  endParty: (status = '') => apiFetch('/chzzkdock/remote/party/end', {
    method: 'POST',
    body: JSON.stringify({ status })
  }),
  openChannelLive: (channelId) => apiFetch('/chzzkdock/open-channel', {
    method: 'POST',
    body: JSON.stringify({ channelId })
  })
};
