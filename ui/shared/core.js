// ============================================================
//  CHZZK OBS Dock - Shared Core Client & Unified Modal System
//  ui/shared/core.js
// ============================================================

var serverConfig = {};

// API 통신 함수(apiFetch 및 chzzkApi)는 ui/shared/api.js에서 단일 제공됩니다.

async function handleSessionExpired() {
  // 백엔드에 저장된 NID_AUT가 있는지 확인 (백엔드 자동 갱신 중일 수 있으므로 즉시 파기 방지)
  try {
    const cRes = await fetch('/config', { headers: { 'X-Requested-With': 'XMLHttpRequest' } });
    if (cRes.ok) {
      const cfg = await cRes.json();
      if (cfg && cfg.nid_aut) {
        const userRes = await apiFetch('/chzzkdock/user-status');
        if (userRes.ok) {
          const uContent = await userRes.json();
          if (uContent && (uContent.loggedIn === true || uContent.channelId)) {
            return;
          }
        }
      }
    }
  } catch (e) {}

  userChannelId = "";
  currentChannelName = "";
  currentChannelAvatarUrl = "";
  const elChannelName = document.getElementById('header-channel-name');
  const headerAvatarImg = document.getElementById('header-avatar-img');
  const headerLiveSvg = document.getElementById('header-live-svg');
  const headerLiveIcon = document.getElementById('header-live-icon');
  const popoverMenu = document.getElementById('channel-popover-menu');

  if (elChannelName) elChannelName.style.display = 'none';
  if (headerAvatarImg) { headerAvatarImg.src = ''; headerAvatarImg.classList.add('hidden'); }
  if (headerLiveSvg) headerLiveSvg.classList.remove('hidden');
  if (headerLiveIcon) headerLiveIcon.style.border = 'none';
  if (popoverMenu) popoverMenu.classList.add('hidden');
  if (typeof liveCheckTimeout !== 'undefined') clearTimeout(liveCheckTimeout);

  const text = document.getElementById('header-status-text');
  const badgeA = document.getElementById('badge-method-a');
  const badgeB = document.getElementById('badge-method-b');
  const groupLoginBadge = document.getElementById('group-login-badge');
  const btnWebview = document.getElementById('btn-webview-login');
  const inputAut = document.getElementById('cookie-nid-aut');
  const inputSes = document.getElementById('cookie-nid-ses');

  if (text) { text.textContent = '로그인 필요'; text.className = 'font-medium text-[#FF4D6A]'; }
  if (badgeA) { badgeA.textContent = "⚠️ 로그인 필요"; badgeA.className = "text-[9px] px-1.5 py-0.5 rounded font-bold bg-amber-950 text-amber-400 border border-amber-800/60"; }
  if (badgeB) { badgeB.textContent = "⚠️ 미연동"; badgeB.className = "text-[9px] px-1.5 py-0.5 rounded font-bold bg-amber-950 text-amber-400 border border-amber-800/60"; }
  
  if (groupLoginBadge) {
    groupLoginBadge.textContent = "로그인 필요";
    groupLoginBadge.className = "text-[9px] px-1.5 py-0.5 rounded font-bold bg-amber-950 text-amber-400 border border-amber-800/60";
  }
  if (btnWebview) {
    btnWebview.innerHTML = "🔑 네이버 로그인 시작";
    btnWebview.className = "save-btn text-xs py-2 bg-emerald-600 hover:bg-emerald-500 text-white shadow-lg shadow-emerald-950/40 transition-colors cursor-pointer";
    btnWebview.onclick = (typeof startWebviewLogin === 'function') ? startWebviewLogin : null;
  }
  if (inputAut) inputAut.className = "chzzk-input border-amber-500/60 bg-amber-950/20 text-amber-300";
  if (inputSes) inputSes.className = "chzzk-input border-amber-500/60 bg-amber-950/20 text-amber-300";
  if (typeof setBroadcastFormLocked === 'function') setBroadcastFormLocked(true);
}

async function loadServerConfig() {
  try {
    const res = await apiFetch('/config');
    if (res.ok) {
      serverConfig = await res.json();
      const offlineBanner = document.getElementById('dock-server-offline-banner');
      if (offlineBanner) offlineBanner.classList.add('hidden');
      return;
    }
  } catch (e) {
    const offlineBanner = document.getElementById('dock-server-offline-banner');
    if (offlineBanner) offlineBanner.classList.remove('hidden');
    if (typeof showToast === 'function') {
      showToast('⚠️ 로컬 서버 연결 실패', 'warn', 'chzzk-dock.exe 로컬 서버가 실행 중인지 확인하세요.');
    }
  }
}

async function resolveChannelId() {
  if (userChannelId) return userChannelId;
  try {
    const res = await apiFetch('/chzzkdock/user-status');
    if (res.ok) {
      const content = await res.json();
      if (content && (content.loggedIn === true || content.channelId)) {
        userChannelId = content.channelId || "";
        return userChannelId;
      }
    }
  } catch (e) {
    console.warn('resolveChannelId 실패:', e);
  }
  return "";
}


// ============================================================
//  CHZZK OBS Dock - Unified Toast & Modal System
// ============================================================

var confirmModalResolver = null;

function escapeHtml(str) {
  if (typeof str !== 'string') return str;
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

var activeToastCooldowns = new Set();

/**
 * 플로팅 토스트 알림 표시 (최상단 노출, 3초 후 자동 제거 및 클릭 시 즉시 제거)
 * @param {string} title - 제목
 * @param {'success'|'error'|'warn'|'info'} type - 알림 종류
 * @param {string} [detail=''] - 세부 메시지
 */
function showToast(title, type = 'info', detail = '') {
  const container = document.getElementById('toast-container');
  if (!container) return;

  const toastKey = `${type}:${title}:${detail}`;
  if (activeToastCooldowns.has(toastKey)) {
    return;
  }

  const cooldown = (type === 'warn' || type === 'error') ? 2000 : 1000;
  activeToastCooldowns.add(toastKey);
  setTimeout(() => activeToastCooldowns.delete(toastKey), cooldown);

  const toast = document.createElement('div');
  toast.className = `toast ${type}`;

  const iconMap = {
    success: '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="20 6 9 17 4 12"/></svg>',
    error: '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>',
    warn: '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
    info: '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>'
  };

  let message = title;
  if (!message && detail) {
    message = detail;
    detail = '';
  }

  toast.innerHTML = `
    <div class="flex items-center gap-1.5 font-bold">
      ${iconMap[type] || iconMap.info}
      <span>${escapeHtml(message)}</span>
    </div>
    ${detail ? `<div class="text-[10px] opacity-80 pl-4 font-normal">${escapeHtml(detail)}</div>` : ''}
  `;

  // 클릭 시 즉시 닫기
  toast.onclick = () => {
    toast.classList.add('removing');
    setTimeout(() => {
      if (toast.parentNode) toast.remove();
    }, 150);
  };

  container.appendChild(toast);

  // 3초 후 페이드아웃 및 100% 제거
  setTimeout(() => {
    toast.classList.add('removing');
    setTimeout(() => {
      if (toast.parentNode) toast.remove();
    }, 260);
  }, 3000);
}

/**
 * 공통 비동기 안내/알림 모달 (Custom Notice Modal)
 */
function showNoticeModal({
  title = '안내',
  message = '',
  submessage = '',
  html = '',
  icon = 'ℹ️',
  buttons = null
} = {}) {
  const modal = document.getElementById('modal-custom-confirm');
  const elTitle = document.getElementById('confirm-modal-title');
  const elMsg = document.getElementById('confirm-modal-message');
  const elSub = document.getElementById('confirm-modal-submessage');
  const elCustom = document.getElementById('confirm-modal-custom-content');
  const elIcon = document.getElementById('confirm-modal-icon');
  const btnContainer = document.getElementById('confirm-modal-buttons');

  if (elTitle) elTitle.textContent = title;
  if (elIcon) elIcon.textContent = icon;
  if (elMsg) { elMsg.textContent = message; elMsg.classList.toggle('hidden', !message); }
  if (elSub) { elSub.textContent = submessage; elSub.classList.toggle('hidden', !submessage); }
  if (elCustom) { elCustom.innerHTML = html; elCustom.classList.toggle('hidden', !html); }

  if (btnContainer) {
    btnContainer.innerHTML = '';
    const list = buttons && buttons.length > 0 ? buttons : [
      {
        text: '확인',
        className: 'px-3.5 py-1.5 rounded-lg bg-[#00FFA3] hover:bg-[#00E591] text-black text-xs font-bold transition-all cursor-pointer',
        onClick: () => window.__resolveConfirmModal(true)
      }
    ];
    list.forEach(b => {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = b.className || 'px-3.5 py-1.5 rounded-lg bg-[#161F2E] hover:bg-[#1E2738] text-[#00FFA3] text-xs font-bold cursor-pointer transition-colors';
      btn.textContent = b.text;
      btn.onclick = (e) => {
        if (b.onClick) b.onClick(e);
        else window.__resolveConfirmModal(true);
      };
      btnContainer.appendChild(btn);
    });
  }

  if (modal) {
    modal.classList.remove('hidden');
  }
}

/**
 * 공통 비동기 확인/취소 모달 (Promise 기반)
 */
function showConfirmModal({ title = '확인', message = '', submessage = '', confirmText = '확인', cancelText = '취소', danger = false, icon = '⚠️' } = {}) {
  return new Promise((resolve) => {
    confirmModalResolver = resolve;
    showNoticeModal({
      title,
      message,
      submessage,
      icon,
      buttons: [
        {
          text: cancelText,
          className: 'px-3 py-1.5 rounded-lg bg-[#1C1C26] hover:bg-[#282837] text-zinc-300 text-xs font-medium transition-colors cursor-pointer',
          onClick: () => window.__resolveConfirmModal(false)
        },
        {
          text: confirmText,
          className: danger
            ? 'px-3.5 py-1.5 rounded-lg bg-red-500/90 hover:bg-red-500 text-white text-xs font-bold shadow-lg shadow-red-500/20 transition-all cursor-pointer'
            : 'px-3.5 py-1.5 rounded-lg bg-[#00FFA3] hover:bg-[#00E591] text-black text-xs font-bold transition-all cursor-pointer',
          onClick: () => window.__resolveConfirmModal(true)
        }
      ]
    });
    const cancelBtn = document.getElementById('confirm-modal-buttons')?.firstElementChild;
    if (cancelBtn) cancelBtn.focus();
  });
}

window.__resolveConfirmModal = function(val) {
  const modal = document.getElementById('modal-custom-confirm');
  if (modal) modal.classList.add('hidden');
  if (confirmModalResolver) {
    const r = confirmModalResolver;
    confirmModalResolver = null;
    r(val);
  }
};

window.__closeConfirmModal = function() {
  window.__resolveConfirmModal(false);
};


