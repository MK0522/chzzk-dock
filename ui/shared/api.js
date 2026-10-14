// ============================================================
//  CHZZK OBS Dock - Unified Backend API Client
//  ui/shared/api.js
// ============================================================

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
  fetch: apiFetch,

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
  toggleDonationChat: (enabled) => apiFetch('/chzzkdock/donations/chat', {
    method: 'PUT',
    body: JSON.stringify({ enabled: !!enabled })
  }),
  toggleDonationVideo: (enabled) => apiFetch('/chzzkdock/donations/video', {
    method: 'PUT',
    body: JSON.stringify({ enabled: !!enabled })
  }),
  toggleDonationMission: (enabled) => apiFetch('/chzzkdock/donations/mission', {
    method: 'PUT',
    body: JSON.stringify({ enabled: !!enabled })
  }),
  toggleDonationSubscription: (enabled) => apiFetch('/chzzkdock/donations/subscription', {
    method: 'PUT',
    body: JSON.stringify({ enabled: !!enabled })
  }),
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
  openBrowser: (url) => apiFetch(`/open-browser?url=${encodeURIComponent(url)}`),
  openFolder: (path) => apiFetch(`/api/open-folder?path=${encodeURIComponent(path)}`),
  getDiskTotal: (drive = 'C') => apiFetch(`/api/disk-total?drive=${encodeURIComponent(drive)}`),

  // --- [4. 방송 설정 & 치지직 상태] ---
  getUserStatus: () => apiFetch('/chzzkdock/user-status'),
  getLiveStatus: (channelId = '') => apiFetch(`/chzzkdock/live-status${channelId ? '?channelId=' + encodeURIComponent(channelId) : ''}`),
  getBroadcast: () => apiFetch('/chzzkdock/broadcast'),
  saveBroadcast: (payload) => apiFetch('/chzzkdock/broadcast', {
    method: 'PUT',
    body: JSON.stringify(payload)
  }),
  getChatRule: () => apiFetch('/chzzkdock/chat-rule'),
  saveChatRule: (payload) => apiFetch('/chzzkdock/chat-rule', {
    method: 'PUT',
    body: JSON.stringify(payload)
  }),

  // --- [5. 앱 설정 & 인증 세션] ---
  getConfig: () => apiFetch('/config'),
  getSessions: () => apiFetch('/sessions'),
  getAppSettings: () => apiFetch('/app-settings'),
  saveAppSettings: (patch) => apiFetch('/app-settings', {
    method: 'PUT',
    body: JSON.stringify(patch)
  })
};
