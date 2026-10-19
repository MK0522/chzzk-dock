// ui/main-dock/js/broadcast.js


async function fetchSessionBootstrap() {
  const res = await apiFetch('/chzzkdock/session/bootstrap');
  if (!res.ok) throw new Error(`부트스트랩 실패 (HTTP ${res.status})`);
  const data = await res.json();

  if (data.config) {
    serverConfig = data.config;
    const offlineBanner = document.getElementById('dock-server-offline-banner');
    if (offlineBanner) offlineBanner.classList.add('hidden');
  }

  if (Array.isArray(data.sessions)) {
    cachedSessions = data.sessions;
  }

  return applyBootstrapState(data);
}

function applyBootstrapState(content) {
  const text = document.getElementById('header-status-text');
  const badgeA = document.getElementById('badge-method-a');
  const badgeB = document.getElementById('badge-method-b');
  const inputAut = document.getElementById('cookie-nid-aut');
  const inputSes = document.getElementById('cookie-nid-ses');
  const btnWebview = document.getElementById('btn-webview-login');
  const elChannelName = document.getElementById('header-channel-name');

  const DUMMY_MASK = "••••••••••••••••••••••••••••••••";
  if (serverConfig.nid_aut && inputAut && document.activeElement !== inputAut) inputAut.value = DUMMY_MASK;
  if (serverConfig.nid_ses && inputSes && document.activeElement !== inputSes) inputSes.value = DUMMY_MASK;

  if (content && (content.loggedIn === true || content.channelId)) {
    userChannelId = content.channelId || "";
    currentChannelName = content.nickname || "";
    currentChannelAvatarUrl = content.profileImageUrl || "";

    const headerAvatarImg = document.getElementById('header-avatar-img');
    const headerLiveSvg = document.getElementById('header-live-svg');
    if (headerAvatarImg && headerLiveSvg) {
      if (currentChannelAvatarUrl) {
        headerAvatarImg.src = currentChannelAvatarUrl;
        headerAvatarImg.classList.remove('hidden');
        headerLiveSvg.classList.add('hidden');
      } else {
        headerAvatarImg.classList.add('hidden');
        headerLiveSvg.classList.remove('hidden');
      }
    }

    if (elChannelName) {
      if (currentChannelName) {
        elChannelName.style.display = 'inline-flex';
        updateLiveStatusUI((typeof isCurrentlyLive !== 'undefined') ? isCurrentlyLive : false);
      } else {
        elChannelName.style.display = 'none';
      }
    }
    checkLiveStatus();
    if (typeof checkChannelMonetization === 'function') checkChannelMonetization();

    if (text) { text.textContent = '연동됨'; text.className = 'font-medium text-[#00FFA3]'; }
    if (badgeA) { badgeA.textContent = "● 쿠키 연동됨"; badgeA.className = "text-[9px] px-1.5 py-0.5 rounded font-bold bg-emerald-950 text-[#00FFA3] border border-emerald-800/60"; }
    if (badgeB) { badgeB.textContent = "● 등록됨"; badgeB.className = "text-[9px] px-1.5 py-0.5 rounded font-bold bg-emerald-950 text-[#00FFA3] border border-emerald-800/60"; }
    const groupLoginBadge = document.getElementById('group-login-badge');
    if (groupLoginBadge) {
      groupLoginBadge.textContent = "연동됨";
      groupLoginBadge.className = "text-[9px] px-1.5 py-0.5 rounded font-bold bg-emerald-950 text-[#00FFA3] border border-emerald-800/60";
    }
    if (btnWebview) {
      btnWebview.innerHTML = "🚪 로그아웃";
      btnWebview.className = "save-btn text-xs py-2 bg-rose-900/60 hover:bg-rose-800 text-rose-200 border border-rose-600/60 transition-colors cursor-pointer";
      btnWebview.onclick = handleLogout;
    }
    if (inputAut) inputAut.className = "chzzk-input border-emerald-500/40 bg-emerald-950/10 text-emerald-300";
    if (inputSes) inputSes.className = "chzzk-input border-emerald-500/40 bg-emerald-950/10 text-emerald-300";
    setBroadcastFormLocked(false);
    return userChannelId;
  } else {
    userChannelId = "";
    currentChannelName = "";
    if (elChannelName) elChannelName.style.display = 'none';
    handleSessionExpired();
    return "";
  }
}

async function updateConnectionStatus() {
  try {
    return await fetchSessionBootstrap();
  } catch (e) {
    if (!userChannelId) {
      const text = document.getElementById('header-status-text');
      const elChannelName = document.getElementById('header-channel-name');
      if (text) { text.textContent = '오류'; text.className = 'font-medium text-[#FF4D6A]'; }
      if (elChannelName) elChannelName.style.display = 'none';
      setBroadcastFormLocked(true);
    }
    return userChannelId;
  }
}

async function initConnection() {
  const INIT_MAX_RETRIES = 5;
  const INIT_RETRY_DELAY_MS = 1500;
  for (let attempt = 1; attempt <= INIT_MAX_RETRIES; attempt++) {
    if (isLoggingIn) return;

    try {
      const channelId = await updateConnectionStatus();
      if (channelId) {
        await loadCurrentSettings(true);
        return;
      }
    } catch (e) {
      console.warn(`[CHZZK Dock] 연결 시도 ${attempt} 예외:`, e);
    }

    if (attempt < INIT_MAX_RETRIES) {
      await new Promise(r => setTimeout(r, INIT_RETRY_DELAY_MS));
    }
  }

  if (!serverConfig.nid_aut && !serverConfig.nid_ses) {
    showToast('⚙ 로그인 필요', 'warn', '치지직 로그인이 필요합니다. [설정] ➔ 로그인 버튼을 눌러 연동해 주세요.');
  } else {
    showToast('⚠️ 치지직 연결 지연', 'warn', '치지직 서버 응답이 지연되고 있습니다. 잠시 후 다시 시도해 주세요.');
  }
}


    var currentTags = [];
    var categorySearchTimeout = null;
    var userChannelId = "";
    var currentChannelName = "";
    var currentChannelAvatarUrl = "";

    var currentLiveSetting = {
      adult: false,
      clipActive: true,
      paidPromotion: false,
      krOnlyViewing: false,
      replayPublishType: 'MANUAL',
      watchPartyNo: null,
      rawContent: null
    };

    // 드롭스(Drops) 상태 관리
    var currentDropsCampaignNo = null;
    var isDropsActive = false;
    var isDropsSupported = false;
    var currentDropsCampaignTitle = '';

    var availableWatchParties = [];
    var activeWatchParty = null;
    var isLoggingIn = false;

    // 카테고리 & 태그 & OBS 전역 상태
    var livePopularCategories = [];
    var livePopularCategoriesLastFetch = 0;
    var categoryFavoritesList = [];
    var isTagInputOpen = false;
    var currentObsStatus = { detected: false, path: '', installed: false, needs_update: false };
    var selfHealRunning = false;

    // ===== Font Scale & Screen Zoom (화면 및 글꼴 배율: 1.0, 1.2, 1.35) =====

    function captureCurrentBroadcastState() {
      const elTitle = document.getElementById('live-title');
      const elCatId = document.getElementById('category-id');
      const elCatType = document.getElementById('category-type');
      const elCatSearch = document.getElementById('category-search');
      const elAdult = document.getElementById('toggle-adult');
      const elClip = document.getElementById('toggle-clip');
      const elOverseas = document.getElementById('toggle-overseas');
      const elPaid = document.getElementById('toggle-paid-promotion');
      const elReplay = document.querySelector('input[name="replayPublishType"]:checked');

      return {
        title: (elTitle ? elTitle.value : '').trim(),
        categoryId: elCatId ? elCatId.value : '',
        categoryType: elCatType ? elCatType.value : '',
        categoryValue: (elCatSearch ? elCatSearch.value : '').trim(),
        tags: (currentTags || []).slice().sort().join(','),
        thumbnail: currentLiveSetting.defaultThumbnailImageUrl || '',
        adult: elAdult ? elAdult.checked : false,
        clipActive: elClip ? elClip.checked : true,
        overseas: elOverseas ? elOverseas.checked : true,
        paidPromotion: elPaid ? elPaid.checked : false,
        replayPublishType: elReplay ? elReplay.value : 'MANUAL',
        watchPartyNo: (document.getElementById('select-watch-party')?.value || ''),
        boardMessage: currentDonationBoardMessage || '',
        dropsCampaignNo: (isDropsSupported && isDropsActive) ? currentDropsCampaignNo : null
      };
    }


    function updateThumbnailToolbarButton() {
      const btn = document.getElementById('btn-thumbnail');
      const label = document.getElementById('thumbnail-status-label');
      if (!btn || !label) return;

      const hasThumb = !!currentLiveSetting.defaultThumbnailImageUrl;
      if (hasThumb) {
        btn.classList.remove('bg-[#121721]', 'border-[#243044]', 'text-slate-200');
        btn.classList.add('bg-[#00FFA3]/10', 'border-[#00FFA3]/60', 'text-[#00FFA3]');
        label.textContent = '등록됨';
        label.className = 'text-[9.5px] px-1.5 py-0.2 rounded bg-[#00FFA3]/20 text-[#00FFA3] font-mono border border-[#00FFA3]/40 leading-none';
      } else {
        btn.classList.remove('bg-[#00FFA3]/10', 'border-[#00FFA3]/60', 'text-[#00FFA3]');
        btn.classList.add('bg-[#121721]', 'border-[#243044]', 'text-slate-200');
        label.textContent = '미등록';
        label.className = 'text-[9.5px] px-1.5 py-0.2 rounded bg-[#161F2E] text-slate-400 font-mono border border-[#243044] leading-none';
      }
    }

    // ── 드롭스(Drops) 캠페인 도우미 함수 ──
    const dropsCampaignDetailsCache = {};


    function extractDropsCampaignNo(cat) {
      if (!cat) return null;
      if (Array.isArray(cat.dropsCampaignNos) && cat.dropsCampaignNos.length > 0) {
        const validNos = cat.dropsCampaignNos.map(Number).filter(n => !isNaN(n) && n > 0);
        if (validNos.length > 0) {
          // 배열 내 가장 최신(번호가 가장 큰) 활성 캠페인 선택
          return Math.max(...validNos);
        }
      }
      if (cat.dropsCampaignNo) {
        return Number(cat.dropsCampaignNo) || cat.dropsCampaignNo;
      }
      if (cat.dropsCampaign && cat.dropsCampaign.campaignNo) {
        return Number(cat.dropsCampaign.campaignNo) || cat.dropsCampaign.campaignNo;
      }
      return null;
    }


    async function fetchDropsCampaignDetails(campaignNo) {
      if (!campaignNo) return null;
      if (dropsCampaignDetailsCache[campaignNo]) {
        applyDropsCampaignDetails(dropsCampaignDetailsCache[campaignNo]);
        return dropsCampaignDetailsCache[campaignNo];
      }
      try {
        const res = await apiFetch(`/chzzkdock/drops/match?campaignNo=${encodeURIComponent(campaignNo)}`);
        if (res.ok) {
          const matchData = await res.json();
          if (matchData && matchData.supported) {
            const details = {
              campaignNo: matchData.campaignNo || campaignNo,
              title: matchData.title || '드롭스 캠페인',
              isTerminated: matchData.isTerminated,
              rewards: matchData.rewards || [],
              linkUrl: matchData.linkUrl
            };
            dropsCampaignDetailsCache[campaignNo] = details;
            applyDropsCampaignDetails(details);
            return details;
          }
        }
      } catch (e) {
        console.warn('드롭스 캠페인 상세 조회 오류:', e);
      }
      return null;
    }

    let activeChannelDrops = []; // 채널별 실시간 활성 드롭스 캠페인 목록 캐시

    function applyDropsCampaignDetails(details) {
      if (!details) return;
      currentDropsCampaignTitle = details.title;
      updateDropsBadgeUI(details);
    }

    // 치지직 스튜디오 공식 API: 현재 채널이 참여 가능한 실시간 활성 드롭스 캠페인 목록 조회 (BFF)
    async function fetchChannelDropsCampaigns(channelId) {
      try {
        const res = await apiFetch('/chzzkdock/drops/campaigns');
        if (res.ok) {
          const data = await res.json();
          activeChannelDrops = Array.isArray(data) ? data : (data?.content?.campaignList || []);
          return activeChannelDrops;
        }
      } catch (e) {
        console.warn('채널 드롭스 캠페인 목록 조회 실패:', e);
      }
      return activeChannelDrops;
    }

    // 카테고리 ID 또는 이름과 일치하는 실시간 활성 드롭스 캠페인 탐색

    function findActiveDropsCampaign(categoryId, categoryValue) {
      if (!activeChannelDrops || activeChannelDrops.length === 0) return null;
      const targetIdStr = categoryId ? String(categoryId).trim() : '';
      const targetValStr = categoryValue ? String(categoryValue).trim().toLowerCase() : '';

      return activeChannelDrops.find(c => {
        const catId = c.categoryId || c.category?.categoryId || '';
        const catVal = c.categoryValue || c.category?.categoryValue || '';
        if (targetIdStr && catId && String(catId).trim() === targetIdStr) return true;
        if (targetValStr && catVal && String(catVal).trim().toLowerCase() === targetValStr) return true;
        return false;
      }) || null;
    }


    async function checkCategoryDropsSupport(keyword, targetId) {
      if (!keyword && !targetId) return;
      try {
        // 1. 채널 활성 드롭스 목록이 아직 비어있다면 공식 API 우선 로드
        if (activeChannelDrops.length === 0 && userChannelId) {
          await fetchChannelDropsCampaigns(userChannelId);
        }

        // 2. 공식 채널 활성 캠페인과 매칭
        const matched = findActiveDropsCampaign(targetId, keyword);
        if (matched && matched.campaignNo) {
          currentDropsCampaignNo = matched.campaignNo;
          isDropsSupported = true;
          isDropsActive = true;
          updateDropsBadgeUI();
          await fetchDropsCampaignDetails(matched.campaignNo);
          checkBroadcastStateDirty();
          return;
        }

        // 3. 만약 채널 활성 목록에서 못 찾았더라도 카테고리 검색 결과 보조 확인
        const results = await fetchCategoriesApi(keyword || targetId);
        const match = results.find(item => (targetId && item.categoryId === targetId) || (item.categoryValue && keyword && item.categoryValue.toLowerCase() === keyword.toLowerCase())) || results[0];
        if (match) {
          const dropsNo = extractDropsCampaignNo(match);
          if (dropsNo) {
            const details = await fetchDropsCampaignDetails(dropsNo);
            // 종료되지 않은 캠페인인 경우에만 채택
            if (details && !details.isTerminated) {
              currentDropsCampaignNo = dropsNo;
              isDropsSupported = true;
              isDropsActive = true;
              updateDropsBadgeUI();
              checkBroadcastStateDirty();
              return;
            }
          }
        }

        // 드롭스 미지원 카테고리인 경우 리셋
        currentDropsCampaignNo = null;
        isDropsSupported = false;
        isDropsActive = false;
        currentDropsCampaignTitle = '';
        updateDropsBadgeUI();
      } catch (e) {
        console.warn('드롭스 지원 여부 확인 오류', e);
      }
    }

    // ── 드롭스(Drops) 캠페인 원클릭 토글 뱃지 & 보상 툴팁 UI ──

    function updateDropsBadgeUI(cachedDetails = null) {
      const btn = document.getElementById('badge-drops-toggle');
      const text = document.getElementById('drops-toggle-text');
      const tooltip = document.getElementById('drops-reward-tooltip');
      const tooltipTitle = document.getElementById('drops-tooltip-title');
      const tooltipRewards = document.getElementById('drops-tooltip-rewards');
      if (!btn || !text) return;

      if (!isDropsSupported || !currentDropsCampaignNo) {
        btn.classList.add('hidden');
        if (tooltip) tooltip.classList.add('!hidden');
        return;
      }

      btn.classList.remove('hidden');
      if (tooltip) tooltip.classList.remove('!hidden');

      if (isDropsActive) {
        btn.className = 'px-1.5 py-0.5 rounded text-[9.5px] font-bold transition-all flex items-center gap-1 leading-none cursor-pointer border bg-[#00FFA3]/15 text-[#00FFA3] border-[#00FFA3]/40 hover:bg-[#00FFA3]/25';
        text.textContent = '드롭스 ON';
        btn.title = '클릭해서 On/Off';
      } else {
        btn.className = 'px-1.5 py-0.5 rounded text-[9.5px] font-medium transition-all flex items-center gap-1 leading-none cursor-pointer border bg-[#161F2E] text-slate-400 border-[#243044] hover:bg-[#1E2738] hover:text-slate-300';
        text.textContent = '드롭스 OFF';
        btn.title = '클릭해서 On/Off';
      }

      // 보상 툴팁 내용 업데이트
      const details = cachedDetails || (currentDropsCampaignNo ? dropsCampaignDetailsCache[currentDropsCampaignNo] : null);
      if (tooltipTitle && details) {
        const titleText = details.title || '드롭스 보상';
        if (details.linkUrl) {
          tooltipTitle.innerHTML = `<a href="${escapeHtml(details.linkUrl)}" target="_blank" onclick="event.preventDefault(); openExternalUrl('${escapeHtml(details.linkUrl)}', '드롭스 상세 안내');" class="text-white hover:text-[#00FFA3] underline decoration-[#00FFA3]/50 hover:decoration-[#00FFA3] transition-colors truncate flex items-center gap-1 cursor-pointer" title="드롭스 안내 페이지 열기 (기본 브라우저)"><span>${escapeHtml(titleText)}</span><svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" class="shrink-0 text-[#00FFA3]"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg></a>`;
        } else {
          tooltipTitle.textContent = titleText;
        }
      }
      if (tooltipRewards) {
        if (!details || !details.rewards || details.rewards.length === 0) {
          tooltipRewards.innerHTML = '<div class="text-[10px] text-slate-400 py-1.5 text-center">등록된 보상 정보가 없습니다.</div>';
        } else {
          tooltipRewards.innerHTML = details.rewards.map(r => `
            <div class="flex items-center gap-2 p-1.5 rounded-lg bg-[#161F2E] border border-[#243044] hover:bg-[#1C283C] transition-colors">
              <div class="w-8 h-8 rounded-md bg-[#0D1219] border border-[#243044] flex items-center justify-center p-0.5 shrink-0 overflow-hidden">
                ${r.imageUrl ? `<img src="${escapeHtml(r.imageUrl)}" class="w-full h-full object-contain" onerror="this.parentElement.innerHTML='🎁'" />` : '<span class="text-xs">🎁</span>'}
              </div>
              <div class="min-w-0 flex-1">
                ${r.conditionText ? `<div class="text-[9.5px] text-slate-400 font-medium leading-none mb-0.5">${escapeHtml(r.conditionText)}</div>` : ''}
                <div class="text-[10.5px] font-bold text-white leading-snug break-keep">${escapeHtml(r.name)}</div>
              </div>
            </div>
          `).join('');
        }
      }
    }


    function toggleDropsCampaign() {
      if (!isDropsSupported) return;
      isDropsActive = !isDropsActive;
      updateDropsBadgeUI();
      checkBroadcastStateDirty();
      showToast(isDropsActive ? '🎁 드롭스 활성화' : '🎁 드롭스 비활성화', 'info', isDropsActive ? '이번 방송에 드롭스 보상이 적용됩니다.' : '이번 방송에서 드롭스 보상이 제외됩니다.');
    }


    function updateSavedBroadcastBaseline() {
      savedBroadcastBaseline = captureCurrentBroadcastState();
      updateThumbnailToolbarButton();
      updateDropsBadgeUI();
      checkBroadcastStateDirty();
    }


    function checkBroadcastStateDirty() {
      if (!savedBroadcastBaseline) {
        setSaveButtonDirty(false);
        return false;
      }
      const cur = captureCurrentBroadcastState();
      const base = savedBroadcastBaseline;

      const titleDirty = cur.title !== base.title;
      const catDirty = (cur.categoryId !== base.categoryId || cur.categoryValue !== base.categoryValue);
      const tagsDirty = cur.tags !== base.tags;
      const thumbDirty = cur.thumbnail !== base.thumbnail;
      const adultDirty = cur.adult !== base.adult;
      const clipDirty = cur.clipActive !== base.clipActive;
      const overseasDirty = cur.overseas !== base.overseas;
      const paidDirty = cur.paidPromotion !== base.paidPromotion;
      const replayDirty = cur.replayPublishType !== base.replayPublishType;
      const wpDirty = cur.watchPartyNo !== base.watchPartyNo;
      const boardDirty = cur.boardMessage !== base.boardMessage;
      const dropsDirty = cur.dropsCampaignNo !== (base.dropsCampaignNo || null);

      // 1. 개별 입력 요소 테두리 & 타이틀 점 하이라이트 (글로우 효과 없음)
      const elTitle = document.getElementById('live-title');
      if (elTitle) elTitle.classList.toggle('border-dirty', titleDirty);
      const dotTitle = document.getElementById('dot-dirty-title');
      if (dotTitle) dotTitle.style.display = titleDirty ? 'inline-block' : 'none';

      const elCatSearch = document.getElementById('category-search');
      if (elCatSearch) elCatSearch.classList.toggle('border-dirty', (catDirty || dropsDirty));
      const dotCat = document.getElementById('dot-dirty-category');
      if (dotCat) dotCat.style.display = (catDirty || dropsDirty) ? 'inline-block' : 'none';

      const elTagBox = document.getElementById('tag-container');
      if (elTagBox) elTagBox.classList.toggle('border-dirty', tagsDirty);
      const dotTags = document.getElementById('dot-dirty-tags');
      if (dotTags) dotTags.style.display = tagsDirty ? 'inline-block' : 'none';

      const elThumbBtn = document.getElementById('btn-thumbnail');
      if (elThumbBtn) elThumbBtn.classList.toggle('border-dirty', thumbDirty);
      const dotThumb = document.getElementById('dot-dirty-thumbnail');
      if (dotThumb) dotThumb.style.display = thumbDirty ? 'inline-block' : 'none';

      const elWpSelect = document.getElementById('select-watch-party');
      if (elWpSelect) elWpSelect.classList.toggle('border-dirty', wpDirty);
      const dotWp = document.getElementById('dot-dirty-watch-party');
      if (dotWp) dotWp.style.display = wpDirty ? 'inline-block' : 'none';

      // 2. 캡슐 탭 (다시보기, 방송 옵션) 변경 알림
      const tabReplay = document.getElementById('tab-btn-replay');
      const dotTabReplay = document.getElementById('dot-dirty-tab-replay');
      if (tabReplay) tabReplay.classList.toggle('border-dirty', replayDirty);
      if (dotTabReplay) dotTabReplay.style.display = replayDirty ? 'inline-block' : 'none';

      const streamDirty = adultDirty || clipDirty || overseasDirty || paidDirty;
      const tabStream = document.getElementById('tab-btn-stream');
      const dotTabStream = document.getElementById('dot-dirty-tab-stream');
      if (tabStream) tabStream.classList.toggle('border-dirty', streamDirty);
      if (dotTabStream) dotTabStream.style.display = streamDirty ? 'inline-block' : 'none';

      // 세부 옵션 개별 라벨 테두리 하이라이트
      const elAdult = document.getElementById('label-adult');
      if (elAdult) elAdult.classList.toggle('border-dirty', adultDirty);
      const elClip = document.getElementById('label-clip');
      if (elClip) elClip.classList.toggle('border-dirty', clipDirty);
      const elOverseas = document.getElementById('label-overseas');
      if (elOverseas) elOverseas.classList.toggle('border-dirty', overseasDirty);

      const isDirty = titleDirty || catDirty || tagsDirty || thumbDirty || streamDirty || replayDirty || wpDirty || boardDirty || dropsDirty;

      setSaveButtonDirty(isDirty);
      return isDirty;
    }


    function setSaveButtonDirty(isDirty) {
      const btn = document.getElementById('btn-save');
      const saveText = document.getElementById('save-text');
      const btnRevert = document.getElementById('btn-revert');
      if (!btn) return;

      if (isDirty) {
        btn.classList.add('is-dirty');
        if (saveText) saveText.textContent = '변경사항 적용';
        if (btnRevert) btnRevert.disabled = false;
      } else {
        btn.classList.remove('is-dirty');
        if (saveText) saveText.textContent = '적용';
        if (btnRevert) btnRevert.disabled = true;
      }
    }



    // ===== Live Status & Real-time Viewers Polling (방송 중: 10초 실시간 갱신, 오프라인: 15초) =====
    let liveCheckTimeout = null;
    let isCurrentlyLive = false;
    let currentViewerCount = 0;
    let showViewerCountState = true;


    async function checkLiveStatus() {
      clearTimeout(liveCheckTimeout);
      if (!userChannelId) {
        liveCheckTimeout = setTimeout(checkLiveStatus, 10000);
        return;
      }

      try {
        let isLive = false;
        let viewerCount = 0;

        // 치지직 실시간 라이브 상태 BFF 호출 (쿠키 불필요, 공개 캐싱)
        const res = await chzzkApi.getLiveStatus();
        if (res.ok) {
          const content = await res.json();
          if (content && (content.isLive || content.status === 'OPEN')) {
            isLive = true;
            viewerCount = typeof content.concurrentUserCount === 'number' ? content.concurrentUserCount : 0;
          }
        }

        isCurrentlyLive = isLive;
        currentViewerCount = viewerCount;
        updateLiveStatusUI(isLive, viewerCount);
      } catch (e) {
        console.warn('[CHZZK Dock] 라이브 상태 확인 오류:', e);
      }

      // 방송 중일 때는 10초마다 시청자 증감 실시간 반영, 오프라인일 때는 15초마다 확인
      const nextDelay = isCurrentlyLive ? 10000 : 15000;
      liveCheckTimeout = setTimeout(checkLiveStatus, nextDelay);
    }


    function updateLiveStatusUI(openLive, viewerCount = 0) {
      const iconBtn = document.getElementById('header-live-icon');
      const iconSvg = document.getElementById('header-live-svg');
      const elChannelName = document.getElementById('header-channel-name');
      const elChannelText = document.getElementById('header-channel-text');
      const elViewerBadge = document.getElementById('header-viewer-count');
      const elViewerVal = document.getElementById('viewer-count-val');
      if (!iconBtn) return;

      if (openLive) {
        // 방송 중 (ON / LIVE): 2px 솔리드 레드 테두리 (글로우/블러 금지)
        iconBtn.style.border = '2px solid #FF4D6A';
        iconBtn.style.boxShadow = 'none';
        iconBtn.style.filter = 'none';
        iconBtn.title = '치지직 방송 중 (ON) - 클릭 시 즉시 새로고침 (3초 쿨타임)';
        if (iconSvg) {
          iconSvg.setAttribute('stroke', '#FF4D6A');
          iconSvg.classList.remove('opacity-70');
        }

        if (elViewerBadge && elViewerVal) {
          const formattedCount = Number(viewerCount || 0).toLocaleString('ko-KR');
          elViewerVal.textContent = formattedCount;
          if (showViewerCountState) {
            elViewerBadge.classList.remove('hidden');
            elViewerBadge.classList.add('flex');
            elViewerBadge.title = `실시간 동시 시청자: ${formattedCount}명 (10초 주기 실시간 반영)`;
          } else {
            elViewerBadge.classList.add('hidden');
            elViewerBadge.classList.remove('flex');
          }
        }
      } else {
        // 방송 꺼짐 (OFF): 테두리 없음
        iconBtn.style.border = 'none';
        iconBtn.style.boxShadow = 'none';
        iconBtn.style.filter = 'none';
        iconBtn.title = '치지직 방송 꺼짐 (OFF) - 클릭 시 즉시 새로고침 (3초 쿨타임)';
        if (iconSvg) {
          iconSvg.setAttribute('stroke', '#64748B');
          iconSvg.classList.add('opacity-70');
        }

        if (elViewerBadge) {
          elViewerBadge.classList.add('hidden');
          elViewerBadge.classList.remove('flex');
        }
      }

      if (elChannelText && currentChannelName) {
        elChannelText.textContent = currentChannelName;
      }
      if (elChannelName) {
        elChannelName.style.display = currentChannelName ? 'inline-flex' : 'none';
      }

      const popover = document.getElementById('channel-popover-menu');
      if (popover && !popover.classList.contains('hidden')) {
        renderChannelPopoverList();
      }
    }


    function toggleChannelMenu() {
      const menu = document.getElementById('channel-popover-menu');
      if (!menu) return;
      const isHidden = menu.classList.contains('hidden');
      if (isHidden) {
        renderChannelPopoverList();
        menu.classList.remove('hidden');
      } else {
        menu.classList.add('hidden');
      }
    }

    let cachedSessions = null;


    function renderSessionItems(sessions) {
      const listContainer = document.getElementById('channel-list-items');
      if (!listContainer) return;
      if (!sessions || sessions.length === 0) {
        listContainer.innerHTML = '<div class="p-2 text-center text-[10px] text-slate-400">연동된 계정이 없습니다.</div>';
        return;
      }
      listContainer.innerHTML = sessions.map(s => {
        const isActive = s.active;
        const isExp = s.expired;
        // 아바타 테두리: 활성 계정이고 라이브 중이면 2px 빨간색 테두리(글로우 없음), 그 외는 테두리 없음 (50% 확대: 40px)
        const avatarBorder = (isActive && isCurrentlyLive) ? 'border: 2px solid #FF4D6A;' : 'border: none;';
        const avatarEl = s.profile_image_url
          ? `<img src="${escapeHtml(s.profile_image_url)}" class="w-10 h-10 rounded-full object-cover flex-shrink-0" style="${avatarBorder}" alt="" onerror="this.outerHTML='<div class=\\'w-10 h-10 rounded-full bg-[#161F2E] flex items-center justify-center text-sm flex-shrink-0\\'>📺</div>';" />`
          : `<div class="w-10 h-10 rounded-full bg-[#161F2E] flex items-center justify-center text-sm flex-shrink-0" style="${avatarBorder}">📺</div>`;

        const activeBadge = isActive
          ? '<span class="text-[8.5px] px-1 py-0.2 rounded font-bold bg-[#00FFA3]/15 text-[#00FFA3] border border-[#00FFA3]/30">사용 중</span>'
          : '';
        const expiredBadge = isExp
          ? '<span class="text-[8.5px] px-1 py-0.2 rounded font-bold bg-amber-950 text-amber-400 border border-amber-800/60">⚠️ 만료됨</span>'
          : '';

        const cardBorderClass = isActive
          ? 'border-[#00FFA3]/40 bg-[#161F2E]'
          : 'border-[#2E3F58] bg-[#0E131C] hover:border-slate-500 hover:bg-[#161F2E]';

        return `
          <div class="flex items-center justify-between p-1.5 rounded-lg border ${cardBorderClass} transition-colors group cursor-pointer" onclick="handleSessionCardClick(${s.index}, ${isExp}, ${isActive})">
            <div class="flex items-center gap-2 min-w-0 pr-1 flex-1">
              ${avatarEl}
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-1">
                  <span class="text-[14px] font-bold text-slate-100 truncate">${escapeHtml(s.channel_name || '치지직 채널')}</span>
                  ${activeBadge}
                  ${expiredBadge}
                </div>
                <div class="text-[9px] text-slate-400 font-mono truncate">${escapeHtml(s.channel_id || (isExp ? '재로그인 필요' : ''))}</div>
              </div>
            </div>
            <div class="flex items-center gap-1 flex-shrink-0" onclick="event.stopPropagation()">
              ${s.channel_id ? `
              <button type="button" onclick="openExternalUrl('https://chzzk.naver.com/live/${encodeURIComponent(s.channel_id)}', '${escapeHtml(s.channel_name || '채널')}');" class="w-6 h-6 rounded flex items-center justify-center text-slate-300 hover:text-[#00FFA3] hover:bg-[#202C3F] transition-colors cursor-pointer" title="방송 바로가기 (브라우저)">
                <span class="text-xs">🌐</span>
              </button>
              ` : ''}
              <button type="button" onclick="removeAccountSession(${s.index})" class="w-6 h-6 rounded flex items-center justify-center text-slate-400 hover:text-rose-400 hover:bg-[#202C3F] transition-colors cursor-pointer" title="계정 연동 해제">
                <span class="text-xs">🗑️</span>
              </button>
            </div>
          </div>
        `;
      }).join('');
    }


    async function renderChannelPopoverList(silent = false) {
      const listContainer = document.getElementById('channel-list-items');
      if (!listContainer) return;

      // SWR: 캐시가 존재하면 로딩 문구 없이 즉시 이전 데이터 렌더링 (깜빡임 원천 차단)
      if (cachedSessions && cachedSessions.length > 0) {
        renderSessionItems(cachedSessions);
      } else {
        listContainer.innerHTML = '<div class="p-2 text-center text-[10px] text-slate-400">계정 목록 불러오는 중...</div>';
      }

      try {
        const res = await apiFetch('/sessions');
        if (!res.ok) throw new Error('세션 목록 조회 실패');
        const data = await res.json();
        cachedSessions = data.sessions || [];
        renderSessionItems(cachedSessions);
      } catch (e) {
        if (!cachedSessions) {
          listContainer.innerHTML = '<div class="p-2 text-center text-[10px] text-rose-400">계정 목록 불러오기 실패</div>';
        }
      }
    }


    async function handleSessionCardClick(index, isExpired, isActive) {
      if (isActive && !isExpired) {
        toggleChannelMenu();
        return;
      }
      if (isExpired) {
        showToast('⚠️ 세션 만료', 'warn', '네이버 로그인이 만료되었습니다. 다시 로그인해 주세요.');
        toggleChannelMenu();
        startWebviewLogin();
        return;
      }
      try {
        const res = await apiFetch('/switch-session', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ index })
        });
        if (res.ok) {
          cachedSessions = null; // 세션 변경 시 캐시 무효화
          userChannelId = "";
          currentChannelName = "";
          currentChannelAvatarUrl = "";
          showToast('계정 전환 완료', 'success', '선택한 계정으로 전환되었습니다.');
          toggleChannelMenu();
          await updateConnectionStatus();
          await loadCurrentSettings(true);
        } else {
          showToast('계정 전환 실패', 'error', '세션을 전환할 수 없습니다.');
        }
      } catch (e) {
        showToast('오류', 'error', e.message);
      }
    }


    async function removeAccountSession(index) {
      const ok = await showConfirmModal({
        title: '계정 연동 해제',
        message: '해당 치지직 계정 연동을 해제하시겠습니까?',
        submessage: '금고에서 세션이 삭제됩니다.',
        confirmText: '연동 해제',
        cancelText: '취소',
        danger: true,
        icon: '🗑️'
      });
      if (!ok) return;

      try {
        const res = await apiFetch('/remove-session', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ index })
        });
        if (res.ok) {
          cachedSessions = null; // 캐시 무효화
          showToast('계정 연동 해제', 'info', '계정이 삭제되었습니다.');
          await updateConnectionStatus();
          await loadCurrentSettings(true);
          renderChannelPopoverList();
        } else {
          showToast('삭제 실패', 'error', '계정 삭제에 실패했습니다.');
        }
      } catch (e) {
        showToast('오류', 'error', e.message);
      }
    }


    function toggleCookieAccordion() {
      const panel = document.getElementById('cookie-accordion-panel');
      const btn = document.getElementById('btn-cookie-accordion-toggle');
      if (!panel || !btn) return;
      const isOpen = !panel.classList.contains('hidden');
      if (isOpen) {
        panel.classList.add('hidden');
        btn.classList.remove('hidden');
      } else {
        panel.classList.remove('hidden');
        btn.classList.add('hidden');
      }
    }


    // 바깥 클릭 시 모든 플로팅 팝오버 닫기
    document.addEventListener('click', (e) => {
      // 1. 채널 팝오버
      const popoverChannel = document.getElementById('channel-popover-menu');
      const channelBtn = document.getElementById('header-channel-name');
      if (popoverChannel && !popoverChannel.classList.contains('hidden')) {
        if (!popoverChannel.contains(e.target) && !channelBtn.contains(e.target)) {
          popoverChannel.classList.add('hidden');
        }
      }

      // 2. 채팅모드 팝오버
      const popoverChatMode = document.getElementById('popover-chatmode');
      const btnChatMode = document.getElementById('btn-quick-chatmode');
      if (popoverChatMode && !popoverChatMode.classList.contains('hidden')) {
        if (!popoverChatMode.contains(e.target) && !btnChatMode.contains(e.target)) {
          popoverChatMode.classList.add('hidden');
        }
      }

      // 3. 후원 팝오버
      const popoverDonations = document.getElementById('popover-donations');
      const btnDonations = document.getElementById('btn-quick-donations');
      if (popoverDonations && !popoverDonations.classList.contains('hidden')) {
        if (!popoverDonations.contains(e.target) && !btnDonations.contains(e.target)) {
          popoverDonations.classList.add('hidden');
        }
      }
    });

    let lastLiveCheckTime = 0;

    async function manualCheckLiveStatus() {
      const now = Date.now();
      const elapsed = (now - lastLiveCheckTime) / 1000;
      if (elapsed < 3) {
        // 쿨타임 중에는 토스트 알림창 일절 띄우지 않고 무시
        return;
      }
      lastLiveCheckTime = now;

      const iconSvg = document.getElementById('header-live-svg');
      if (iconSvg) iconSvg.classList.add('animate-spin');

      try {
        await checkLiveStatus();
        const viewerMsg = isCurrentlyLive
          ? `현재 방송 중(ON) · 실시간 시청자 ${currentViewerCount.toLocaleString('ko-KR')}명 (쿨타임 3초)`
          : '현재 방송 꺼짐(OFF)입니다. (쿨타임 3초)';
        showToast('방송 상태 확인 완료', 'info', viewerMsg);
      } finally {
        if (iconSvg) iconSvg.classList.remove('animate-spin');
      }
    }


    function updatePanelScrollLock() {
      const hasOpenPanel = document.querySelector('.settings-panel.open') !== null;
      if (hasOpenPanel) {
        document.body.classList.add('panel-open');
      } else {
        document.body.classList.remove('panel-open');
      }
    }


    function setBroadcastFormLocked(locked) {
      const card = document.getElementById('card-broadcast');
      const titleInput = document.getElementById('live-title');
      const catInput = document.getElementById('category-search');
      if (titleInput) titleInput.readOnly = locked;
      if (catInput) catInput.readOnly = locked;
      if (card) {
        if (locked) {
          card.classList.add('opacity-75');
        } else {
          card.classList.remove('opacity-75');
        }
      }
    }


    // checkLoginGuard: 비로그인 상태에서 방송 제어 및 웹뷰 창 실행 요청을 UI 단에서 사전 차단
    // 로그아웃 상태에서는 안내 토스트만 표시하고 백엔드 웹뷰 실행 요청 자체를 전송하지 않습니다.
    function checkLoginGuard(actionName) {
      const hasCookies = (serverConfig.nid_aut && serverConfig.nid_ses);
      if (!userChannelId && !hasCookies) {
        showToast('🔒 네이버 로그인 필요', 'warn', (actionName ? actionName + '을(를) 사용하려면 ' : '') + '네이버 로그인이 필요합니다. [설정]에서 먼저 로그인해 주세요.');
        const btnSettings = document.getElementById('btn-settings');
        if (btnSettings) {
          btnSettings.classList.add('live-soft-glow');
          setTimeout(() => btnSettings.classList.remove('live-soft-glow'), 2500);
        }
        return false;
      }
      return true;
    }


    function toggleSettings() {
      closeAllPopovers();
      const urlsPanel = document.getElementById('urls-panel');
      if (urlsPanel && urlsPanel.classList.contains('open')) urlsPanel.classList.remove('open');
      const remotePanel = document.getElementById('remote-panel');
      if (remotePanel && remotePanel.classList.contains('open')) remotePanel.classList.remove('open');
      const panel = document.getElementById('settings-panel');
      if (panel) {
        panel.classList.toggle('open');
        if (panel.classList.contains('open')) {
          if (typeof checkObsScriptStatus === 'function') checkObsScriptStatus();
          if (typeof loadAllDockSettings === 'function') loadAllDockSettings();
        }
      }
      updatePanelScrollLock();
    }

    // ===== 설정창 내비게이션 탭 전환 (로그인 / 일반 / UI / 고급 / 앱 정보) =====

    function switchSettingsTab(tabName) {
      const tabs = ['login', 'general', 'ui', 'advanced', 'info'];
      tabs.forEach(t => {
        const btn = document.getElementById(`tab-btn-${t}`);
        const content = document.getElementById(`tab-content-${t}`);
        if (t === tabName) {
          if (btn) btn.classList.add('active');
          if (content) content.classList.remove('hidden');
        } else {
          if (btn) btn.classList.remove('active');
          if (content) content.classList.add('hidden');
        }
      });
    }


    function onReplayNoneClicked() {
      const elWp = document.getElementById('select-watch-party');
      if (elWp && elWp.value) {
        showToast('다시보기 고정 안내', 'warn', '같이보기 방송 중에는 규정에 따라 다시보기가 생성되지 않도록 고정되어 있습니다.');
        return;
      }
      showToast('다시보기 미생성 주의', 'warn', '생성 안 함 선택 시 방송 종료 후 다시보기가 저장되지 않으며 복구할 수 없습니다.');
    }


    async function reloadWatchPartiesOnly() {
      const icon = document.getElementById('wp-reload-icon');
      if (icon) icon.classList.add('animate-spin');
      try {
        if (!userChannelId) await resolveChannelId();
        if (!userChannelId) {
          showToast('로그인 필요', 'warn', '치지직 로그인을 먼저 확인해 주세요.');
          return;
        }
        const wpRes = await apiFetch('/chzzkdock/watch-parties');
        if (wpRes.ok) {
          const wpData = await wpRes.json();
          const list = wpData.watchParties || wpData.content?.watchParties || [];
          if (Array.isArray(list)) {
            availableWatchParties = list;
            renderWatchPartyOptions(availableWatchParties);
            const curVal = document.getElementById('select-watch-party')?.value;
            if (curVal) {
              const wp = availableWatchParties.find(p => String(p.watchPartyNo) === String(curVal));
              if (wp) applyWatchPartyLock(wp, false);
            }
            showToast('같이보기 목록 갱신', 'success', `${availableWatchParties.length}개의 방송이 조회되었습니다.`);
          }
        } else {
          showToast('같이보기 목록 갱신 실패', 'error');
        }
      } catch (e) {
        showToast('통신 오류', 'error', e.message);
      } finally {
        if (icon) setTimeout(() => icon.classList.remove('animate-spin'), 350);
      }
    }


    function renderWatchPartyOptions(parties) {
      const select = document.getElementById('select-watch-party');
      if (!select) return;
      const currentVal = select.value;
      select.innerHTML = '<option value="">사용 안 함 (일반 방송)</option>';
      if (Array.isArray(parties)) {
        parties.forEach(p => {
          const opt = document.createElement('option');
          opt.value = String(p.watchPartyNo);
          const catStr = p.category && p.category.categoryValue ? ` [${p.category.categoryValue}]` : '';
          const isSelectable = p.selectable !== false;
          if (!isSelectable) {
            opt.disabled = true;
            opt.textContent = `${p.watchPartyTitle}${catStr} (선택 불가)`;
          } else {
            opt.textContent = `${p.watchPartyTitle}${catStr}`;
          }
          select.appendChild(opt);
        });
      }
      if (currentVal) {
        select.value = currentVal;
      }
    }

    let currentDonationBoardMessage = '';
    let preWatchPartySnapshot = null;


    function capturePreWatchPartySnapshot() {
      const replayChecked = document.querySelector('input[name="replayPublishType"]:checked')?.value || 'MANUAL';
      return {
        categoryId: document.getElementById('category-id')?.value || '',
        categoryType: document.getElementById('category-type')?.value || 'ETC',
        categorySearch: document.getElementById('category-search')?.value || '',
        adult: document.getElementById('toggle-adult')?.checked ?? false,
        clip: document.getElementById('toggle-clip')?.checked ?? true,
        overseas: document.getElementById('toggle-overseas')?.checked ?? true,
        replay: replayChecked,
        chatDonation: document.getElementById('toggle-chat-donation')?.checked ?? false
      };
    }


    function openWatchPartyRestartModal() {
      showNoticeModal({
        icon: '🔄',
        title: '같이보기 송출 재정비 안내',
        message: '이 같이보기는 원활한 방송을 위해 자동으로 송출 장비를 재설정합니다.\n\n• 송출 프로그램(OBS/Prism 등)의 [설정 > 고급 > 자동으로 다시 연결] 옵션을 반드시 켜주세요.\n• 같이보기 on/off 시 송출이 잠시 끊겼다가 자동 재시작되며 안내 화면이 노출됩니다.\n• 채팅과 후원 참여 내역은 모두 유지됩니다.\n• 다시보기는 분할된 영상으로 저장됩니다.'
      });
    }


    function openWatchPartyMidRollModal() {
      showNoticeModal({
        icon: '📢',
        title: '안내',
        message: '선택하신 같이보기는 중간광고를 직접 제어할 수 없으며, 콘텐츠 흐름에 따라 적절한 시점에 중간광고가 재생됩니다.'
      });
    }


    function onBoardMessageInput(val) {
      currentDonationBoardMessage = (val || '').slice(0, 25);
      const preview = document.getElementById('preview-board-banner');
      const count = document.getElementById('count-board-message');
      if (count) count.textContent = `${currentDonationBoardMessage.length}/25`;
      if (preview) {
        preview.textContent = currentDonationBoardMessage || '원하는 대로 전광판 메시지를 입력해보세요!';
        preview.className = currentDonationBoardMessage
          ? 'text-xs font-bold text-[#00FFA3] tracking-wide break-all min-h-[18px]'
          : 'text-xs font-bold text-slate-300 tracking-wide break-all min-h-[18px]';
      }
    }


    function setBoardMessagePreset(text) {
      const input = document.getElementById('input-board-message');
      if (input) {
        input.value = text;
        onBoardMessageInput(text);
      }
    }


    function openDonationBoardModal() {
      if (!activeWatchParty || !activeWatchParty.donationBoardInfo) return;
      const modal = document.getElementById('modal-donation-board');
      const input = document.getElementById('input-board-message');
      const presetsBox = document.getElementById('board-preset-chips');

      if (input) {
        input.value = currentDonationBoardMessage;
        onBoardMessageInput(currentDonationBoardMessage);
      }

      if (presetsBox) {
        const msgs = activeWatchParty.donationBoardInfo.defaultMessages || [];
        presetsBox.innerHTML = msgs.map(m => `
          <button type="button" onclick="setBoardMessagePreset('${escapeHtml(m.text)}')" class="px-2 py-1 rounded bg-[#161F2E] hover:bg-[#1E2738] border border-[#1E2738] text-[11px] text-slate-200 hover:text-[#00FFA3] cursor-pointer transition-colors text-left truncate max-w-full">
            ${escapeHtml(m.text)}
          </button>
        `).join('');
      }

      if (modal) modal.classList.remove('hidden');
    }


    function closeDonationBoardModal() {
      const modal = document.getElementById('modal-donation-board');
      if (modal) modal.classList.add('hidden');
      checkBroadcastStateDirty();
    }


    function saveDonationBoardNow() {
      closeDonationBoardModal();
      saveAll();
    }



    function applyWatchPartyLock(wp, showNotify = true) {
      activeWatchParty = wp;
      const isRs = !!(wp && wp.watchPartyType === 'RS');
      const badge = document.getElementById('watch-party-badge');
      if (badge) {
        badge.textContent = isRs ? '🎬 같이보기+ 방송' : '🎬 같이보기 방송';
        badge.className = 'text-[9.5px] px-1.5 py-0.5 rounded font-bold text-[#00FFA3] bg-[#00FFA3]/10 border border-[#00FFA3]/30 leading-none';
      }
      const notice = document.getElementById('watch-party-notice');
      if (notice) notice.style.display = 'flex';

      // 1. 카테고리 고정 (캠페인에 지정된 카테고리가 있는 경우 고정, 없으면 기본값 talk 설정)
      const elCatSearch = document.getElementById('category-search');
      const elCatId = document.getElementById('category-id');
      const elCatType = document.getElementById('category-type');
      const elCatClear = document.getElementById('category-clear');
      const elCatWrapper = document.getElementById('category-wrapper');
      const lockCat = document.getElementById('lock-category');

      if (wp && wp.category && (wp.category.categoryId || wp.category.categoryValue)) {
        if (elCatId) elCatId.value = wp.category.categoryId || '';
        if (elCatType) elCatType.value = wp.category.categoryType || 'ENTERTAINMENT';
        if (elCatSearch) elCatSearch.value = wp.category.categoryValue || wp.category.categoryId || '';
        if (elCatSearch) elCatSearch.disabled = true;
        if (elCatClear) elCatClear.style.display = 'none';
        if (elCatWrapper) elCatWrapper.classList.add('locked-by-party');
        if (lockCat) lockCat.style.display = 'inline';
      } else {
        // 강제되는 카테고리가 없는 경우 기본값 talk 적용 (수정 가능)
        if (showNotify || !elCatId || !elCatId.value) {
          if (elCatId) elCatId.value = 'talk';
          if (elCatType) elCatType.value = 'ETC';
          if (elCatSearch) elCatSearch.value = 'talk';
        }
        if (elCatSearch) elCatSearch.disabled = false;
        if (elCatWrapper) elCatWrapper.classList.remove('locked-by-party');
        if (lockCat) lockCat.style.display = 'none';
        if (elCatClear) elCatClear.style.display = (elCatId && elCatId.value) ? 'block' : 'none';
      }

      // 2. 태그 영역: 같이보기 지정 태그 표시 갱신
      renderTags();

      // 3. 해외 시청 (캠페인 krOnlyViewing 규정으로 100% 강제 동기화 및 잠금)
      const elOverseas = document.getElementById('toggle-overseas');
      const labelOverseas = document.getElementById('label-overseas');
      const lockOverseas = document.getElementById('lock-overseas');
      if (wp && wp.krOnlyViewing !== undefined) {
        const isKrOnly = !!wp.krOnlyViewing;
        if (elOverseas) {
          elOverseas.checked = !isKrOnly;
          elOverseas.disabled = true;
          if (isKrOnly) {
            elOverseas.classList.add('locked-x');
          } else {
            elOverseas.classList.remove('locked-x');
          }
        }
        if (labelOverseas) {
          labelOverseas.classList.add('locked-by-party');
          labelOverseas.classList.toggle('locked-label-x', isKrOnly);
        }
        if (lockOverseas) {
          if (isKrOnly) {
            lockOverseas.innerHTML = '<span class="text-rose-400 font-bold text-[10.5px] leading-none" title="같이보기 규정으로 비활성화됨">✕</span>';
          } else {
            lockOverseas.textContent = '🔒';
          }
          lockOverseas.style.display = 'inline';
        }
      }

      // 4. 클립 생성 허용 (clipActive가 false인 경우에만 잠금)
      const elClip = document.getElementById('toggle-clip');
      const labelClip = document.getElementById('label-clip');
      const lockClip = document.getElementById('lock-clip');
      const isClipLocked = !!(wp && wp.clipActive === false);
      if (isClipLocked) {
        if (elClip) {
          elClip.checked = false;
          elClip.disabled = true;
          elClip.classList.add('locked-x');
        }
        if (labelClip) {
          labelClip.classList.add('locked-by-party');
          labelClip.classList.add('locked-label-x');
        }
        if (lockClip) {
          lockClip.innerHTML = '<span class="text-rose-400 font-bold text-[10.5px] leading-none" title="같이보기 규정으로 비활성화됨">✕</span>';
          lockClip.style.display = 'inline';
        }
      } else {
        if (elClip) {
          elClip.disabled = false;
          elClip.classList.remove('locked-x');
        }
        if (labelClip) {
          labelClip.classList.remove('locked-by-party', 'locked-label-x');
        }
        if (lockClip) {
          lockClip.textContent = '🔒';
          lockClip.style.display = 'none';
        }
      }

      // 5. 다시보기 (allowReplayVideo가 false인 경우에만 '생성 안 함' 강제 잠금)
      const radioNone = document.querySelector('input[name="replayPublishType"][value="NONE"]');
      const radioVerify = document.querySelector('input[name="replayPublishType"][value="VERIFY_AFTER"]');
      const radioAuto = document.querySelector('input[name="replayPublishType"][value="AUTO_PUBLISH"]');
      const containerReplay = document.getElementById('container-replay-radios');
      const isReplayLocked = !!(wp && wp.allowReplayVideo === false);
      if (isReplayLocked) {
        if (radioVerify) radioVerify.checked = false;
        if (radioAuto) radioAuto.checked = false;
        if (radioNone) radioNone.checked = true;
        document.querySelectorAll('input[name="replayPublishType"]').forEach(r => r.disabled = true);
        if (containerReplay) containerReplay.classList.add('locked-by-party');
        document.querySelectorAll('.lock-replay-disabled').forEach(el => {
          el.innerHTML = '<span class="text-rose-400 font-bold text-[10px] leading-none" title="같이보기 방송 규정으로 선택 불가">✕</span>';
          el.style.display = 'inline';
        });
      } else {
        document.querySelectorAll('input[name="replayPublishType"]').forEach(r => r.disabled = false);
        if (containerReplay) containerReplay.classList.remove('locked-by-party');
        document.querySelectorAll('.lock-replay-disabled').forEach(el => {
          el.textContent = '🔒';
          el.style.display = 'none';
        });
      }

      // 6. 연령 제한 (성인 콘텐츠 규정 동기화 - 성인 방송 고정은 활성화 고정이므로 자물쇠)
      const elAdult = document.getElementById('toggle-adult');
      const labelAdult = document.getElementById('label-adult');
      const lockAdult = document.getElementById('lock-adult');
      const isAdultLocked = !!(wp && wp.adult);
      if (isAdultLocked) {
        if (elAdult) {
          elAdult.checked = true;
          elAdult.disabled = true;
          elAdult.classList.remove('locked-x');
        }
        if (labelAdult) labelAdult.classList.add('locked-by-party');
        if (lockAdult) {
          lockAdult.textContent = '🔒';
          lockAdult.style.display = 'inline';
        }
      } else {
        if (elAdult) {
          elAdult.disabled = false;
          elAdult.classList.remove('locked-x');
        }
        if (labelAdult) labelAdult.classList.remove('locked-by-party');
        if (lockAdult) lockAdult.style.display = 'none';
      }

      // 7. 같이보기+(RS)인 경우 채팅 후원 필수 자동 ON
      if (isRs) {
        const elChatDonation = document.getElementById('toggle-chat-donation');
        if (elChatDonation && !elChatDonation.checked) {
          elChatDonation.checked = true;
          if (typeof updateDonationDetailButtonsState === 'function') {
            updateDonationDetailButtonsState();
          }
        }
      }

      // 8. 응원 전광판 버튼 노출 및 초기화
      const btnBoard = document.getElementById('btn-open-donation-board');
      if (btnBoard) {
        btnBoard.style.display = (wp && wp.donationBoardInfo) ? 'inline-flex' : 'none';
      }
      if (wp && wp.donationBoardInfo) {
        currentDonationBoardMessage = wp.donationBoardInfo.channelDonationBoardMessage || '';
      } else {
        currentDonationBoardMessage = '';
      }

      // 안내 문구 생성
      const lockedItems = [];
      if (wp && wp.category && (wp.category.categoryId || wp.category.categoryValue)) lockedItems.push('카테고리');
      lockedItems.push(wp && wp.krOnlyViewing ? '한국 전용' : '해외 시청 허용');
      if (isClipLocked) lockedItems.push('클립 OFF');
      if (isReplayLocked) lockedItems.push('다시보기 OFF');
      if (isAdultLocked) lockedItems.push('연령 제한(19세)');
      if (isRs) lockedItems.push('채팅 후원 필수(ON)');

      const noticeText = document.getElementById('watch-party-notice-text');
      if (noticeText) {
        const fullNotice = `같이보기 규정에 따라 ${lockedItems.join(', ')} 설정이 고정/적용됩니다.`;
        noticeText.textContent = fullNotice;
        noticeText.title = fullNotice;
      }

      if (showNotify && wp) {
        const title = wp.watchPartyTitle || '같이보기';
        const lockSummary = lockedItems.length > 0 ? ` (${lockedItems.join(', ')})` : '';
        showToast('같이보기 방송 설정', 'info', `"${title}" 규정에 맞춰 설정이 조정되었습니다.${lockSummary}`);
      }
    }


    function releaseWatchPartyLock(showNotify = true) {
      activeWatchParty = null;
      currentDonationBoardMessage = '';
      const badge = document.getElementById('watch-party-badge');
      const notice = document.getElementById('watch-party-notice');
      if (badge) {
        badge.textContent = '일반 방송';
        badge.className = 'text-[9.5px] px-1.5 py-0.5 rounded font-medium text-slate-400 bg-slate-800/80 border border-slate-700/50 leading-none';
      }
      if (notice) notice.style.display = 'none';
      const btnBoard = document.getElementById('btn-open-donation-board');
      if (btnBoard) btnBoard.style.display = 'none';

      // 1. 태그 렌더링 갱신 (지정 태그 표시 제거)
      renderTags();

      // 2. 카테고리 복원 및 잠금 해제
      const elCatSearch = document.getElementById('category-search');
      const elCatId = document.getElementById('category-id');
      const elCatType = document.getElementById('category-type');
      const elCatClear = document.getElementById('category-clear');
      const elCatWrapper = document.getElementById('category-wrapper');
      const lockCat = document.getElementById('lock-category');
      if (preWatchPartySnapshot) {
        if (elCatId) elCatId.value = preWatchPartySnapshot.categoryId || '';
        if (elCatType) elCatType.value = preWatchPartySnapshot.categoryType || 'ETC';
        if (elCatSearch) elCatSearch.value = preWatchPartySnapshot.categorySearch || preWatchPartySnapshot.categoryId || '';
      }
      if (elCatSearch) elCatSearch.disabled = false;
      if (elCatClear) elCatClear.style.display = (elCatId && elCatId.value) ? 'block' : 'none';
      if (elCatWrapper) elCatWrapper.classList.remove('locked-by-party');
      if (lockCat) lockCat.style.display = 'none';

      // 3. 해외 시청 복원 및 잠금 해제
      const elOverseas = document.getElementById('toggle-overseas');
      const labelOverseas = document.getElementById('label-overseas');
      const lockOverseas = document.getElementById('lock-overseas');
      if (elOverseas) {
        if (preWatchPartySnapshot) {
          elOverseas.checked = preWatchPartySnapshot.overseas;
        }
        elOverseas.disabled = false;
        elOverseas.classList.remove('locked-x');
      }
      if (labelOverseas) labelOverseas.classList.remove('locked-by-party', 'locked-off', 'locked-label-x');
      if (lockOverseas) {
        lockOverseas.textContent = '🔒';
        lockOverseas.style.display = 'none';
      }

      // 4. 클립 허용 복원 및 잠금 해제
      const elClip = document.getElementById('toggle-clip');
      const labelClip = document.getElementById('label-clip');
      const lockClip = document.getElementById('lock-clip');
      if (elClip) {
        if (preWatchPartySnapshot) {
          elClip.checked = preWatchPartySnapshot.clip;
        }
        elClip.disabled = false;
        elClip.classList.remove('locked-x');
      }
      if (labelClip) labelClip.classList.remove('locked-by-party', 'locked-off', 'locked-label-x');
      if (lockClip) {
        lockClip.textContent = '🔒';
        lockClip.style.display = 'none';
      }

      // 5. 다시보기 복원 및 잠금 해제
      const containerReplay = document.getElementById('container-replay-radios');
      if (preWatchPartySnapshot) {
        const prevRadio = document.querySelector(`input[name="replayPublishType"][value="${preWatchPartySnapshot.replay}"]`);
        if (prevRadio) prevRadio.checked = true;
      }
      document.querySelectorAll('input[name="replayPublishType"]').forEach(r => r.disabled = false);
      if (containerReplay) containerReplay.classList.remove('locked-by-party');
      document.querySelectorAll('.lock-replay-disabled').forEach(el => {
        el.textContent = '🔒';
        el.style.display = 'none';
      });

      // 6. 연령 제한 복원 및 잠금 해제
      const elAdult = document.getElementById('toggle-adult');
      const labelAdult = document.getElementById('label-adult');
      const lockAdult = document.getElementById('lock-adult');
      if (elAdult) {
        if (preWatchPartySnapshot) {
          elAdult.checked = preWatchPartySnapshot.adult;
        }
        elAdult.disabled = false;
        elAdult.classList.remove('locked-x');
      }
      if (labelAdult) labelAdult.classList.remove('locked-by-party', 'locked-off', 'locked-label-x');
      if (lockAdult) {
        lockAdult.textContent = '🔒';
        lockAdult.style.display = 'none';
      }

      // 7. 채팅 후원 복원
      if (preWatchPartySnapshot && preWatchPartySnapshot.chatDonation !== undefined) {
        const elChatDonation = document.getElementById('toggle-chat-donation');
        if (elChatDonation) {
          elChatDonation.checked = preWatchPartySnapshot.chatDonation;
          if (typeof updateDonationDetailButtonsState === 'function') {
            updateDonationDetailButtonsState();
          }
        }
      }

      preWatchPartySnapshot = null;

      if (showNotify) {
        showToast('일반 방송 전환', 'info', '카테고리 및 방송 옵션이 같이보기 전의 원래 상태로 복원되었습니다.');
      }
    }


    function onWatchPartyChange(val) {
      if (!val) {
        releaseWatchPartyLock(true);
      } else {
        // 같이보기 첫 진입 시 직전 방송 설정을 스냅샷으로 보존 (해제 시 원복용)
        if (!preWatchPartySnapshot) {
          preWatchPartySnapshot = capturePreWatchPartySnapshot();
        }
        const wp = availableWatchParties.find(p => String(p.watchPartyNo) === String(val));
        if (wp) {
          applyWatchPartyLock(wp, true);
          if (wp.hasLiveRestartTrigger) {
            openWatchPartyRestartModal();
          }
          if (wp.midRollAdIntegrationControl) {
            openWatchPartyMidRollModal();
          }
        }
      }
      checkBroadcastStateDirty();
    }

    // ===== App Initialization (UI 즉시 활성화 + 백그라운드 연결) =====
    const INIT_MAX_RETRIES = 5;
    const INIT_RETRY_DELAY_MS = 1500;


    function startApp() {
      // ── 0. 초기 미로그인 상태 설정 (연동 확인 전까지 방송 폼 잠금) ──
      setBroadcastFormLocked(true);

      // ── 1. 이벤트 리스너 즉시 등록 (UI 반응성 확보) ──
      const titleInput = document.getElementById('live-title');
      const titleClearBtn = document.getElementById('title-clear');
      if (titleInput) {
        titleInput.addEventListener('input', () => {
          document.getElementById('title-count').textContent = `${titleInput.value.length}/50`;
          if (titleClearBtn) {
            titleClearBtn.style.display = titleInput.value.length > 0 ? 'block' : 'none';
          }
          checkBroadcastStateDirty();
        });
      }

      const portInput = document.getElementById('input-custom-port');
      if (portInput) {
        portInput.addEventListener('input', () => {
          updatePortClearButton();
        });
        portInput.addEventListener('blur', () => {
          if (!portInput.value.trim()) {
            portInput.value = currentActivePort || currentConfiguredPort || 8081;
            updatePortClearButton();
          }
        });
      }

      const catSearch = document.getElementById('category-search');
      if (catSearch) {
        let selectedCatIdx = -1;

        catSearch.addEventListener('input', (e) => {
          clearTimeout(categorySearchTimeout);
          selectedCatIdx = -1;
          const query = e.target.value.trim();
          setCategoryErrorState(false);
          const clearBtn = document.getElementById('category-clear');
          if (clearBtn) clearBtn.style.display = query ? 'block' : 'none';
          if (query.length < 1) {
            renderFavoritesDropdown();
            return;
          }
          categorySearchTimeout = setTimeout(() => searchCategories(query), 300);
        });

        catSearch.addEventListener('focus', () => {
          const query = catSearch.value.trim();
          selectedCatIdx = -1;
          if (query.length >= 1) {
            searchCategories(query);
          } else {
            renderFavoritesDropdown();
          }
        });

        catSearch.addEventListener('blur', () => {
          // 마우스 클릭 이벤트가 먼저 완료될 수 있도록 지연 후 자동 확정
          setTimeout(() => {
            const dropdown = document.getElementById('category-dropdown');
            if (dropdown && dropdown.style.display !== 'none') {
              autoConfirmOrValidateCategory();
              dropdown.style.display = 'none';
            } else {
              validateCategoryInput();
            }
          }, 200);
        });

        catSearch.addEventListener('keydown', (e) => {
          const dropdown = document.getElementById('category-dropdown');
          if (!dropdown || dropdown.style.display === 'none') return;
          const items = dropdown.querySelectorAll('.category-item');
          if (!items.length) return;

          if (e.key === 'ArrowDown') {
            e.preventDefault();
            selectedCatIdx = (selectedCatIdx + 1) % items.length;
            items.forEach((it, idx) => it.classList.toggle('highlighted', idx === selectedCatIdx));
            items[selectedCatIdx]?.scrollIntoView({ block: 'nearest' });
          } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            selectedCatIdx = (selectedCatIdx - 1 + items.length) % items.length;
            items.forEach((it, idx) => it.classList.toggle('highlighted', idx === selectedCatIdx));
            items[selectedCatIdx]?.scrollIntoView({ block: 'nearest' });
          } else if (e.key === 'Enter') {
            e.preventDefault();
            if (selectedCatIdx >= 0 && selectedCatIdx < items.length) {
              selectCategoryItem(items[selectedCatIdx]);
            } else if (items.length > 0) {
              // 화살표로 선택하지 않았더라도 첫 번째 일치 항목 자동 선택
              selectCategoryItem(items[0]);
            }
            selectedCatIdx = -1;
          }
        });
      }

      document.addEventListener('click', (e) => {
        const catWrapper = document.getElementById('category-wrapper');
        if (catWrapper && !catWrapper.contains(e.target)) {
          const drop = document.getElementById('category-dropdown');
          if (drop && drop.style.display !== 'none') {
            autoConfirmOrValidateCategory();
            drop.style.display = 'none';
          }
        }
      });


      // 방송 설정 변경 감지 리스너 일괄 등록
      document.getElementById('toggle-adult')?.addEventListener('change', checkBroadcastStateDirty);
      document.getElementById('toggle-clip')?.addEventListener('change', checkBroadcastStateDirty);
      document.getElementById('toggle-overseas')?.addEventListener('change', checkBroadcastStateDirty);
      document.getElementById('toggle-paid-promotion')?.addEventListener('change', checkBroadcastStateDirty);
      document.querySelectorAll('input[name="replayPublishType"]').forEach(el => {
        el.addEventListener('change', () => {
          if (el.value === 'NONE' && el.checked) {
            onReplayNoneClicked();
          }
          checkBroadcastStateDirty();
        });
      });
      document.getElementById('select-watch-party')?.addEventListener('change', (e) => {
        onWatchPartyChange(e.target.value);
      });

      // 태그 인라인 입력 이벤트 리스너 (한글 IME 조합 및 좌표 이탈 완벽 대응)
      const tagInput = document.getElementById('tag-inline-input');
      if (tagInput) {
        let isTagComposing = false;
        tagInput.addEventListener('compositionstart', () => { isTagComposing = true; });
        tagInput.addEventListener('compositionend', () => { isTagComposing = false; });
        tagInput.addEventListener('keydown', (e) => {
          if (e.isComposing || e.keyCode === 229 || isTagComposing) {
            return;
          }
          if (e.key === 'Enter') {
            e.preventDefault();
            confirmTagInput();
          }
        });
        tagInput.addEventListener('keyup', (e) => {
          if (e.key === 'Enter' && !isTagComposing) {
            if (tagInput.value.trim().length > 0) {
              confirmTagInput();
            }
          }
        });
      }

      // ── 2. 백그라운드에서 연결 시도 및 통합 설정 1회 로드 (원리 A) ──
      initConnection();
      if (typeof loadAllDockSettings === 'function') {
        loadAllDockSettings();
      }
      checkObsScriptStatus();
      loadBroadcastPresets();
      loadCategoryFavorites();
      checkForUpdate(false);
    }

// [startApp moved to EOF]


    // ===== OBS 자동 종료 대기 시간 설정 제어 =====

    function manualCheckForUpdate() {
      checkForUpdate(true);
    }


    function openUpdateModal(forceProgress = false) {
      if (!cachedUpdateInfo) return;
      const modal = document.getElementById('modal-updater');
      const promptView = document.getElementById('update-prompt-view');
      const progressView = document.getElementById('update-progress-view');
      const latestVerEl = document.getElementById('update-latest-ver');
      const currentVerEl = document.getElementById('update-current-ver');

      if (latestVerEl) latestVerEl.textContent = cachedUpdateInfo.latest_version || '';
      if (currentVerEl) currentVerEl.textContent = cachedUpdateInfo.current_version || '';

      const installBtnLabel = document.getElementById('btn-start-update-label');
      if (installBtnLabel) {
        installBtnLabel.textContent = cachedUpdateInfo.already_downloaded ? '즉시 설치 (다운로드 완료됨)' : '업데이트 설치';
      }

      if (forceProgress) {
        if (promptView) promptView.classList.add('hidden');
        if (progressView) progressView.classList.remove('hidden');
      } else {
        if (promptView) promptView.classList.remove('hidden');
        if (progressView) progressView.classList.add('hidden');
      }

      if (modal) modal.classList.remove('hidden');
    }


    function closeUpdateModal() {
      if (updatePollTimer) clearInterval(updatePollTimer);
      const modal = document.getElementById('modal-updater');
      if (modal) modal.classList.add('hidden');
    }


    function openReleaseNotesLink() {
      const url = cachedUpdateInfo?.release_url || 'https://github.com/MK0522/chzzk-dock/releases';
      openExternalUrl(url, 'GitHub 릴리즈');
    }


    async function handleLogout() {
      const ok = await showConfirmModal({
        title: '치지직 연동 해제',
        message: '치지직 계정 연동을 해제(로그아웃)하시겠습니까?',
        submessage: '저장된 로그인 쿠키가 삭제되며, 방송 정보 자동 연동이 중단됩니다.',
        confirmText: '연동 해제',
        cancelText: '취소',
        danger: true,
        icon: '🚪'
      });
      if (!ok) return;
      try {
        const res = await apiFetch('/logout', { method: 'POST' });
        if (res.ok) {
          serverConfig = { nid_aut: '', nid_ses: '' };
          userChannelId = '';
          currentChannelName = '';
          streamingInfoLoaded = false;
          donationsLoaded = false;
          const inputAut = document.getElementById('cookie-nid-aut');
          const inputSes = document.getElementById('cookie-nid-ses');
          const elChannelName = document.getElementById('header-channel-name');
          if (inputAut) inputAut.value = '';
          if (inputSes) inputSes.value = '';
          if (elChannelName) elChannelName.style.display = 'none';
          handleSessionExpired();
          showToast('로그아웃 완료', 'info', '치지직 연동이 해제되었습니다. 새로운 계정으로 로그인할 수 있습니다.');
        } else {
          showToast('로그아웃 실패', 'error', '서버에서 로그아웃 처리에 실패했습니다.');
        }
      } catch (e) {
        showToast('로그아웃 오류', 'error', e.message);
      }
    }

    


    async function startWebviewLogin() {
      if (isLoggingIn) return;
      isLoggingIn = true;

      const btnWebview = document.getElementById('btn-webview-login');
      if (btnWebview) {
        btnWebview.disabled = true;
        btnWebview.classList.add('opacity-50', 'pointer-events-none');
      }

      try {
        const res = await apiFetch('/login-webview');
        const data = await res.json();
        if (data.status === 'started' || data.status === 'already_open') {
          showToast('네이버 로그인 창이 열렸습니다. 로그인해 주세요.', 'info');

          // [이벤트 기반] 단일 대기
          const waitRes = await apiFetch('/login-wait');
          const waitData = await waitRes.json();

          // 세션 완료 여부 다중 체크 (status === 'completed' 또는 쿠키 직접 보유)
          const hasCookies = Boolean(waitData.config && waitData.config.nid_aut && waitData.config.nid_ses);

          if (waitData.status === 'completed' || hasCookies) {
            serverConfig = waitData.config || (await (await apiFetch('/config')).json());
            const cid = await updateConnectionStatus();
            if (cid) {
              showToast('🎉 치지직 로그인 및 방송 연동 완료!', 'success');
              const panel = document.getElementById('settings-panel');
              if (panel && panel.classList.contains('open')) {
                toggleSettings();
              }
              await loadCurrentSettings(true);
            } else {
              showToast('⚠️ 연동 확인 실패', 'warn', '쿠키는 저장되었으나 치지직 채널 정보 확인에 실패했습니다. 다시 [로그인] 버튼을 눌러주세요.');
            }
          } else {
            // [자가 치유]: 창이 닫혔더라도 금고에 저장되었는지 최종 재확인
            await loadServerConfig();
            if (serverConfig.nid_aut && serverConfig.nid_ses) {
              const cid = await updateConnectionStatus();
              if (cid) {
                showToast('🎉 치지직 로그인 자동 복구 완료!', 'success');
                const panel = document.getElementById('settings-panel');
                if (panel && panel.classList.contains('open')) {
                  toggleSettings();
                }
                await loadCurrentSettings(true);
                return;
              }
            }
            showToast('⚠️ 로그인 미완료', 'warn', '로그인이 완료되지 않고 창이 닫혔습니다. 연동을 위해 [네이버 로그인] 버튼을 다시 눌러주세요.');
          }
        } else {
          showToast('웹뷰 로그인 알림', 'info', data.message || '로그인 창을 열 수 없습니다. 다시 시도해 주세요.');
        }
      } catch (e) {
        // 네트워크/통신 에러 시 자가 치유 시도 후 명확한 안내 토스트
        await loadServerConfig();
        if (serverConfig.nid_aut && serverConfig.nid_ses) {
          const cid = await updateConnectionStatus();
          if (cid) {
            showToast('🎉 치지직 로그인 자동 복구 완료!', 'success');
            await loadCurrentSettings(true);
            return;
          }
        }
        showToast('⚠️ 로그인 연결 오류', 'error', '통신 중 오류가 발생했습니다. [로그인] 버튼을 다시 눌러주세요.');
      } finally {
        setTimeout(() => {
          isLoggingIn = false;
          if (btnWebview) {
            btnWebview.disabled = false;
            btnWebview.classList.remove('opacity-50', 'pointer-events-none');
          }
        }, 500);
      }
    }

    // [자가 치유 & 자동 동기화] 미연동 상태에서 독 창 복귀/가시 전환 시 자동 세션 확인
    

    async function trySelfHeal(source) {
      if (userChannelId || isLoggingIn || selfHealRunning) return;
      selfHealRunning = true;
      try {
        await loadServerConfig();
        if (serverConfig.nid_aut && serverConfig.nid_ses) {
          const cid = await updateConnectionStatus();
          if (cid) {
            showToast('🎉 치지직 로그인 자동 연결 완료!', 'success');
            await loadCurrentSettings(true);
          }
        }
      } finally {
        selfHealRunning = false;
      }
    }

    window.addEventListener('focus', () => trySelfHeal('focus'));

    // OBS 브라우저 독은 focus 이벤트가 발생하지 않을 수 있으므로 visibilitychange도 감시
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden) trySelfHeal('visibility');
    });


    function toggleCookieHelp() {
      const box = document.getElementById('cookie-help-box');
      if (box) {
        box.classList.toggle('hidden');
      }
    }


    async function saveManualCookies() {
      const nidAut = document.getElementById('cookie-nid-aut').value.trim();
      const nidSes = document.getElementById('cookie-nid-ses').value.trim();

      if (!nidAut || !nidSes) {
        showToast('입력 오류', 'error', 'NID_AUT와 NID_SES를 모두 입력해 주세요.');
        return;
      }

      // 더미 마스킹 값이 그대로 제출된 경우 기존 쿠키 설정 유지
      if (nidAut.includes('•') || nidAut.includes('*') || nidSes.includes('•') || nidSes.includes('*')) {
        showToast('쿠키 안내', 'info', '기존에 안전하게 저장된 쿠키 설정이 유지됩니다.');
        toggleSettings();
        return;
      }

      try {
        const res = await apiFetch('/save-config', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ nid_aut: nidAut, nid_ses: nidSes })
        });
        if (res.ok) {
          serverConfig.nid_aut = nidAut;
          serverConfig.nid_ses = nidSes;
          const cid = await updateConnectionStatus();
          showToast('네이버 로그인 쿠키 저장 완료!', 'success');
          toggleSettings();
          if (cid) await loadCurrentSettings();
        } else {
          showToast('쿠키 저장 실패', 'error');
        }
      } catch (e) {
        showToast('서버 통신 오류', 'error', e.message);
      }
    }

    // ===== Main Settings Operations =====

    async function loadCurrentSettings(silent = false) {
      const loadSpinner = document.getElementById('load-spinner');
      if (loadSpinner) loadSpinner.classList.add('animate-spin');

      try {
        if (!userChannelId) {
          await resolveChannelId();
        }
        if (!userChannelId) {
          showToast('로그인 설정 필요', 'warn', '⚙ 상단 로그인 설정 버튼을 눌러 연동을 진행하세요.');
          return;
        }

        let loaded = false;

        // 0. 같이보기(Watch Party) 목록 조회 (BFF)
        try {
          const wpRes = await apiFetch('/chzzkdock/watch-parties');
          if (wpRes.ok) {
            const wpData = await wpRes.json();
            const list = wpData.watchParties || wpData.content?.watchParties || [];
            if (Array.isArray(list)) {
              availableWatchParties = list;
              renderWatchPartyOptions(availableWatchParties);
            }
          }
        } catch (e) {}

        // 0-1. 치지직 스튜디오 공식 API: 채널 실시간 활성 드롭스 캠페인 목록 조회
        try {
          await fetchChannelDropsCampaigns(userChannelId);
        } catch (e) {}

        // 1. 방송 정보 및 다시보기/연령제한/클립 설정 (BFF)
        try {
          const res = await chzzkApi.getBroadcast();
          if (res.ok) {
            const data = await res.json();
            const content = data.content || data;
            if (content) {
              currentLiveSetting.rawContent = content;
              const liveTitle = content.defaultLiveTitle || '';
              document.getElementById('live-title').value = liveTitle;
              document.getElementById('title-count').textContent = `${liveTitle.length}/50`;
              const titleClearBtn = document.getElementById('title-clear');
              if (titleClearBtn) titleClearBtn.style.display = liveTitle ? 'block' : 'none';

              const cat = content.category || {};
              const catId = cat.categoryId || content.liveCategory || '';
              const catType = cat.categoryType || content.categoryType || 'ETC';
              const catValue = cat.categoryValue || catId;
              document.getElementById('category-id').value = catId;
              document.getElementById('category-type').value = catType;
              const elCatSearch = document.getElementById('category-search');
              if (elCatSearch) {
                elCatSearch.value = catValue;
                elCatSearch.dataset.confirmedValue = catValue;
                elCatSearch.dataset.confirmedId = catId;
              }
              setCategoryErrorState(false);
              const catClearBtn = document.getElementById('category-clear');
              if (catClearBtn) catClearBtn.style.display = (catId || catValue) ? 'block' : 'none';
              updateCategoryFavoriteToggleUI();

              currentTags = (content.tags && Array.isArray(content.tags)) ? [...content.tags] : [];
              renderTags();

              if (content.adult !== undefined) currentLiveSetting.adult = !!content.adult;
              if (content.clipActive !== undefined) currentLiveSetting.clipActive = !!content.clipActive;
              if (content.paidPromotion !== undefined) currentLiveSetting.paidPromotion = !!content.paidPromotion;
              if (content.replayPublishType) currentLiveSetting.replayPublishType = content.replayPublishType;
              if (content.krOnlyViewing !== undefined) currentLiveSetting.krOnlyViewing = !!content.krOnlyViewing;

              document.getElementById('toggle-adult').checked = currentLiveSetting.adult;
              document.getElementById('toggle-clip').checked = currentLiveSetting.clipActive;
              const elPaid = document.getElementById('toggle-paid-promotion');
              if (elPaid) elPaid.checked = currentLiveSetting.paidPromotion;
              const elOverseas = document.getElementById('toggle-overseas');
              if (elOverseas) elOverseas.checked = !currentLiveSetting.krOnlyViewing;
              const radio = document.querySelector(`input[name="replayPublishType"][value="${currentLiveSetting.replayPublishType}"]`);
              if (radio) radio.checked = true;

              // 같이보기 상태 확인 및 UI 반영
              if (content.watchParty && content.watchParty.watchPartyNo) {
                currentLiveSetting.watchPartyNo = content.watchParty.watchPartyNo;
                const selWp = document.getElementById('select-watch-party');
                if (selWp) selWp.value = String(content.watchParty.watchPartyNo);
                const wpItem = availableWatchParties.find(p => String(p.watchPartyNo) === String(content.watchParty.watchPartyNo)) || content.watchParty;
                applyWatchPartyLock(wpItem, false);
                if (content.watchParty.donationBoardInfo && content.watchParty.donationBoardInfo.channelDonationBoardMessage) {
                  currentDonationBoardMessage = content.watchParty.donationBoardInfo.channelDonationBoardMessage;
                }
              } else {
                currentLiveSetting.watchPartyNo = null;
                const selWp = document.getElementById('select-watch-party');
                if (selWp) selWp.value = "";
                preWatchPartySnapshot = null;
                releaseWatchPartyLock(false);
              }

              currentLiveSetting.defaultThumbnailImageUrl = content.defaultThumbnailImageUrl || null;
              updateThumbnailToolbarButton();


              // 드롭스(Drops) 상태 동기화: 치지직 채널 공식 실시간 활성 캠페인 매칭 우선
              const matchedDrop = findActiveDropsCampaign(catId, catValue);
              if (matchedDrop && matchedDrop.campaignNo) {
                currentDropsCampaignNo = matchedDrop.campaignNo;
                isDropsSupported = true;
                isDropsActive = true;
                await fetchDropsCampaignDetails(matchedDrop.campaignNo);
              } else {
                // 공식 활성 목록에 일치하는 캠페인이 없는 경우 (미지원 카테고리이거나 종료됨)
                const catDropsNo = extractDropsCampaignNo(cat);
                const targetDropsNo = catDropsNo || content.dropsCampaignNo || null;
                if (targetDropsNo) {
                  const details = await fetchDropsCampaignDetails(targetDropsNo);
                  if (details && !details.isTerminated) {
                    currentDropsCampaignNo = targetDropsNo;
                    isDropsSupported = true;
                    isDropsActive = true;
                  } else {
                    currentDropsCampaignNo = null;
                    isDropsSupported = false;
                    isDropsActive = false;
                    currentDropsCampaignTitle = '';
                  }
                } else {
                  currentDropsCampaignNo = null;
                  isDropsSupported = false;
                  isDropsActive = false;
                  currentDropsCampaignTitle = '';
                }
              }
              updateDropsBadgeUI();

              loaded = true;
            }
          }
        } catch (e) {}

      // 2. 채팅 설정 (BFF)
      try {
        const chatRes = await chzzkApi.getChatRule();
        if (chatRes.ok) {
          const chatData = await chatRes.json();
          const chatContent = chatData.content || chatData;
          if (chatContent) {
            if (chatContent.chatAvailableGroup) {
              setSelectedChatGroup(chatContent.chatAvailableGroup);
            }
            if (chatContent.minFollowerMinute !== undefined) {
              document.getElementById('min-follower-minute').value = chatContent.minFollowerMinute;
            }
            if (chatContent.allowSubscriberInFollowerMode !== undefined) {
              document.getElementById('toggle-sub-exception').checked = chatContent.allowSubscriberInFollowerMode;
            }
            loaded = true;
          }
        }
      } catch (e) {}

      if (loaded) {
        if (!silent) showToast('현재 방송 설정을 불러왔습니다', 'success');
        // 설정 로드 성공 시 헤더 상태도 '연동됨(초록불)'으로 확정
        const text = document.getElementById('header-status-text');
        const badgeA = document.getElementById('badge-method-a');
        const badgeB = document.getElementById('badge-method-b');
        const btnWebview = document.getElementById('btn-webview-login');

        if (text) { text.textContent = '연동됨'; text.className = 'font-medium text-[#00FFA3]'; }
        if (badgeA) { badgeA.textContent = "✅ 쿠키 연동됨"; badgeA.className = "text-[9px] px-1.5 py-0.5 rounded font-bold bg-emerald-950 text-[#00FFA3] border border-emerald-800/60"; }
        if (badgeB) { badgeB.textContent = "✅ 등록됨"; badgeB.className = "text-[9px] px-1.5 py-0.5 rounded font-bold bg-emerald-950 text-[#00FFA3] border border-emerald-800/60"; }
        if (btnWebview) {
          btnWebview.innerHTML = "🚪 로그아웃";
          btnWebview.className = "save-btn text-xs py-2 bg-rose-900/60 hover:bg-rose-800 text-rose-200 border border-rose-600/60 transition-colors cursor-pointer";
          btnWebview.onclick = handleLogout;
        }
        updateSavedBroadcastBaseline();
      } else {
        if (!silent) showToast('설정 불러오기 실패', 'error', '네이버 로그인 쿠키 상태를 확인해 주세요.');
      }
    } finally {
      if (loadSpinner) {
        setTimeout(() => loadSpinner.classList.remove('animate-spin'), 350);
      }
    }
  }



    async function saveAll() {
      if (!checkLoginGuard('설정 저장')) return;
      if (!userChannelId) {
        await resolveChannelId();
      }
      if (!userChannelId) {
        showToast('채널 정보 없음', 'error', '⚙ 로그인 설정을 먼저 완료해 주세요.');
        return;
      }

      const btn = document.getElementById('btn-save');
      const saveText = document.getElementById('save-text');
      const spinner = document.getElementById('save-spinner');

      btn.disabled = true;
      saveText.textContent = '적용 중...';
      spinner.style.display = 'inline-block';

      let liveSuccess = false;

      try {
        // 1. 방송 정보 & 세부 설정 저장 (PUT /live-setting/normal)
      try {
        let title = document.getElementById('live-title').value.trim();
        if (!title) {
          if (savedBroadcastBaseline && savedBroadcastBaseline.title) {
            title = savedBroadcastBaseline.title;
            document.getElementById('live-title').value = title;
            document.getElementById('title-count').textContent = `${title.length}/50`;
            showToast('제목 복구', 'info', '제목이 비어있어 이전 값으로 복구되었습니다.');
          } else {
            showToast('입력 확인', 'warn', '방송 제목을 입력해 주세요.');
            btn.disabled = false;
            saveText.textContent = '적용';
            spinner.style.display = 'none';
            return;
          }
        }

        let catSearchVal = (document.getElementById('category-search')?.value || '').trim();
        let catId = (document.getElementById('category-id')?.value || '').trim();
        let catType = (document.getElementById('category-type')?.value || '').trim();

        // 카테고리 미지정 시 자동 확정 1회 시도 후, 유효하지 않으면 에러 표시 및 저장 중단
        if (!catId || !catSearchVal) {
          await autoConfirmOrValidateCategory();
          catSearchVal = (document.getElementById('category-search')?.value || '').trim();
          catId = (document.getElementById('category-id')?.value || '').trim();
          catType = (document.getElementById('category-type')?.value || '').trim();
        }
        if (catId.toLowerCase() === 'talk' || catSearchVal.toLowerCase() === 'talk') {
          catId = 'talk';
          catSearchVal = 'talk';
          catType = 'ETC';
        }

        if (!catId || !catSearchVal) {
          setCategoryErrorState(true);
          showToast('카테고리 지정 필요', 'warn', '정확한 카테고리를 목록에서 선택해 주세요.');
          btn.disabled = false;
          saveText.textContent = '적용';
          spinner.style.display = 'none';
          return;
        }
        setCategoryErrorState(false);
        const targetAdult = document.getElementById('toggle-adult').checked;
        const targetClip = document.getElementById('toggle-clip').checked;
        const targetPaid = document.getElementById('toggle-paid-promotion')?.checked ?? false;
        const targetOverseas = document.getElementById('toggle-overseas')?.checked ?? true;
        const targetKrOnly = !targetOverseas;
        const targetReplay = document.querySelector('input[name="replayPublishType"]:checked')?.value || 'MANUAL';
        const targetWpNo = document.getElementById('select-watch-party')?.value || '';
        const isWp = !!targetWpNo;

        // 선택 불가 같이보기 캠페인 가드
        if (isWp && activeWatchParty && activeWatchParty.selectable === false) {
          showToast('선택 불가 같이보기', 'error', '현재 선택할 수 없는 같이보기 캠페인입니다.');
          btn.disabled = false;
          saveText.textContent = '적용';
          spinner.style.display = 'none';
          return;
        }

        // 같이보기+(RS) 선택 시 필수인 채팅 후원 자동 ON 동기화
        if (isWp && activeWatchParty && activeWatchParty.watchPartyType === 'RS') {
          const elChat = document.getElementById('toggle-chat-donation');
          if (elChat && !elChat.checked) {
            elChat.checked = true;
          }
        }

        // 1. 후원 3종 설정 선행 저장 (같이보기+ RS 방송의 채팅 후원 필수 요구조건 충족을 위해 선행)
        const chatActive = document.getElementById('toggle-chat-donation')?.checked ?? false;
        const videoActive = document.getElementById('toggle-video-donation')?.checked ?? false;
        const missionActive = document.getElementById('toggle-mission-donation')?.checked ?? false;

        if (chatActive !== currentDonationState.chatDonationActive) {
          await updateDonationSetting('CHAT', true);
        }
        if (videoActive !== currentDonationState.videoDonationActive) {
          await updateDonationSetting('VIDEO', true);
        }
        if (missionActive !== currentDonationState.missionDonationActive) {
          await updateDonationSetting('MISSION', true);
        }

        // 캠페인별 규칙 적용 (카테고리/클립/다시보기/성인/해외시청/응원전광판)
        const isWpAdult = !!(isWp && activeWatchParty && activeWatchParty.adult);
        const isWpClipFalse = !!(isWp && activeWatchParty && activeWatchParty.clipActive === false);
        const isWpReplayFalse = !!(isWp && activeWatchParty && activeWatchParty.allowReplayVideo === false);

        if (isWp && activeWatchParty && activeWatchParty.category && (activeWatchParty.category.categoryId || activeWatchParty.category.categoryValue)) {
          catId = activeWatchParty.category.categoryId || catId;
          catType = activeWatchParty.category.categoryType || catType;
        }

        const base = currentLiveSetting.rawContent || {};
        const payloadLive = {
          defaultLiveTitle: title,
          categoryType: catType,
          liveCategory: catId,
          tags: currentTags || [],
          adult: isWpAdult ? true : targetAdult,
          clipActive: isWpClipFalse ? false : targetClip,
          replayPublishType: isWpReplayFalse ? 'NONE' : targetReplay,
          paidPromotion: targetPaid,
          krOnlyViewing: isWp ? !!activeWatchParty.krOnlyViewing : targetKrOnly,
          cvExposure: base.cvExposure !== undefined ? base.cvExposure : true,
          timeMachineActive: isWpClipFalse ? false : (base.timeMachineActive !== undefined ? base.timeMachineActive : true),
          aiCaptionActive: !!base.aiCaptionActive,
          streamerShopCatalogTagActive: !!base.streamerShopCatalogTagActive,
          defaultThumbnailImageUrl: (currentLiveSetting.defaultThumbnailImageUrl !== undefined ? currentLiveSetting.defaultThumbnailImageUrl : (base.defaultThumbnailImageUrl || null)),
          dropsCampaignNo: (isDropsSupported && isDropsActive) ? currentDropsCampaignNo : null,
          paidProductId: base.paidProductId || null,
          broadcastLanguage: base.broadcastLanguage || null,
          watchPartyNo: isWp ? Number(targetWpNo) : null,
          channelDonationBoardMessage: (isWp && activeWatchParty && activeWatchParty.donationBoardInfo) ? (currentDonationBoardMessage || '') : void 0
        };

        const resLive = await chzzkApi.saveBroadcast(payloadLive);

        if (resLive.ok) {
          currentLiveSetting.adult = payloadLive.adult;
          currentLiveSetting.clipActive = payloadLive.clipActive;
          currentLiveSetting.krOnlyViewing = payloadLive.krOnlyViewing;
          currentLiveSetting.replayPublishType = payloadLive.replayPublishType;
          currentLiveSetting.watchPartyNo = payloadLive.watchPartyNo;
          currentLiveSetting.defaultThumbnailImageUrl = payloadLive.defaultThumbnailImageUrl;
          currentLiveSetting.dropsCampaignNo = payloadLive.dropsCampaignNo;
          updateThumbnailToolbarButton();
          updateDropsBadgeUI();
          liveSuccess = true;
        } else {
          let errorMsg = '치지직 서버 권한을 확인하세요.';
          try {
            const errData = await resLive.json();
            if (errData && errData.message) {
              errorMsg = errData.message;
            }
          } catch (e) {}
          showToast('방송 정보 저장 실패', 'error', errorMsg);
        }
      } catch (err) {
        showToast('방송 정보 저장 오류', 'error', err.message);
      }

        // 2. 방송 설정 저장이 성공한 경우에만 화면에 최신 방송 정보 다시 동기화
        // (저장이 실패한 경우 사용자가 선택해둔 같이보기나 폼 입력값이 리셋되는 문제 방지)
        if (liveSuccess) {
          await loadCurrentSettings(true);
          saveText.textContent = '✅ 적용 완료!';
          showToast('🎉 방송 설정 적용 완료!', 'success');
          setTimeout(() => { saveText.textContent = '적용'; }, 1500);
        } else {
          saveText.textContent = '적용';
        }
      } finally {
        btn.disabled = false;
        spinner.style.display = 'none';
      }
    }


    // 인기 방송 카테고리 및 약칭/은어 자동 완성 매핑 사전
    const CATEGORY_ALIAS_MAP = {
      '롤': '리그 오브 레전드',
      'lol': '리그 오브 레전드',
      '리그': '리그 오브 레전드',
      'league': '리그 오브 레전드',
      'league of legends': '리그 오브 레전드',
      'leagueoflegends': '리그 오브 레전드',
      '리그오브레전드': '리그 오브 레전드',
      '배그': 'PUBG: 배틀그라운드',
      'pubg': 'PUBG: 배틀그라운드',
      '배틀그라운드': 'PUBG: 배틀그라운드',
      '옵치': '오버워치',
      '옵치2': '오버워치 2',
      '오버워치2': '오버워치 2',
      'overwatch': '오버워치',
      '로아': '로스트아크',
      'lostark': '로스트아크',
      'lost ark': '로스트아크',
      '로스트아크': '로스트아크',
      '발로': '발로란트',
      '발로란트': '발로란트',
      'valorant': '발로란트',
      '저챗': 'talk',
      '저스트채팅': 'talk',
      '토크': 'talk',
      '소통': 'talk',
      'chat': 'talk',
      'talk': 'talk',
      '마크': 'Minecraft',
      'minecraft': 'Minecraft',
      '마인크래프트': 'Minecraft',
      '메플': '메이플스토리',
      '메이플': '메이플스토리',
      '메이플스토리': '메이플스토리',
      'maplestory': '메이플스토리',
      '던파': '던전앤파이터',
      'dnf': '던전앤파이터',
      '던전앤파이터': '던전앤파이터',
      '스타': '스타크래프트',
      'starcraft': '스타크래프트',
      '스타크래프트': '스타크래프트',
      '피파': 'EA SPORTS FC™ 온라인',
      'fconline': 'EA SPORTS FC™ 온라인',
      'fc온라인': 'EA SPORTS FC™ 온라인',
      '에펙': 'Apex 레전드',
      'apex': 'Apex 레전드',
      'apex레전드': 'Apex 레전드',
      '철권': '철권',
      '철권8': '철권 8',
      'tekken': '철권',
      '몬헌': '몬스터 헌터',
      '몬스터헌터': '몬스터 헌터',
      '원신': '원신',
      'genshin': '원신',
      '붕스': '붕괴: 스타레일',
      '스타레일': '붕괴: 스타레일',
      '붕괴스타레일': '붕괴: 스타레일',
      '블아': '블루 아카이브',
      '블루아카': '블루 아카이브',
      '블루아카이브': '블루 아카이브',
      '명조': '명조: 워더링 웨이브',
      '팰월드': '팰월드',
      'palworld': '팰월드',
      '스팀': '스팀'
    };

    // ===== 카테고리 즐겨찾기 관리 (최대 8개, settings.json 연동) =====
    const MAX_CATEGORY_FAVORITES = 8;
    
    


    async function fetchLivePopularCategories() {
      const now = Date.now();
      if (livePopularCategories.length > 0 && (now - livePopularCategoriesLastFetch < 60000)) {
        return livePopularCategories;
      }
      try {
        const res = await apiFetch('/chzzkdock/categories/recommended');
        if (res.ok) {
          const data = await res.json();
          const list = data?.categories || [];
          if (Array.isArray(list) && list.length > 0) {
            livePopularCategories = list;
            livePopularCategoriesLastFetch = now;
          }
        }
      } catch (err) {
        console.warn('실시간 인기 카테고리 로드 실패:', err);
      }
      return livePopularCategories;
    }

    


    async function fetchCategoriesApi(keyword) {
      if (!keyword || !keyword.trim()) return [];
      try {
        const q = encodeURIComponent(keyword.trim());
        const res = await apiFetch(`/chzzkdock/categories/search?keyword=${q}&size=50`);
        if (res.ok) {
          const results = await res.json();
          return Array.isArray(results) ? results : (results?.content?.results || []);
        }
      } catch (err) {
        console.error('fetchCategoriesApi 에러', err);
      }
      return [];
    }


    async function searchCategories(query) {
      if (!query || !query.trim()) {
        const dropdown = document.getElementById('category-dropdown');
        if (dropdown) dropdown.style.display = 'none';
        return;
      }

      const qTrim = query.trim();
      const qLower = qTrim.toLowerCase();
      const mappedAlias = CATEGORY_ALIAS_MAP[qLower];

      try {
        if (activeChannelDrops.length === 0 && userChannelId) {
          fetchChannelDropsCampaigns(userChannelId).catch(() => {});
        }
        // 별칭 매핑이 있는 경우, 별칭 검색과 원본 검색을 병렬로 동시 요청
        const reqs = [fetchCategoriesApi(qTrim)];
        if (mappedAlias && mappedAlias.toLowerCase() !== qLower) {
          reqs.push(fetchCategoriesApi(mappedAlias));
        }

        const responses = await Promise.all(reqs);
        let mergedList = [];
        const seenIds = new Set();

        // 매핑된 별칭 결과가 있으면 최우선 배치
        if (responses.length > 1) {
          for (const item of responses[1]) {
            if (item && item.categoryId && !seenIds.has(item.categoryId)) {
              seenIds.add(item.categoryId);
              mergedList.push(item);
            }
          }
        }

        // 원본 검색어 결과 추가
        for (const item of responses[0]) {
          if (item && item.categoryId && !seenIds.has(item.categoryId)) {
            seenIds.add(item.categoryId);
            mergedList.push(item);
          }
        }

        // '저챗'/'토크'/'소통'/'talk'의 경우 공식 talk 항목 보장
        if (qLower === '저챗' || qLower === '저스트채팅' || qLower === '토크' || qLower === '소통' || qLower === 'talk') {
          if (!seenIds.has('talk')) {
            mergedList.unshift({
              categoryType: 'ETC',
              categoryId: 'talk',
              categoryValue: 'talk',
              posterImageUrl: 'https://ssl.pstatic.net/static/nng/glive/icon/category_poster_talk.png'
            });
          }
        }

        // 정확도(Relevance) 기반 우선순위 정렬
        // 1. 공백 제거 후 완전 일치 (예: '리그오브레전드' == '리그 오브 레전드')
        // 2. 카테고리명이 검색어나 별칭과 정확히 일치하는 경우
        // 3. 카테고리명이 검색어나 별칭으로 시작하는 경우
        const cleanStr = s => (s || '').replace(/\s+/g, '').toLowerCase();
        const targetClean = cleanStr(mappedAlias || qTrim);

        mergedList.sort((a, b) => {
          const aId = a.categoryId || '';
          const bId = b.categoryId || '';
          const aFav = isCategoryFavorite(aId) ? 1 : 0;
          const bFav = isCategoryFavorite(bId) ? 1 : 0;
          if (aFav !== bFav) return bFav - aFav; // 즐겨찾기 등록 항목 최우선

          const aVal = (a.categoryValue || a.categoryId || '').toLowerCase();
          const bVal = (b.categoryValue || b.categoryId || '').toLowerCase();
          const target = (mappedAlias || qTrim).toLowerCase();

          // 1단계: 띄어쓰기 무관 완전 일치
          const aCleanExact = cleanStr(aVal) === targetClean ? 1 : 0;
          const bCleanExact = cleanStr(bVal) === targetClean ? 1 : 0;
          if (aCleanExact !== bCleanExact) return bCleanExact - aCleanExact;

          // 2단계: 띄어쓰기 포함 원문 완전 일치
          const aExact = aVal === target ? 1 : 0;
          const bExact = bVal === target ? 1 : 0;
          if (aExact !== bExact) return bExact - aExact;

          // 3단계: 전방 일치
          const aStarts = aVal.startsWith(target) ? 1 : 0;
          const bStarts = bVal.startsWith(target) ? 1 : 0;
          if (aStarts !== bStarts) return bStarts - aStarts;

          return 0;
        });

        const dropdown = document.getElementById('category-dropdown');
        if (!dropdown) return;

        if (!mergedList.length) {
          dropdown.innerHTML = '<div class="px-2.5 py-3 text-[11px] text-chzzk-muted text-center">검색 결과 없음</div>';
          dropdown.style.display = 'block';
          return;
        }

        dropdown.innerHTML = mergedList.map(cat => {
          const catType = cat.categoryType || 'GAME';
          const catId = cat.categoryId || '';
          const catVal = cat.categoryValue || catId;
          const poster = cat.posterImageUrl || '';
          const dropsCampaignNo = extractDropsCampaignNo(cat);
          const isFav = isCategoryFavorite(catId);
          return renderCategoryRowHtml(catType, catId, catVal, poster, dropsCampaignNo, isFav);
        }).join('');
        dropdown.style.display = 'block';
      } catch (err) {
        console.error('카테고리 검색 오류', err);
      }
    }


    function selectCategoryItem(el) {
      const type = el.getAttribute('data-type') || 'ETC';
      const id = el.getAttribute('data-id') || '';
      const value = el.getAttribute('data-value') || '';
      const drops = el.getAttribute('data-drops') || '';
      selectCategory(type, id, value, drops ? (Number(drops) || drops) : null);
    }


    function selectCategory(type, id, value, dropsCampaignNo = null) {
      if ((id && id.toLowerCase() === 'talk') || (value && value.toLowerCase() === 'talk')) {
        id = 'talk';
        value = 'talk';
        type = 'ETC';
      }
      const elSearch = document.getElementById('category-search');
      if (elSearch) {
        elSearch.value = value;
        elSearch.dataset.confirmedValue = value;
        elSearch.dataset.confirmedId = id;
      }
      document.getElementById('category-type').value = type;
      document.getElementById('category-id').value = id;
      document.getElementById('category-dropdown').style.display = 'none';
      document.getElementById('category-clear').style.display = 'block';

      // 1. 치지직 채널 공식 실시간 활성 드롭스 캠페인 매칭 최우선
      const matchedDrop = findActiveDropsCampaign(id, value);
      const targetDropNo = (matchedDrop && matchedDrop.campaignNo) ? matchedDrop.campaignNo : dropsCampaignNo;

      if (targetDropNo) {
        currentDropsCampaignNo = targetDropNo;
        isDropsSupported = true;
        isDropsActive = true;
        currentDropsCampaignTitle = '';
        fetchDropsCampaignDetails(targetDropNo);
      } else {
        currentDropsCampaignNo = null;
        isDropsSupported = false;
        isDropsActive = false;
        currentDropsCampaignTitle = '';
        if (value || id) {
          checkCategoryDropsSupport(value, id);
        }
      }
      updateDropsBadgeUI();
      setCategoryErrorState(false);
      updateCategoryFavoriteToggleUI();

      showToast(`카테고리: ${value}`, 'info');
      checkBroadcastStateDirty();
    }


    function setCategoryErrorState(hasError) {
      const elSearch = document.getElementById('category-search');
      const elMsg = document.getElementById('category-error-msg');
      if (elSearch) {
        if (hasError) {
          elSearch.classList.add('!border-rose-500', 'bg-rose-950/20');
        } else {
          elSearch.classList.remove('!border-rose-500', 'bg-rose-950/20');
        }
      }
      if (elMsg) {
        if (hasError) {
          elMsg.classList.remove('hidden');
          elMsg.classList.add('flex');
        } else {
          elMsg.classList.add('hidden');
          elMsg.classList.remove('flex');
        }
      }
    }


    async function autoConfirmOrValidateCategory() {
      const elSearch = document.getElementById('category-search');
      const elId = document.getElementById('category-id');
      const dropdown = document.getElementById('category-dropdown');
      if (!elSearch) return;

      const val = elSearch.value.trim();
      if (!val) {
        clearCategory();
        setCategoryErrorState(true);
        return;
      }
      const valLower = val.toLowerCase();
      if (valLower === 'talk' || valLower === '저챗' || valLower === '저스트채팅' || valLower === '토크' || valLower === '소통') {
        selectCategory('ETC', 'talk', 'talk', null);
        setCategoryErrorState(false);
        return;
      }

      // 1. 이미 정상 확정된 카테고리인 경우 (confirmedValue와 일치하고 id가 있는 경우)
      if (elId && elId.value && (elSearch.dataset.confirmedValue === val || elId.value === val)) {
        setCategoryErrorState(false);
        return;
      }

      // 2. 현재 드롭다운에 항목들이 떠 있다면 1위 일치 항목 자동 선택
      const items = dropdown ? dropdown.querySelectorAll('.category-item') : [];
      if (items.length > 0) {
        selectCategoryItem(items[0]);
        setCategoryErrorState(false);
        return;
      }

      // 3. 드롭다운이 안 떴거나 비어있었다면 직접 API로 1위 항목 검색 후 자동 매핑
      try {
        const results = await fetchCategoriesApi(val);
        if (results && results.length > 0) {
          const match = results.find(item => (item.categoryValue && item.categoryValue.toLowerCase() === val.toLowerCase()) || item.categoryId === val) || results[0];
          const dropsCampaignNo = extractDropsCampaignNo(match);
          selectCategory(match.categoryType || 'GAME', match.categoryId || '', match.categoryValue || match.categoryId || '', dropsCampaignNo);
          setCategoryErrorState(false);
        } else {
          // 일치하는 카테고리가 전혀 없으면 에러 표시
          setCategoryErrorState(true);
        }
      } catch (e) {
        if (elId && elId.value) {
          setCategoryErrorState(false);
        } else {
          setCategoryErrorState(true);
        }
      }
    }


    function validateCategoryInput() {
      const elSearch = document.getElementById('category-search');
      const elId = document.getElementById('category-id');
      if (!elSearch) return;
      const val = elSearch.value.trim();
      if (!val) {
        setCategoryErrorState(true);
      } else if (elId && elId.value && (elSearch.dataset.confirmedValue === val || !elSearch.dataset.confirmedValue)) {
        setCategoryErrorState(false);
      } else {
        autoConfirmOrValidateCategory();
      }
    }


    function clearCategory() {
      document.getElementById('category-search').value = '';
      document.getElementById('category-type').value = '';
      document.getElementById('category-id').value = '';
      document.getElementById('category-clear').style.display = 'none';

      currentDropsCampaignNo = null;
      isDropsSupported = false;
      isDropsActive = false;
      currentDropsCampaignTitle = '';
      updateDropsBadgeUI();
      setCategoryErrorState(false);
      updateCategoryFavoriteToggleUI();

      checkBroadcastStateDirty();
    }


    function clearTitle() {
      const elTitle = document.getElementById('live-title');
      if (elTitle) {
        elTitle.value = '';
        const countEl = document.getElementById('title-count');
        if (countEl) countEl.textContent = '0/50';
        const clearBtn = document.getElementById('title-clear');
        if (clearBtn) clearBtn.style.display = 'none';
        elTitle.focus();
        checkBroadcastStateDirty();
      }
    }

    


    function addTag(value) {
      if (!value) return false;
      value = value.replace(/\s+/g, '');
      if (!value) return false;
      if (!/^[a-zA-Z0-9가-힣ㄱ-ㅎㅏ-ㅣ]+$/.test(value)) {
        showToast('입력 오류', 'error', '공백 및 특수문자는 사용할 수 없습니다.');
        return false;
      }
      if (value.length > 15) {
        showToast('태그 길이 제한', 'warn', '태그는 최대 15자까지 입력할 수 있습니다.');
        return false;
      }
      if (currentTags.length >= 5) {
        showToast('태그 제한', 'warn', '태그는 최대 5개까지 추가할 수 있습니다.');
        return false;
      }
      if (currentTags.includes(value)) {
        showToast('중복 태그', 'warn', '이미 추가된 태그입니다.');
        return false;
      }
      currentTags.push(value);
      renderTags();
      return true;
    }


    function removeTag(index) {
      if (index >= 0 && index < currentTags.length) {
        currentTags.splice(index, 1);
        renderTags();
      }
    }


    function updateTagCharCounter() {
      const inputEl = document.getElementById('tag-inline-input');
      const counterEl = document.getElementById('tag-char-counter');
      if (!inputEl || !counterEl) return;
      const len = inputEl.value.length;
      counterEl.textContent = `${len}/15`;
      if (len >= 15) {
        counterEl.className = 'absolute -top-3 right-0 text-[8.5px] font-bold px-1 py-0 rounded bg-rose-950/90 border border-rose-500 text-rose-300 shadow-md select-none pointer-events-none transition-colors duration-150 animate-pulse';
      } else if (len >= 12) {
        counterEl.className = 'absolute -top-3 right-0 text-[8.5px] font-bold px-1 py-0 rounded bg-amber-950/90 border border-amber-500 text-amber-300 shadow-md select-none pointer-events-none transition-colors duration-150';
      } else {
        counterEl.className = 'absolute -top-3 right-0 text-[8.5px] font-bold px-1 py-0 rounded bg-[#0B0F15] border border-[#00FFA3]/50 text-[#00FFA3] shadow-md select-none pointer-events-none transition-colors duration-150';
      }
    }


    function openTagInput() {
      if (currentTags.length >= 5) {
        showToast('태그 제한', 'warn', '태그는 최대 5개까지 추가할 수 있습니다.');
        return;
      }
      isTagInputOpen = true;
      const inlineBox = document.getElementById('tag-inline-box');
      const btnOpen = document.getElementById('btn-open-tag');
      const inputEl = document.getElementById('tag-inline-input');

      if (inlineBox) inlineBox.style.display = 'inline-flex';
      if (btnOpen) btnOpen.style.display = 'none';

      if (inputEl) {
        inputEl.value = '';
        updateTagCharCounter();
        requestAnimationFrame(() => {
          inputEl.focus();
        });
      }
    }


    function closeTagInput() {
      isTagInputOpen = false;
      const inlineBox = document.getElementById('tag-inline-box');
      const btnOpen = document.getElementById('btn-open-tag');
      const inputEl = document.getElementById('tag-inline-input');

      if (inlineBox) inlineBox.style.display = 'none';
      if (btnOpen && currentTags.length < 5) btnOpen.style.display = 'inline-flex';
      if (inputEl) inputEl.value = '';
    }


    function confirmTagInput() {
      const inputEl = document.getElementById('tag-inline-input');
      if (!inputEl) return;
      const val = inputEl.value.trim();
      if (!val) {
        closeTagInput();
        return;
      }
      const ok = addTag(val);
      if (ok) {
        inputEl.value = '';
        if (currentTags.length >= 5) {
          closeTagInput();
        } else {
          requestAnimationFrame(() => {
            inputEl.focus();
          });
        }
      }
    }


    function renderTags() {
      const chipsWrapper = document.getElementById('tag-chips-wrapper');
      const countEl = document.getElementById('tag-count-label');
      const inlineBox = document.getElementById('tag-inline-box');
      const btnOpen = document.getElementById('btn-open-tag');

      if (countEl) {
        countEl.textContent = `${currentTags.length}/5`;
      }

      if (chipsWrapper) {
        let html = '';

        // 1. 치지직 같이보기 시스템 규정 필수 태그 (스튜디오 동일: 삭제 불가 전용 고정 뱃지)
        if (activeWatchParty && activeWatchParty.watchPartyTag) {
          const isRs = activeWatchParty.watchPartyType === 'RS';
          const badgeClass = isRs
            ? '!border-[#00FFA3]/40 !bg-[#00FFA3]/10 !text-[#00FFA3]'
            : '!border-purple-500/40 !bg-purple-500/10 !text-purple-300';

          html += `
            <span class="tag-chip ${badgeClass} font-semibold cursor-default flex items-center gap-1 select-none" title="치지직 같이보기 규정 태그 (자동 부여 및 고정)">
              <span class="text-[9.5px]">🔒</span>
              <span>#${escapeHtml(activeWatchParty.watchPartyTag)}</span>
            </span>
          `;
        }

        // 2. 사용자가 직접 등록한 일반 태그 목록 (최대 5개, 삭제 가능)
        html += currentTags.map((tag, i) => `
          <span class="tag-chip">
            #${escapeHtml(tag)}
            <button type="button" onclick="removeTag(${i})" title="태그 삭제">✕</button>
          </span>
        `).join('');

        chipsWrapper.innerHTML = html;
      }

      if (currentTags.length >= 5) {
        if (inlineBox) inlineBox.style.display = 'none';
        if (btnOpen) btnOpen.style.display = 'none';
        isTagInputOpen = false;
      } else {
        if (isTagInputOpen) {
          if (inlineBox) inlineBox.style.display = 'inline-flex';
          if (btnOpen) btnOpen.style.display = 'none';
        } else {
          if (inlineBox) inlineBox.style.display = 'none';
          if (btnOpen) btnOpen.style.display = 'inline-flex';
        }
      }

      checkBroadcastStateDirty();
    }

// ============================================================
//  Bootstrap Entrypoint Execution
// ============================================================
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', startApp);
} else {
  startApp();
}
