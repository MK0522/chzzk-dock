// ui/main-dock/js/settings.js

    function setFontScale(scale, save = true) {
      if (document.body) {
        document.body.style.zoom = String(scale);
      }

      const scales = [1.0, 1.2, 1.35];
      scales.forEach(s => {
        const idKey = Math.round(s * 100);
        const btn = document.getElementById(`btn-scale-${idKey}`);
        if (btn) {
          if (Math.abs(s - scale) < 0.01) {
            btn.className = 'py-1 text-[11px] font-bold rounded text-[#00FFA3] bg-[#161F2E] border border-[#00FFA3]/40 transition-all text-center cursor-pointer';
          } else {
            btn.className = 'py-1 text-[11px] font-medium rounded text-slate-400 hover:text-slate-200 transition-all text-center cursor-pointer';
          }
        }
      });

      const label = document.getElementById('font-scale-current-label');
      if (label) {
        if (scale === 1.0) label.textContent = '100% (보통)';
        else if (scale === 1.2) label.textContent = '120% (크게)';
        else if (scale === 1.35) label.textContent = '135% (아주 크게)';
      }

      if (save) {
        apiFetch('/app-settings', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ font_scale: scale })
        }).then(res => {
          if (res.ok) {
            showToast(`독 화면 크기 ${Math.round(scale * 100)}% 적용 완료`, 'success');
          } else {
            showToast('독 화면 크기 저장 실패', 'error');
          }
        }).catch(err => {
          showToast('독 화면 크기 저장 오류', 'error', err ? err.message : '');
        });
      }
    }


    async function initFontScale() {
      let saved = 1.2;
      try {
        const res = await apiFetch('/app-settings');
        if (res.ok) {
          const data = await res.json();
          if (data && typeof data.font_scale === 'number') saved = data.font_scale;
        }
      } catch (e) {}
      setFontScale(saved || 1.2, false);
    }

    // ===== Broadcast Settings Dirty State (설정 변경 감지 및 테두리 하이라이트) =====
    let savedBroadcastBaseline = null;


    async function loadWatchdogTimeout() {
      try {
        const res = await apiFetch('/watchdog-timeout');
        if (res.ok) {
          const data = await res.json();
          const sec = data.timeout_sec;
          const select = document.getElementById('select-watchdog-timeout');
          const customContainer = document.getElementById('custom-watchdog-container');
          const customInput = document.getElementById('input-custom-watchdog');
          if (!select) return;

          const presets = ['0', '10', '30', '60', '300', '600', '-1'];
          if (presets.includes(String(sec))) {
            select.value = String(sec);
            if (customContainer) customContainer.classList.add('hidden');
          } else {
            select.value = 'custom';
            if (customInput) customInput.value = sec;
            if (customContainer) customContainer.classList.remove('hidden');
          }
        }
      } catch (e) {
        console.warn('[CHZZK Dock] OBS 대기 시간 로드 예외:', e);
      }
    }


    async function onWatchdogTimeoutChange(val) {
      const customContainer = document.getElementById('custom-watchdog-container');
      const customInput = document.getElementById('input-custom-watchdog');
      if (val === 'custom') {
        if (customContainer) customContainer.classList.remove('hidden');
        if (customInput) {
          customInput.focus();
          if (!customInput.value) customInput.value = '45';
        }
      } else {
        if (customContainer) customContainer.classList.add('hidden');
        const sec = parseInt(val, 10);
        if (!isNaN(sec)) {
          await saveWatchdogTimeout(sec);
        }
      }
    }


    async function saveCustomWatchdogTimeout() {
      const customInput = document.getElementById('input-custom-watchdog');
      if (!customInput) return;
      let sec = parseInt(customInput.value, 10);
      if (isNaN(sec) || sec < 5) {
        showToast('입력 오류', 'warn', '대기 시간은 최소 5초 이상이어야 합니다.');
        return;
      }
      if (sec > 86400) {
        showToast('입력 오류', 'warn', '대기 시간은 최대 24시간(86400초) 이하이어야 합니다.');
        return;
      }
      await saveWatchdogTimeout(sec);
    }


    async function saveWatchdogTimeout(sec) {
      try {
        const res = await apiFetch('/watchdog-timeout', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ timeout_sec: sec })
        });
        if (res.ok) {
          if (sec === 0) {
            showToast('⏱️ 자동 종료 설정', 'info', 'OBS 종료 시 지연 없이 즉시 함께 종료되도록 설정되었습니다.');
          } else if (sec === -1) {
            showToast('⏱️ 자동 종료 설정', 'info', 'OBS 자동 종료가 비활성화되었습니다. (상시 실행 유지)');
          } else {
            let label = `${sec}초`;
            if (sec === 60) label = '1분';
            else if (sec === 300) label = '5분';
            else if (sec === 600) label = '10분';
            else if (sec >= 60 && sec % 60 === 0) label = `${sec / 60}분`;
            showToast('⏱️ 자동 종료 설정', 'info', `OBS 종료 시 ${label} 뒤 함께 종료되도록 설정되었습니다.`);
          }
        } else {
          showToast('설정 저장 실패', 'warn', '서버에 설정을 저장하지 못했습니다.');
        }
      } catch (e) {
        showToast('통신 오류', 'warn', '서버 통신 실패: ' + e.message);
      }
    }

    // ===== 서버 포트 관리 및 충돌 점검 제어 =====
    let currentActivePort = 8081;
    let currentConfiguredPort = 8081;
    let pendingPortToSave = null;


    function clearCustomPort() {
      const input = document.getElementById('input-custom-port');
      if (input) {
        input.value = '';
        const clearBtn = document.getElementById('port-clear');
        if (clearBtn) clearBtn.style.display = 'none';
        input.focus();
      }
    }


    function updatePortClearButton() {
      const input = document.getElementById('input-custom-port');
      const clearBtn = document.getElementById('port-clear');
      if (input && clearBtn) {
        clearBtn.style.display = input.value.length > 0 ? 'block' : 'none';
      }
    }


    async function loadPortStatus() {
      try {
        const res = await apiFetch('/port-status');
        if (!res.ok) return;
        const data = await res.json();
        currentActivePort = data.active_port || 8081;
        currentConfiguredPort = data.configured_port || 8081;

        const badge = document.getElementById('badge-port-status');
        const banner = document.getElementById('port-fallback-banner');
        const reasonEl = document.getElementById('port-fallback-reason');
        const inputPort = document.getElementById('input-custom-port');

        if (badge) {
          if (data.is_fallback) {
            badge.className = 'px-2 py-0.5 rounded text-[10px] font-bold bg-amber-950/80 text-amber-400 border border-amber-500/30 font-mono';
            badge.textContent = `현재 포트: ${currentActivePort} (대체)`;
          } else {
            badge.className = 'px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-950/80 text-emerald-400 border border-emerald-500/30 font-mono';
            badge.textContent = `현재 포트: ${currentActivePort}`;
          }
        }

        if (banner && reasonEl) {
          if (data.is_fallback) {
            banner.classList.remove('hidden');
            reasonEl.textContent = data.fallback_reason || '기본 포트 충돌로 인해 임시 포트로 실행 중입니다.';
          } else {
            banner.classList.add('hidden');
          }
        }

        if (inputPort && !inputPort.value) {
          inputPort.value = currentConfiguredPort;
        }
        updatePortClearButton();
      } catch (e) {
        console.warn('[CHZZK Dock] 포트 상태 조회 예외:', e);
      }
    }


    async function checkCustomPort() {
      const input = document.getElementById('input-custom-port');
      const resultEl = document.getElementById('port-check-result');
      if (!input || !resultEl) return;

      const port = parseInt(input.value, 10);
      if (isNaN(port) || port < 1024 || port > 65535) {
        resultEl.className = 'text-[10px] leading-tight text-rose-400 font-medium';
        resultEl.textContent = '❌ 포트 번호는 1024 ~ 65535 사이의 숫자여야 합니다.';
        return;
      }

      resultEl.className = 'text-[10px] leading-tight text-chzzk-muted font-medium';
      resultEl.textContent = `⏳ 포트 ${port}번 점유 여부 확인 중...`;

      try {
        const res = await apiFetch('/check-port', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ port })
        });
        const data = await res.json();

        if (data.available) {
          resultEl.className = 'text-[10px] leading-tight text-emerald-400 font-medium';
          resultEl.textContent = `✅ ${data.message || `포트 ${port}번은 사용 가능합니다.`}`;
        } else {
          resultEl.className = 'text-[10px] leading-tight text-rose-400 font-medium';
          const occupied = data.occupied_by ? ` ('${data.occupied_by}', PID ${data.pid})` : '';
          resultEl.textContent = `❌ 포트 ${port}번은 이미 다른 프로그램${occupied}에서 사용 중입니다.`;
        }
      } catch (e) {
        resultEl.className = 'text-[10px] leading-tight text-rose-400 font-medium';
        resultEl.textContent = '❌ 포트 점검 요청 실패: ' + e.message;
      }
    }


    function openPortApplyModal(port) {
      pendingPortToSave = port;
      showNoticeModal({
        icon: '🌐',
        title: '서버 포트 변경 적용',
        message: `기본 HTTP 서버 포트를 ${port}번으로 변경합니다.\n포트 설정을 지금 즉시 재시작하여 적용할지, 프로그램의 다음 실행 시 적용할지 선택해 주세요.`,
        submessage: '💡 지금 적용 선택 시 독 서버가 즉시 새 포트로 재시작되며 새 주소로 자동 이동합니다.',
        buttons: [
          {
            text: '취소',
            className: 'px-2.5 py-1 rounded bg-[#1B2535] hover:bg-[#253247] text-slate-300 text-xs font-medium cursor-pointer transition-colors',
            onClick: () => closePortApplyModal()
          },
          {
            text: '다음 실행시 적용',
            className: 'px-3 py-1 rounded bg-[#1E293B] hover:bg-[#2C3B52] border border-[#334155] text-slate-200 hover:text-white text-xs font-semibold cursor-pointer transition-colors',
            onClick: () => confirmSavePort(false)
          },
          {
            text: '지금 적용',
            className: 'px-3 py-1 rounded bg-[#00FFA3] hover:bg-[#00E591] text-black text-xs font-bold transition-all cursor-pointer',
            onClick: () => confirmSavePort(true)
          }
        ]
      });
    }


    function closePortApplyModal() {
      window.__closeConfirmModal();
      pendingPortToSave = null;
    }


    function saveCustomPort() {
      const input = document.getElementById('input-custom-port');
      if (!input) return;

      const port = parseInt(input.value, 10);
      if (isNaN(port) || port < 1024 || port > 65535) {
        showToast('포트 번호 오류', 'warn', '포트 번호는 1024 ~ 65535 사이여야 합니다.');
        return;
      }

      openPortApplyModal(port);
    }


    async function confirmSavePort(applyNow) {
      const port = pendingPortToSave;
      if (!port) {
        closePortApplyModal();
        return;
      }

      const resultEl = document.getElementById('port-check-result');
      closePortApplyModal();

      if (resultEl) {
        resultEl.className = 'text-[10px] leading-tight text-chzzk-muted font-medium';
        resultEl.textContent = applyNow
          ? `🔄 포트 ${port}번으로 즉시 적용 및 재시작 중...`
          : `⏳ 포트 ${port}번 저장 중...`;
      }

      try {
        const res = await apiFetch('/save-port', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ port, restart: applyNow })
        });
        const data = await res.json();

        if (res.ok && data.code === 200) {
          currentConfiguredPort = port;
          if (applyNow) {
            if (resultEl) {
              resultEl.className = 'text-[10px] leading-tight text-emerald-400 font-medium';
              resultEl.textContent = `🚀 포트 ${port}번 즉시 적용 완료! 잠시 후 새 주소로 자동 이동합니다...`;
            }
            showToast('🌐 포트 즉시 적용', 'success', `포트 ${port}번으로 재시작합니다. 잠시 후 이동합니다...`);
            setTimeout(() => {
              const newUrl = `${window.location.protocol}//${window.location.hostname}:${port}${window.location.pathname}${window.location.search}`;
              window.location.href = newUrl;
            }, 1200);
          } else {
            if (resultEl) {
              resultEl.className = 'text-[10px] leading-tight text-emerald-400 font-medium';
              resultEl.textContent = `💾 기본 포트 ${port}번 저장 완료 (다음 실행 시 적용)`;
            }
            showToast('🌐 포트 설정 저장됨', 'success', `기본 포트가 ${port}번으로 설정되었습니다. 프로그램 재시작 시 적용됩니다.`);
          }
        } else {
          const msg = data.message || '포트 저장에 실패했습니다.';
          if (resultEl) {
            resultEl.className = 'text-[10px] leading-tight text-rose-400 font-medium';
            resultEl.textContent = `❌ ${msg}`;
          }
          showToast('포트 저장 실패', 'error', msg);
        }
      } catch (e) {
        if (resultEl) {
          resultEl.className = 'text-[10px] leading-tight text-rose-400 font-medium';
          resultEl.textContent = '❌ 포트 저장 요청 실패: ' + e.message;
        }
        showToast('통신 오류', 'error', e.message);
      }
    }

    // ===== 독 환경설정 단일화 파이프라인 (GET/PUT /app-settings) =====

    async function loadAllDockSettings(providedData = null) {
      let data = providedData;
      if (!data) {
        try {
          const res = await apiFetch('/app-settings');
          if (res.ok) {
            data = await res.json();
          }
        } catch (e) {
          console.warn('[Settings] Failed to fetch /app-settings:', e);
        }
      }
      if (!data) return;
      applyDockSettings(data);
    }

    function applyDockSettings(data) {
      if (!data) return;

      // 1. 포트 상태
      currentActivePort = data.active_port || data.http_port || 8081;
      currentConfiguredPort = data.configured_port || data.http_port || 8081;
      const badge = document.getElementById('badge-port-status');
      const banner = document.getElementById('port-fallback-banner');
      const reasonEl = document.getElementById('port-fallback-reason');
      const inputPort = document.getElementById('input-custom-port');
      if (badge) {
        if (data.is_fallback) {
          badge.className = 'px-2 py-0.5 rounded text-[10px] font-bold bg-amber-950/80 text-amber-400 border border-amber-500/30 font-mono';
          badge.textContent = `현재 포트: ${currentActivePort} (대체)`;
        } else {
          badge.className = 'px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-950/80 text-emerald-400 border border-emerald-500/30 font-mono';
          badge.textContent = `현재 포트: ${currentActivePort}`;
        }
      }
      if (banner && reasonEl) {
        if (data.is_fallback) {
          banner.classList.remove('hidden');
          reasonEl.textContent = data.fallback_reason || '기본 포트 충돌로 인해 임시 포트로 실행 중입니다.';
        } else {
          banner.classList.add('hidden');
        }
      }
      if (inputPort && !inputPort.value) {
        inputPort.value = currentConfiguredPort;
      }
      updatePortClearButton();

      // 2. 폰트/화면 배율
      if (typeof data.font_scale === 'number') {
        setFontScale(data.font_scale, false);
      }

      // 3. 실시간 시청자 수
      if (typeof data.show_viewer_count === 'boolean') {
        showViewerCountState = data.show_viewer_count;
        const toggleEl = document.getElementById('toggle-show-viewer-count');
        if (toggleEl) toggleEl.checked = showViewerCountState;
        if (typeof updateLiveStatusUI === 'function') {
          updateLiveStatusUI((typeof isCurrentlyLive !== 'undefined') ? isCurrentlyLive : false, (typeof currentViewerCount !== 'undefined') ? currentViewerCount : 0);
        }
      }

      // 4. 시작 시 팝업창
      const togglePopup = document.getElementById('toggle-popup-on-start');
      if (togglePopup && typeof data.popup_on_start === 'boolean') {
        togglePopup.checked = data.popup_on_start;
      }

      // 5. 자동 종료 알림
      const toggleShutdown = document.getElementById('toggle-shutdown-notify');
      if (toggleShutdown && typeof data.notify_on_shutdown === 'boolean') {
        toggleShutdown.checked = data.notify_on_shutdown;
      }

      // 6. GPU 가속
      const toggleGpu = document.getElementById('toggle-enable-gpu');
      if (toggleGpu && typeof data.enable_gpu === 'boolean') {
        toggleGpu.checked = data.enable_gpu;
      }

      // 7. 외부 브라우저 가드 & 기본 브라우저 명
      const toggleExt = document.getElementById('toggle-external-browser');
      if (toggleExt && typeof data.external_browser_guard === 'boolean') {
        toggleExt.checked = data.external_browser_guard;
      }
      const labelBrowser = document.getElementById('label-default-browser');
      if (labelBrowser && data.default_browser) {
        const rawName = data.default_browser;
        labelBrowser.textContent = rawName;
        labelBrowser.classList.remove('hidden');
        const nameLower = rawName.toLowerCase();
        let colorClass = "bg-slate-800/80 text-slate-300 border border-slate-700/60";
        if (nameLower.includes('whale') || nameLower.includes('웨일')) {
          colorClass = "bg-emerald-950/90 text-[#00FFA3] border border-emerald-500/50";
        } else if (nameLower.includes('chrome') || nameLower.includes('크롬')) {
          colorClass = "bg-amber-950/90 text-amber-300 border border-amber-500/50";
        } else if (nameLower.includes('edge') || nameLower.includes('엣지')) {
          colorClass = "bg-sky-950/90 text-sky-300 border border-sky-500/50";
        } else if (nameLower.includes('firefox') || nameLower.includes('파이어폭스')) {
          colorClass = "bg-orange-950/90 text-orange-400 border border-orange-500/50";
        } else if (nameLower.includes('brave') || nameLower.includes('브레이브')) {
          colorClass = "bg-rose-950/90 text-rose-400 border border-rose-500/50";
        } else if (nameLower.includes('opera') || nameLower.includes('오페라')) {
          colorClass = "bg-red-950/90 text-red-400 border border-red-500/50";
        }
        labelBrowser.className = `px-1.5 py-0.5 rounded text-[10px] font-mono font-semibold ${colorClass}`;
      }

      // 8. 베타 업데이트
      const toggleBeta = document.getElementById('toggle-beta-updates');
      if (toggleBeta && typeof data.enable_beta_updates === 'boolean') {
        toggleBeta.checked = data.enable_beta_updates;
      }

      // 9. 업데이트 자동 확인
      const toggleAuto = document.getElementById('toggle-auto-check-update');
      if (toggleAuto && typeof data.auto_check_update === 'boolean') {
        toggleAuto.checked = data.auto_check_update;
      }

      // 10. OBS 자동 종료 대기 시간 (Watchdog)
      const sec = (typeof data.watchdog_timeout_sec === 'number') ? data.watchdog_timeout_sec : data.timeout_sec;
      if (typeof sec === 'number') {
        const select = document.getElementById('select-watchdog-timeout');
        const customContainer = document.getElementById('custom-watchdog-container');
        const customInput = document.getElementById('input-custom-watchdog');
        if (select) {
          const presets = ['0', '10', '30', '60', '300', '600', '-1'];
          if (presets.includes(String(sec))) {
            select.value = String(sec);
            if (customContainer) customContainer.classList.add('hidden');
          } else {
            select.value = 'custom';
            if (customInput) customInput.value = sec;
            if (customContainer) customContainer.classList.remove('hidden');
          }
        }
      }
    }

    async function saveAppSettingField(field, value, successMsg = '') {
      try {
        const res = await apiFetch('/app-settings', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ [field]: value })
        });
        const data = await res.json();
        if (res.ok && data.code === 200) {
          if (successMsg) showToast('설정 저장 완료', 'success', successMsg);
          return true;
        } else {
          showToast('설정 저장 실패', 'error', data.message || '설정 저장에 실패했습니다.');
          return false;
        }
      } catch (e) {
        showToast('통신 오류', 'error', e.message);
        return false;
      }
    }

    // 개별 설정 제어 (단일 API 및 하위 호환 래퍼)
    async function loadShowViewerCountSetting() { await loadAllDockSettings(); }
    async function saveShowViewerCountSetting(checked) {
      showViewerCountState = checked;
      if (typeof updateLiveStatusUI === 'function') updateLiveStatusUI(isCurrentlyLive, currentViewerCount);
      await saveAppSettingField('show_viewer_count', checked, checked ? '메인 UI에 시청자 수가 표시됩니다.' : '메인 UI에서 시청자 수가 숨겨집니다.');
    }

    async function loadPopupOnStartSetting() { await loadAllDockSettings(); }
    async function savePopupOnStartSetting(checked) {
      await saveAppSettingField('popup_on_start', checked, checked ? '프로그램 시작 시 독 팝업창이 자동으로 열립니다.' : '프로그램 시작 시 팝업창을 띄우지 않고 트레이로만 실행됩니다.');
    }

    async function loadShutdownNotifySetting() { await loadAllDockSettings(); }
    async function saveShutdownNotifySetting(checked) {
      await saveAppSettingField('notify_on_shutdown', checked, checked ? 'OBS 종료 또는 미감지로 자동 종료 시 알림을 받습니다.' : '자동 종료 시 알림 없이 조용히 종료됩니다.');
    }

    async function loadGPUSetting() { await loadAllDockSettings(); }
    async function saveGPUSetting(checked) {
      await saveAppSettingField('enable_gpu', checked, checked ? '웹뷰 GPU 하드웨어 가속이 활성화되었습니다. (창을 다시 열 때 적용)' : '웹뷰 GPU 하드웨어 가속이 비활성화되었습니다. (CPU 렌더링, 창을 다시 열 때 적용)');
    }

    async function loadExternalBrowserSetting() { await loadAllDockSettings(); }
    async function saveExternalBrowserSetting(checked) {
      await saveAppSettingField('external_browser_guard', checked, checked ? '외부 링크 클릭 시 기본 브라우저로 열립니다.' : '외부 링크 클릭 시 웹뷰 내에서 열립니다.');
    }

    async function loadBetaUpdatesSetting() { await loadAllDockSettings(); }
    async function saveBetaUpdatesSetting(checked) {
      const ok = await saveAppSettingField('enable_beta_updates', checked, checked ? '테스트(Beta) 버전 업데이트 확인이 활성화되었습니다.' : '안정화(정식) 버전만 업데이트를 받습니다.');
      if (ok && typeof checkForUpdate === 'function') checkForUpdate(true);
    }

    async function loadAutoCheckUpdateSetting() { await loadAllDockSettings(); }
    async function saveAutoCheckUpdateSetting(checked) {
      await saveAppSettingField('auto_check_update', checked, checked ? '앱 시작 시 최신 버전을 자동으로 확인합니다.' : '업데이트 자동 확인이 해제되었습니다. 수동으로만 확인합니다.');
    }

    // ===== [FEAT-06-03] 방송 정보 프리셋 관리 파이프라인 (최대 10개, 불러오기/저장/덮어쓰기) =====
    const MAX_PRESET_SLOTS = 10;
    const MAX_PRESET_NAME_LEN = 10;
    let broadcastPresets = [];
    let activePresetId = null;
    let currentSaveTab = 'new'; // 'new' | 'overwrite'


    async function loadBroadcastPresets() {
      try {
        const res = await apiFetch('/broadcast-presets');
        if (res.ok) {
          const data = await res.json();
          if (data.code === 200 && Array.isArray(data.presets)) {
            broadcastPresets = data.presets;
            updatePresetToolbarLabel();
          }
        }
      } catch (err) {
        console.warn('[Presets] Failed to load broadcast presets:', err);
      }
    }


    function updatePresetToolbarLabel() {
      const badgeEl = document.getElementById('preset-slot-count-badge');
      if (badgeEl) {
        badgeEl.textContent = `${broadcastPresets.length}/${MAX_PRESET_SLOTS}`;
      }
    }

    // ===== [FEAT-06-06] 썸네일 이미지 관리 & 멀티 비율 크롭 엔진 (Wallpaper Engine Fill 맞춤 방식) =====
    // ===== [FEAT-06-06] 썸네일 이미지 관리 & 멀티 비율 크롭 엔진 (단일 뷰 통합) =====
    let currentCropImage = null;
    let cropAspectRatio = '16:9'; // '16:9' | '1:1' | 'free'
    let cropBoxW = 320;
    let cropBoxH = 180;
    let cropZoom = 1.0; // 1.0x (Fill) ~ 3.0x
    let cropPanX = 0;
    let cropPanY = 0;
    let currentBaseScale = 1.0; // 자유 비율 리사이즈 시 이미지 크기 불변 고정용 기본 배율
    let activeCropDrag = null; // null | 'pan' | 'nw' | 'ne' | 'sw' | 'se' | 'n' | 's' | 'w' | 'e'
    let cropDragStart = { mouseX: 0, mouseY: 0, panX: 0, panY: 0, boxW: 0, boxH: 0 };


    function openThumbnailModal() {
      if (!checkLoginGuard('방송 썸네일 관리')) return;

      const modal = document.getElementById('modal-thumbnail');
      if (!modal) return;

      const currentThumb = currentLiveSetting.defaultThumbnailImageUrl;
      if (currentThumb) {
        // 기존 등록된 치지직 썸네일이 있는 경우: 즉시 크롭 뷰로 로드 (치지직은 16:9 고정)
        const img = new Image();
        img.crossOrigin = 'anonymous';
        img.onload = () => {
          startThumbnailCrop(img, 'chzzk_thumbnail');
        };
        img.onerror = () => {
          resetThumbnailModalToEmpty();
        };
        img.src = currentThumb;
      } else {
        resetThumbnailModalToEmpty();
      }

      modal.classList.remove('hidden');
    }


    function resetThumbnailModalToEmpty() {
      currentCropImage = null;
      currentBaseScale = 1.0;
      const cropImg = document.getElementById('thumb-crop-img');
      const box = document.getElementById('thumb-crop-box');
      const emptyEl = document.getElementById('thumb-empty-placeholder');
      const delBtn = document.getElementById('btn-thumbnail-delete');
      const controls = document.getElementById('thumb-crop-controls');

      if (cropImg) {
        cropImg.src = '';
        cropImg.classList.add('hidden');
      }
      if (box) box.style.display = 'none';
      if (emptyEl) emptyEl.classList.remove('hidden');
      if (delBtn) delBtn.classList.add('hidden');
      if (controls) controls.classList.add('opacity-40', 'pointer-events-none');
    }


    function closeThumbnailModal() {
      const modal = document.getElementById('modal-thumbnail');
      if (modal) modal.classList.add('hidden');
      activeCropDrag = null;
    }


    function triggerThumbnailFileInput() {
      const input = document.getElementById('thumbnail-file-input');
      if (input) {
        input.value = '';
        input.click();
      }
    }


    function deleteCurrentThumbnail() {
      currentLiveSetting.defaultThumbnailImageUrl = null;
      resetThumbnailModalToEmpty();
      updateThumbnailToolbarButton();
      checkBroadcastStateDirty();
      showToast('썸네일 삭제', 'info', '썸네일이 제거되었습니다. 하단의 [적용]을 누르면 반영됩니다.');
    }


    function handleThumbnailFileSelect(e) {
      const file = e.target.files && e.target.files[0];
      if (!file) return;

      // 1. 파일 크기 검증 (5MB)
      if (file.size > 5 * 1024 * 1024) {
        showToast('용량 초과', 'warn', '이미지 파일 크기는 5MB 이하여야 합니다.');
        e.target.value = '';
        return;
      }

      // 2. 확장자 검증
      const allowedExts = ['png', 'pjp', 'jfif', 'jpe', 'pjpeg', 'jpeg', 'jpg', 'gif', 'bmp', 'webp'];
      const ext = (file.name.split('.').pop() || '').toLowerCase();
      if (!allowedExts.includes(ext)) {
        showToast('형식 오류', 'warn', '지원되지 않는 이미지 파일 형식입니다.');
        e.target.value = '';
        return;
      }

      // 3. 브라우저 디코더로 유효성 검증 후 크롭 화면으로 즉시 전환
      const objectUrl = URL.createObjectURL(file);
      const img = new Image();
      img.onload = () => {
        startThumbnailCrop(img, file.name);
      };
      img.onerror = () => {
        showToast('이미지 오류', 'warn', '손상되었거나 읽을 수 없는 이미지 파일입니다.');
        URL.revokeObjectURL(objectUrl);
        e.target.value = '';
      };
      img.src = objectUrl;
    }


    function startThumbnailCrop(img, fileName) {
      currentCropImage = img;

      const emptyEl = document.getElementById('thumb-empty-placeholder');
      const cropImg = document.getElementById('thumb-crop-img');
      const box = document.getElementById('thumb-crop-box');
      const delBtn = document.getElementById('btn-thumbnail-delete');
      const controls = document.getElementById('thumb-crop-controls');

      if (emptyEl) emptyEl.classList.add('hidden');
      if (delBtn) delBtn.classList.remove('hidden');
      if (controls) controls.classList.remove('opacity-40', 'pointer-events-none');

      if (cropImg && box) {
        cropImg.src = img.src;
        cropImg.classList.remove('hidden');
        box.style.display = 'block';

        setCropAspectRatio('16:9');
        initCropBoxInteractions();
      }
    }

    // ── Wallpaper Engine Fill 스케일 & 여백 방지 클램핑 ──

    function getCropScaleInfo() {
      const natW = (currentCropImage && currentCropImage.naturalWidth) || 1;
      const natH = (currentCropImage && currentCropImage.naturalHeight) || 1;

      // Wallpaper Engine Fill: 자유 비율 모드 조절 시에는 기존 baseScale을 고정하여 이미지 크기 불변 유지
      if (!currentBaseScale || currentBaseScale <= 0) {
        currentBaseScale = Math.max(cropBoxW / natW, cropBoxH / natH);
      }
      const baseScale = currentBaseScale;
      const currentScale = baseScale * cropZoom;
      const renderedW = Math.round(natW * currentScale);
      const renderedH = Math.round(natH * currentScale);

      return {
        natW,
        natH,
        baseScale,
        currentScale,
        renderedW,
        renderedH,
        minPanX: cropBoxW - renderedW, // 음수 또는 0
        maxPanX: 0,
        minPanY: cropBoxH - renderedH, // 음수 또는 0
        maxPanY: 0
      };
    }


    function clampCropPan() {
      const info = getCropScaleInfo();
      cropPanX = Math.min(0, Math.max(info.minPanX, cropPanX));
      cropPanY = Math.min(0, Math.max(info.minPanY, cropPanY));
    }


    function applyCropTransform() {
      if (!currentCropImage) return;
      const info = getCropScaleInfo();
      const cropImg = document.getElementById('thumb-crop-img');
      const box = document.getElementById('thumb-crop-box');
      const stage = document.getElementById('thumb-stage-container');
      if (!box || !cropImg || !stage) return;

      const stageW = stage.clientWidth || 320;
      const stageH = stage.clientHeight || 220;

      // 박스가 스테이지를 벗어나지 않도록 클램핑
      cropBoxW = Math.min(cropBoxW, Math.max(60, stageW - 8));
      cropBoxH = Math.min(cropBoxH, Math.max(60, stageH - 8));

      const boxLeft = Math.round(Math.max(0, (stageW - cropBoxW) / 2));
      const boxTop = Math.round(Math.max(0, (stageH - cropBoxH) / 2));

      box.style.width = `${cropBoxW}px`;
      box.style.height = `${cropBoxH}px`;
      box.style.left = `${boxLeft}px`;
      box.style.top = `${boxTop}px`;

      cropImg.style.width = `${info.renderedW}px`;
      cropImg.style.height = `${info.renderedH}px`;
      cropImg.style.left = '0px';
      cropImg.style.top = '0px';
      cropImg.style.transform = `translate(${boxLeft + cropPanX}px, ${boxTop + cropPanY}px)`;
    }


    function resetCropFillPosition() {
      cropZoom = 1.0;
      const zoomSlider = document.getElementById('thumb-crop-zoom');
      const zoomLabel = document.getElementById('thumb-crop-zoom-label');
      if (zoomSlider) zoomSlider.value = 1;
      if (zoomLabel) zoomLabel.textContent = '1.0x';

      const info = getCropScaleInfo();
      // 크롭 박스 정중앙에 정렬
      cropPanX = Math.round((cropBoxW - info.renderedW) / 2);
      cropPanY = Math.round((cropBoxH - info.renderedH) / 2);
      clampCropPan();
      applyCropTransform();
    }


    function onCropZoomSliderChange(val) {
      const prevZoom = cropZoom;
      cropZoom = Math.max(1.0, Math.min(3.0, parseFloat(val) || 1.0));
      const zoomLabel = document.getElementById('thumb-crop-zoom-label');
      if (zoomLabel) zoomLabel.textContent = `${cropZoom.toFixed(1)}x`;

      // 줌 변경 시 박스 중심 기준으로 Pan 보정
      const zoomRatio = cropZoom / prevZoom;
      cropPanX = (cropPanX - cropBoxW / 2) * zoomRatio + cropBoxW / 2;
      cropPanY = (cropPanY - cropBoxH / 2) * zoomRatio + cropBoxH / 2;

      clampCropPan();
      applyCropTransform();
    }


    function setCropAspectRatio(ratio) {
      cropAspectRatio = ratio;
      const btn169 = document.getElementById('btn-crop-ratio-16-9');
      const btn11 = document.getElementById('btn-crop-ratio-1-1');
      const btnFree = document.getElementById('btn-crop-ratio-free');
      const box = document.getElementById('thumb-crop-box');
      const stage = document.getElementById('thumb-stage-container');

      const activeCls = 'px-2 py-0.5 rounded text-[10.5px] font-bold bg-[#1A2436] text-[#00FFA3] border border-[#00FFA3]/40 cursor-pointer transition-all';
      const inactiveCls = 'px-2 py-0.5 rounded text-[10.5px] font-medium text-slate-400 hover:text-white cursor-pointer transition-all border border-transparent';

      if (btn169) btn169.className = (ratio === '16:9') ? activeCls : inactiveCls;
      if (btn11) btn11.className = (ratio === '1:1') ? activeCls : inactiveCls;
      if (btnFree) btnFree.className = (ratio === 'free') ? activeCls : inactiveCls;

      const stageW = (stage && stage.clientWidth) ? stage.clientWidth : 320;
      const stageH = (stage && stage.clientHeight) ? stage.clientHeight : 220;

      if (ratio === '16:9') {
        let targetW = Math.min(320, stageW - 12);
        let targetH = Math.round(targetW * 9 / 16);
        if (targetH > stageH - 12) {
          targetH = stageH - 12;
          targetW = Math.round(targetH * 16 / 9);
        }
        cropBoxW = targetW;
        cropBoxH = targetH;
      } else if (ratio === '1:1') {
        const side = Math.min(200, Math.min(stageW - 12, stageH - 12));
        cropBoxW = side;
        cropBoxH = side;
      } else {
        // 자유 비율: 원본 이미지의 자연 비율(Aspect Ratio)을 초기 크기로 유지
        const natW = (currentCropImage && currentCropImage.naturalWidth) || 16;
        const natH = (currentCropImage && currentCropImage.naturalHeight) || 9;
        let targetW = Math.min(280, stageW - 20);
        let targetH = Math.round(targetW * natH / natW);
        if (targetH > stageH - 20) {
          targetH = stageH - 20;
          targetW = Math.round(targetH * natW / natH);
        }
        cropBoxW = Math.max(60, targetW);
        cropBoxH = Math.max(60, targetH);
      }

      const natW = (currentCropImage && currentCropImage.naturalWidth) || 1;
      const natH = (currentCropImage && currentCropImage.naturalHeight) || 1;
      currentBaseScale = Math.max(cropBoxW / natW, cropBoxH / natH);

      // 자유 모드에서만 리사이즈 핸들 노출
      if (box) {
        const sideHandles = box.querySelectorAll('.crop-handle');
        sideHandles.forEach(h => {
          h.style.display = (ratio === 'free') ? 'block' : 'none';
        });
      }

      resetCropFillPosition();
    }


    function initCropBoxInteractions() {
      const stage = document.getElementById('thumb-stage-container');
      const box = document.getElementById('thumb-crop-box');
      if (!stage || !box || box._hasInteractions) return;
      box._hasInteractions = true;

      stage.addEventListener('mousedown', (e) => {
        if (e.target.closest('#thumb-crop-box')) return;
        e.preventDefault();
        activeCropDrag = 'pan';
        cropDragStart = {
          mouseX: e.clientX,
          mouseY: e.clientY,
          panX: cropPanX,
          panY: cropPanY,
          boxW: cropBoxW,
          boxH: cropBoxH
        };
      });

      stage.addEventListener('wheel', (e) => {
        e.preventDefault();
        const delta = e.deltaY < 0 ? 0.1 : -0.1;
        const nextZoom = Math.max(1.0, Math.min(3.0, cropZoom + delta));
        const slider = document.getElementById('thumb-crop-zoom');
        if (slider) slider.value = nextZoom;
        onCropZoomSliderChange(nextZoom);
      }, { passive: false });

      // 마우스 드래그 시작 (Pan 또는 자유 모드 핸들 조절)
      box.addEventListener('mousedown', (e) => {
        e.preventDefault();
        e.stopPropagation();
        const handle = e.target.getAttribute('data-handle');
        activeCropDrag = handle || 'pan';
        cropDragStart = {
          mouseX: e.clientX,
          mouseY: e.clientY,
          panX: cropPanX,
          panY: cropPanY,
          boxW: cropBoxW,
          boxH: cropBoxH
        };
      });

      // 마우스 휠 줌 (1.0x ~ 3.0x)
      box.addEventListener('wheel', (e) => {
        e.preventDefault();
        const delta = e.deltaY < 0 ? 0.1 : -0.1;
        const nextZoom = Math.max(1.0, Math.min(3.0, cropZoom + delta));
        const slider = document.getElementById('thumb-crop-zoom');
        if (slider) slider.value = nextZoom;
        onCropZoomSliderChange(nextZoom);
      }, { passive: false });

      window.addEventListener('mousemove', (e) => {
        if (!activeCropDrag) return;

        const dx = e.clientX - cropDragStart.mouseX;
        const dy = e.clientY - cropDragStart.mouseY;

        if (activeCropDrag === 'pan') {
          // 이미지 이동 (상하/좌우 Pan)
          cropPanX = cropDragStart.panX + dx;
          cropPanY = cropDragStart.panY + dy;
          clampCropPan();
          applyCropTransform();
        } else if (cropAspectRatio === 'free') {
          // 자유 비율 핸들 리사이즈 (이미지 크기는 고정 유지하고 박스만 조절)
          const stage = document.getElementById('thumb-stage-container');
          const stageW = (stage && stage.clientWidth) ? stage.clientWidth : 320;
          const stageH = (stage && stage.clientHeight) ? stage.clientHeight : 220;
          const info = getCropScaleInfo();
          const minSize = 60;
          const maxW = Math.max(minSize, Math.min(info.renderedW, stageW - 8));
          const maxH = Math.max(minSize, Math.min(info.renderedH, stageH - 8));

          let newW = cropDragStart.boxW;
          let newH = cropDragStart.boxH;

          if (activeCropDrag.includes('e')) newW = Math.max(minSize, Math.min(maxW, cropDragStart.boxW + dx));
          if (activeCropDrag.includes('w')) newW = Math.max(minSize, Math.min(maxW, cropDragStart.boxW - dx));
          if (activeCropDrag.includes('s')) newH = Math.max(minSize, Math.min(maxH, cropDragStart.boxH + dy));
          if (activeCropDrag.includes('n')) newH = Math.max(minSize, Math.min(maxH, cropDragStart.boxH - dy));

          cropBoxW = Math.round(newW);
          cropBoxH = Math.round(newH);
          clampCropPan();
          applyCropTransform();
        }
      });

      window.addEventListener('mouseup', () => {
        activeCropDrag = null;
      });
    }


    async function confirmThumbnailCrop() {
      if (!currentCropImage) {
        showToast('알림', 'warn', '등록된 이미지가 없습니다. 이미지를 먼저 선택해주세요.');
        return;
      }

      const info = getCropScaleInfo();
      const btnApply = document.getElementById('btn-apply-crop');
      const spinner = document.getElementById('thumb-upload-spinner');
      const textSpan = document.getElementById('thumb-apply-text');

      if (btnApply) btnApply.disabled = true;
      if (spinner) spinner.classList.remove('hidden');
      const prevText = textSpan ? textSpan.textContent : '';
      if (textSpan) textSpan.textContent = '업로드 중...';

      try {
        const natW = info.natW;
        const natH = info.natH;
        const scale = info.currentScale;

        // 화면에 보이는 박스 기준 원본 이미지 소스 사각형 계산
        const srcX = Math.max(0, Math.min(natW, Math.round(-cropPanX / scale)));
        const srcY = Math.max(0, Math.min(natH, Math.round(-cropPanY / scale)));
        const srcW = Math.min(natW - srcX, Math.round(cropBoxW / scale));
        const srcH = Math.min(natH - srcY, Math.round(cropBoxH / scale));

        let outW = 1280;
        let outH = 720;
        let outFileName = 'thumbnail_1280x720.jpg';

        if (cropAspectRatio === '1:1') {
          outW = 1080;
          outH = 1080;
          outFileName = 'thumbnail_1080x1080.jpg';
        } else if (cropAspectRatio === 'free') {
          outW = Math.min(1920, Math.max(640, Math.round(srcW)));
          outH = Math.round(outW * (srcH / srcW));
          outFileName = 'thumbnail_custom.jpg';
        }

        const offCanvas = document.createElement('canvas');
        offCanvas.width = outW;
        offCanvas.height = outH;
        const offCtx = offCanvas.getContext('2d');

        offCtx.drawImage(currentCropImage, srcX, srcY, srcW, srcH, 0, 0, outW, outH);

        const blob = await new Promise((resolve) => offCanvas.toBlob(resolve, 'image/jpeg', 0.92));
        if (!blob) throw new Error('이미지 변환에 실패했습니다.');

        const formData = new FormData();
        formData.append('file', blob, outFileName);

        const uploadRes = await fetch('/upload-thumbnail', {
          method: 'POST',
          headers: {
            'X-Requested-With': 'ChzzkDock'
          },
          body: formData
        });

        const uploadData = await uploadRes.json();
        if (!uploadRes.ok || uploadData.code !== 200 || !uploadData.imageUrl) {
          throw new Error(uploadData.message || '썸네일 업로드에 실패했습니다.');
        }

        currentLiveSetting.defaultThumbnailImageUrl = uploadData.imageUrl;
        updateThumbnailToolbarButton();
        checkBroadcastStateDirty();
        closeThumbnailModal();
        showToast('썸네일 준비 완료', 'success', '새 썸네일이 준비되었습니다. 하단의 [적용]을 누르면 치지직에 반영됩니다.');
      } catch (err) {
        console.error('[Thumbnail] Crop/Upload error:', err);
        showToast('썸네일 처리 실패', 'error', err.message || '오류가 발생했습니다.');
      } finally {
        if (btnApply) btnApply.disabled = false;
        if (spinner) spinner.classList.add('hidden');
        if (textSpan) textSpan.textContent = prevText || '적용';
      }
    }

    // ── 1. 프리셋 불러오기 모달 제어 ──

    function openPresetLoadModal() {
      const modal = document.getElementById('modal-preset-load');
      const slotEl = document.getElementById('load-modal-slot-count');
      if (!modal) return;

      if (slotEl) {
        slotEl.textContent = `${broadcastPresets.length} / ${MAX_PRESET_SLOTS}`;
        slotEl.className = (broadcastPresets.length >= MAX_PRESET_SLOTS)
          ? 'text-[9.5px] px-1.5 py-0.2 rounded bg-rose-950 text-rose-400 font-mono border border-rose-800/40'
          : 'text-[9.5px] px-1.5 py-0.2 rounded bg-[#161F2E] text-slate-400 font-mono border border-[#243044]';
      }

      renderPresetLoadList();
      modal.classList.remove('hidden');
    }


    function closePresetLoadModal() {
      const modal = document.getElementById('modal-preset-load');
      if (modal) modal.classList.add('hidden');
    }


    function renderPresetLoadList() {
      const listEl = document.getElementById('preset-load-list');
      if (!listEl) return;

      listEl.innerHTML = '';
      if (broadcastPresets.length === 0) {
        listEl.innerHTML = `
          <div class="py-6 px-3 text-center rounded-lg bg-[#0E131A] border border-dashed border-[#1E2738] space-y-2">
            <span class="text-2xl select-none">📋</span>
            <div class="text-xs text-slate-300 font-medium">저장된 방송 프리셋이 없습니다.</div>
            <div class="text-[10.5px] text-slate-500">현재 방송 정보 설정을 [💾 저장] 버튼을 눌러 등록해 보세요.</div>
          </div>
        `;
        return;
      }

      broadcastPresets.forEach(preset => {
        const isActive = (preset.id === activePresetId);
        const card = document.createElement('div');
        card.className = `p-2.5 rounded-lg border transition-all space-y-1.5 ${
          isActive 
            ? 'bg-[#0E131A] border-[#00FFA3]/50 shadow-[0_0_10px_rgba(0,255,163,0.1)]' 
            : 'bg-[#0E131A] border-[#1E2738] hover:border-[#2A374E]'
        }`;

        card.innerHTML = `
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1.5 min-w-0">
              <span class="font-bold text-xs text-white truncate max-w-[140px]">${escapeHtml(preset.name)}</span>
              ${isActive ? '<span class="text-[9px] px-1 py-0.2 rounded font-bold bg-[#00FFA3]/20 text-[#00FFA3] border border-[#00FFA3]/40">현재 활성</span>' : ''}
            </div>
            <div class="flex items-center gap-1 shrink-0">
              <button type="button" onclick="promptRenamePreset('${preset.id}')" class="px-1.5 py-0.5 rounded text-[10px] bg-[#161F2E] hover:bg-[#1E2738] text-slate-300 hover:text-white border border-[#243044] cursor-pointer" title="프리셋 이름 변경">
                ✏️ 수정
              </button>
              <button type="button" onclick="requestDeletePreset('${preset.id}')" class="px-1.5 py-0.5 rounded text-[10px] bg-[#161F2E] hover:bg-rose-950/80 text-slate-400 hover:text-rose-300 border border-[#243044] hover:border-rose-800/50 cursor-pointer" title="프리셋 삭제">
                🗑️ 삭제
              </button>
            </div>
          </div>

          <div id="rename-box-${preset.id}" class="hidden flex items-center gap-1 pt-0.5">
            <input id="rename-input-${preset.id}" type="text" maxlength="10" value="${escapeHtml(preset.name)}" class="chzzk-input text-xs !py-0.5 flex-1" />
            <button type="button" onclick="submitRenamePreset('${preset.id}')" class="px-2 py-0.5 rounded bg-[#00FFA3] text-black text-[11px] font-bold cursor-pointer">확인</button>
            <button type="button" onclick="cancelRenamePreset('${preset.id}')" class="px-2 py-0.5 rounded bg-[#161F2E] text-slate-300 text-[11px] cursor-pointer">취소</button>
          </div>

          <div class="text-[11px] text-slate-300 truncate font-medium">
            ${escapeHtml(preset.title)}
          </div>
          <div class="flex items-center gap-1.5 text-[10px] flex-wrap">
            <span class="text-[#00FFA3] bg-[#00FFA3]/10 px-1 py-0.2 rounded border border-[#00FFA3]/20 font-medium">${escapeHtml(preset.category_name || '-')}</span>
            ${(preset.tags || []).map(t => `<span class="text-slate-400 bg-[#161F2E] px-1 py-0.2 rounded">#${escapeHtml(t)}</span>`).join('')}
          </div>

          <div class="pt-0.5 flex items-center gap-1">
            <button type="button" onclick="applyPresetFromModal('${preset.id}')" class="flex-1 py-1 rounded bg-[#161F2E] hover:bg-[#00FFA3] hover:text-black border border-[#243044] hover:border-[#00FFA3] text-slate-200 text-xs font-semibold transition-all cursor-pointer flex items-center justify-center gap-1">
              <span>📥</span> <span>이 설정으로 불러오기</span>
            </button>
            <button type="button" onclick="openPresetSaveModal('${preset.id}')" class="px-2 py-1 rounded bg-[#161F2E] hover:bg-[#1E2738] border border-[#243044] text-slate-300 text-xs font-medium transition-colors cursor-pointer flex items-center gap-1" title="현재 설정을 이 프리셋에 덮어쓰기">
              <span>🔄</span> <span>덮어쓰기</span>
            </button>
          </div>
        `;
        listEl.appendChild(card);
      });
    }


    function promptRenamePreset(id) {
      const box = document.getElementById(`rename-box-${id}`);
      if (!box) return;
      const isHidden = box.classList.contains('hidden');
      box.classList.toggle('hidden', !isHidden);
      if (isHidden) {
        const input = document.getElementById(`rename-input-${id}`);
        if (input) setTimeout(() => input.focus(), 50);
      }
    }


    function cancelRenamePreset(id) {
      const box = document.getElementById(`rename-box-${id}`);
      if (box) box.classList.add('hidden');
    }


    async function submitRenamePreset(id) {
      const input = document.getElementById(`rename-input-${id}`);
      if (!input) return;
      const newName = input.value.trim();

      if (!newName) {
        showToast('수정 오류', 'warn', '프리셋 이름을 입력해 주세요.');
        return;
      }
      if (newName.length > MAX_PRESET_NAME_LEN) {
        showToast('수정 오류', 'warn', `프리셋 이름은 최대 ${MAX_PRESET_NAME_LEN}자까지 가능합니다.`);
        return;
      }
      if (broadcastPresets.some(p => p.id !== id && p.name === newName)) {
        showToast('수정 오류', 'warn', '이미 같은 이름의 프리셋이 존재합니다.');
        return;
      }

      const target = broadcastPresets.find(p => p.id === id);
      if (!target) return;
      target.name = newName;

      try {
        const res = await apiFetch('/save-broadcast-presets', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ presets: broadcastPresets })
        });
        if (res.ok) {
          updatePresetToolbarLabel();
          renderPresetLoadList();
          showToast('프리셋 이름 변경', 'success', `'${newName}'(으)로 이름이 변경되었습니다.`);
        } else {
          showToast('저장 실패', 'error', '프리셋 이름 저장에 실패했습니다.');
        }
      } catch (err) {
        showToast('저장 실패', 'error', err.message);
      }
    }


    async function requestDeletePreset(id) {
      const target = broadcastPresets.find(p => p.id === id);
      if (!target) return;

      const ok = await showConfirmModal({
        title: '프리셋 삭제 확인',
        message: `'${target.name}' 프리셋을 정말 삭제하시겠습니까?`,
        submessage: '삭제된 프리셋은 복구할 수 없습니다.',
        confirmText: '삭제',
        cancelText: '취소',
        danger: true,
        icon: '🗑️'
      });
      if (!ok) return;

      const nextPresets = broadcastPresets.filter(p => p.id !== id);
      try {
        const res = await apiFetch('/save-broadcast-presets', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ presets: nextPresets })
        });
        if (res.ok) {
          broadcastPresets = nextPresets;
          if (activePresetId === id) activePresetId = null;
          updatePresetToolbarLabel();
          renderPresetLoadList();
          showToast('프리셋 삭제 완료', 'info', `'${target.name}' 프리셋이 삭제되었습니다.`);
        } else {
          showToast('삭제 실패', 'error', '프리셋 저장 중 오류가 발생했습니다.');
        }
      } catch (err) {
        showToast('삭제 실패', 'error', err.message);
      }
    }


    function applyPresetFromModal(id) {
      applyPreset(id);
      closePresetLoadModal();
    }


    function applyPreset(id) {
      const target = broadcastPresets.find(p => p.id === id);
      if (!target) return;

      activePresetId = id;

      // 1. 방송 제목 적용
      const elTitle = document.getElementById('live-title');
      const elTitleClear = document.getElementById('title-clear');
      const countEl = document.getElementById('title-count');
      if (elTitle) {
        elTitle.value = target.title || '';
        if (countEl) countEl.textContent = `${elTitle.value.length}/50`;
        if (elTitleClear) elTitleClear.style.display = elTitle.value ? 'block' : 'none';
        flashElement(elTitle);
      }

      // 2. 카테고리 적용 (같이보기 락이 걸려있지 않은 경우에만)
      const elCatSearch = document.getElementById('category-search');
      const elCatId = document.getElementById('category-id');
      const elCatType = document.getElementById('category-type');
      const elCatClear = document.getElementById('category-clear');
      if (elCatSearch && !elCatSearch.disabled) {
        elCatSearch.value = target.category_name || '';
        if (elCatId) elCatId.value = target.category_id || '';
        if (elCatType) elCatType.value = target.category_type || '';
        if (elCatClear) elCatClear.style.display = (target.category_id || target.category_name) ? 'block' : 'none';
        flashElement(elCatSearch);
      }

      // 3. 태그 적용 (기존 캠페인 태그 락 유지하면서 교체)
      currentTags = Array.isArray(target.tags) ? [...target.tags] : [];
      renderTags();
      const tagContainer = document.getElementById('tag-container');
      if (tagContainer) flashElement(tagContainer);

      // 4. Dirty 상태 갱신
      checkBroadcastStateDirty();
      updatePresetToolbarLabel();
      showToast('프리셋 불러오기 완료', 'info', `'${target.name}' 설정을 불러왔습니다. 제목 수정 후 [방송 정보 적용]을 누르세요.`);
    }


    function flashElement(el) {
      if (!el) return;
      el.classList.add('pulse-feedback');
      setTimeout(() => el.classList.remove('pulse-feedback'), 400);
    }

    // ── 2. 프리셋 저장/덮어쓰기 모달 제어 ──

    function openPresetSaveModal(presetIdToOverwrite = null) {
      closePresetLoadModal();
      const modal = document.getElementById('modal-preset-save');
      const slotEl = document.getElementById('save-modal-slot-count');
      if (!modal) return;

      if (slotEl) {
        slotEl.textContent = `${broadcastPresets.length} / ${MAX_PRESET_SLOTS}`;
        slotEl.className = (broadcastPresets.length >= MAX_PRESET_SLOTS)
          ? 'text-[9.5px] px-1.5 py-0.2 rounded bg-rose-950 text-rose-400 font-mono border border-rose-800/40'
          : 'text-[9.5px] px-1.5 py-0.2 rounded bg-[#161F2E] text-slate-400 font-mono border border-[#243044]';
      }

      // 현재 방송 설정 스냅샷 미리보기 채우기
      const curTitle = (document.getElementById('live-title')?.value || '').trim();
      const curCat = (document.getElementById('category-search')?.value || '').trim();

      const previewTitle = document.getElementById('save-preview-title');
      const previewCat = document.getElementById('save-preview-cat');
      const previewTags = document.getElementById('save-preview-tags');

      if (previewTitle) previewTitle.textContent = curTitle || '(제목 없음)';
      if (previewCat) previewCat.textContent = curCat || '(카테고리 없음)';
      if (previewTags) {
        previewTags.innerHTML = '';
        if (currentTags.length === 0) {
          previewTags.innerHTML = '<span class="text-slate-500">(태그 없음)</span>';
        } else {
          currentTags.forEach(t => {
            previewTags.innerHTML += `<span class="px-1.5 py-0.2 rounded bg-[#161F2E] border border-[#243044] text-[#00FFA3] text-[9.5px]">#${escapeHtml(t)}</span>`;
          });
        }
      }

      // 덮어쓰기 셀렉트박스 옵션 구성
      populateOverwriteSelect(presetIdToOverwrite);

      // 슬롯이 가득 찼거나 특정 대상을 덮어쓰기 위해 열었으면 덮어쓰기 탭 활성화
      if (presetIdToOverwrite || broadcastPresets.length >= MAX_PRESET_SLOTS) {
        switchPresetSaveTab('overwrite');
      } else {
        switchPresetSaveTab('new');
      }

      const inputName = document.getElementById('input-save-new-name');
      if (inputName) {
        inputName.value = '';
        validatePresetSaveNew();
      }

      modal.classList.remove('hidden');
    }


    function openPresetSaveModalFromLoad() {
      openPresetSaveModal();
    }


    function closePresetSaveModal() {
      const modal = document.getElementById('modal-preset-save');
      if (modal) modal.classList.add('hidden');
    }


    function populateOverwriteSelect(selectedId = null) {
      const selectEl = document.getElementById('select-preset-overwrite');
      if (!selectEl) return;

      selectEl.innerHTML = '';
      if (broadcastPresets.length === 0) {
        selectEl.innerHTML = '<option value="">(덮어쓸 기존 프리셋 없음)</option>';
        onSelectOverwritePreset('');
        return;
      }

      broadcastPresets.forEach(preset => {
        const opt = document.createElement('option');
        opt.value = preset.id;
        opt.textContent = `${preset.name} - [${preset.category_name || '카테고리 없음'}] ${preset.title}`;
        if (selectedId && preset.id === selectedId) {
          opt.selected = true;
        } else if (!selectedId && preset.id === activePresetId) {
          opt.selected = true;
        }
        selectEl.appendChild(opt);
      });

      onSelectOverwritePreset(selectEl.value);
    }


    function onSelectOverwritePreset(presetId) {
      const section = document.getElementById('overwrite-comparison-section');
      const target = broadcastPresets.find(p => p.id === presetId);
      if (!target || !section) {
        if (section) section.classList.add('hidden');
        return;
      }

      section.classList.remove('hidden');
      const elPrevTitle = document.getElementById('ow-prev-title');
      const elPrevCat = document.getElementById('ow-prev-cat');
      const elPrevTags = document.getElementById('ow-prev-tags');
      if (elPrevTitle) elPrevTitle.textContent = target.title || '(제목 없음)';
      if (elPrevCat) elPrevCat.textContent = target.category_name || '(카테고리 없음)';
      if (elPrevTags) elPrevTags.textContent = (target.tags && target.tags.length > 0) ? target.tags.map(t => '#' + t).join(' ') : '(태그 없음)';

      const curTitle = (document.getElementById('live-title')?.value || '').trim();
      const curCat = (document.getElementById('category-search')?.value || '').trim();
      const elCurrTitle = document.getElementById('ow-curr-title');
      const elCurrCat = document.getElementById('ow-curr-cat');
      const elCurrTags = document.getElementById('ow-curr-tags');
      if (elCurrTitle) elCurrTitle.textContent = curTitle || '(제목 없음)';
      if (elCurrCat) elCurrCat.textContent = curCat || '(카테고리 없음)';
      if (elCurrTags) elCurrTags.textContent = (currentTags.length > 0) ? currentTags.map(t => '#' + t).join(' ') : '(태그 없음)';
    }


    function switchPresetSaveTab(tab) {
      currentSaveTab = tab;
      const btnNew = document.getElementById('tab-btn-save-new');
      const btnOw = document.getElementById('tab-btn-save-overwrite');
      const contentNew = document.getElementById('tab-content-save-new');
      const contentOw = document.getElementById('tab-content-save-overwrite');

      if (tab === 'new') {
        if (btnNew) {
          btnNew.className = 'py-1 text-[11px] font-semibold rounded transition-all text-center bg-[#1B2535] text-[#00FFA3] shadow-sm cursor-pointer';
        }
        if (btnOw) {
          btnOw.className = 'py-1 text-[11px] font-semibold rounded transition-all text-center text-slate-400 hover:text-white cursor-pointer';
        }
        if (contentNew) contentNew.classList.remove('hidden');
        if (contentOw) contentOw.classList.add('hidden');
        validatePresetSaveNew();
      } else {
        if (btnNew) {
          btnNew.className = 'py-1 text-[11px] font-semibold rounded transition-all text-center text-slate-400 hover:text-white cursor-pointer';
        }
        if (btnOw) {
          btnOw.className = 'py-1 text-[11px] font-semibold rounded transition-all text-center bg-[#1B2535] text-[#00FFA3] shadow-sm cursor-pointer';
        }
        if (contentNew) contentNew.classList.add('hidden');
        if (contentOw) contentOw.classList.remove('hidden');
        validatePresetSaveOverwrite();
      }
    }


    function validatePresetSaveNew() {
      if (currentSaveTab !== 'new') return;
      const nameInput = document.getElementById('input-save-new-name');
      const counter = document.getElementById('save-new-name-counter');
      const valMsg = document.getElementById('save-preset-validation-msg');
      const valText = document.getElementById('save-preset-validation-text');
      const btnConfirm = document.getElementById('btn-confirm-save-action');
      if (!nameInput || !btnConfirm) return;

      const name = nameInput.value.trim();
      if (counter) counter.textContent = `${name.length} / ${MAX_PRESET_NAME_LEN}자`;

      const curTitle = (document.getElementById('live-title')?.value || '').trim();
      const curCat = (document.getElementById('category-search')?.value || '').trim();

      let err = null;
      if (broadcastPresets.length >= MAX_PRESET_SLOTS) {
        err = `슬롯이 가득 찼습니다 (최대 ${MAX_PRESET_SLOTS}개). 기존 프리셋에 덮어쓰거나 삭제하세요.`;
      } else if (!name) {
        err = '프리셋 이름을 입력해 주세요 (최대 10자).';
      } else if (name.length > MAX_PRESET_NAME_LEN) {
        err = `프리셋 이름은 최대 ${MAX_PRESET_NAME_LEN}자까지 가능합니다.`;
      } else if (broadcastPresets.some(p => p.name === name)) {
        err = '이미 같은 이름의 프리셋이 존재합니다.';
      } else if (!curTitle) {
        err = '현재 방송 제목이 비어있습니다. 제목을 먼저 입력하세요.';
      } else if (!curCat) {
        err = '현재 카테고리가 비어있습니다. 카테고리를 먼저 선택하세요.';
      } else if (currentTags.length > 5) {
        err = '태그는 최대 5개까지만 등록할 수 있습니다.';
      }

      if (err) {
        if (valMsg) valMsg.classList.remove('hidden');
        if (valText) valText.textContent = err;
        btnConfirm.disabled = true;
      } else {
        if (valMsg) valMsg.classList.add('hidden');
        btnConfirm.disabled = false;
      }
    }


    function validatePresetSaveOverwrite() {
      if (currentSaveTab !== 'overwrite') return;
      const valMsg = document.getElementById('save-preset-validation-msg');
      const valText = document.getElementById('save-preset-validation-text');
      const btnConfirm = document.getElementById('btn-confirm-save-action');
      if (!btnConfirm) return;

      const targetId = document.getElementById('select-preset-overwrite')?.value;
      const curTitle = (document.getElementById('live-title')?.value || '').trim();
      const curCat = (document.getElementById('category-search')?.value || '').trim();

      let err = null;
      if (!targetId || broadcastPresets.length === 0) {
        err = '덮어쓸 대상 프리셋이 없습니다.';
      } else if (!curTitle) {
        err = '현재 방송 제목이 비어있습니다.';
      } else if (!curCat) {
        err = '현재 카테고리가 비어있습니다.';
      }

      if (err) {
        if (valMsg) valMsg.classList.remove('hidden');
        if (valText) valText.textContent = err;
        btnConfirm.disabled = true;
      } else {
        if (valMsg) valMsg.classList.add('hidden');
        btnConfirm.disabled = false;
      }
    }


    async function executePresetSaveAction() {
      const curTitle = (document.getElementById('live-title')?.value || '').trim();
      const curCatSearch = (document.getElementById('category-search')?.value || '').trim();
      const curCatId = (document.getElementById('category-id')?.value || '').trim();
      const curCatType = (document.getElementById('category-type')?.value || '').trim();

      if (!curTitle || !curCatSearch) {
        showToast('저장 불가', 'warn', '방송 제목과 카테고리를 먼저 설정해 주세요.');
        return;
      }

      if (currentSaveTab === 'new') {
        const nameInput = document.getElementById('input-save-new-name');
        const name = nameInput ? nameInput.value.trim() : '';
        if (!name) {
          validatePresetSaveNew();
          return;
        }
        if (broadcastPresets.length >= MAX_PRESET_SLOTS) {
          showToast('슬롯 초과', 'warn', `프리셋은 최대 ${MAX_PRESET_SLOTS}개까지만 등록할 수 있습니다.`);
          return;
        }

        const newPreset = {
          id: 'preset_' + Date.now(),
          name: name,
          title: curTitle,
          category_id: curCatId,
          category_type: curCatType,
          category_name: curCatSearch,
          tags: [...currentTags]
        };

        const nextPresets = [...broadcastPresets, newPreset];
        try {
          const res = await apiFetch('/save-broadcast-presets', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ presets: nextPresets })
          });
          if (res.ok) {
            broadcastPresets = nextPresets;
            activePresetId = newPreset.id;
            updatePresetToolbarLabel();
            closePresetSaveModal();
            showToast('프리셋 등록 완료', 'success', `'${name}' 프리셋이 성공적으로 저장되었습니다!`);
          } else {
            const data = await res.json().catch(() => ({}));
            showToast('저장 실패', 'error', data.message || '프리셋 저장 중 오류가 발생했습니다.');
          }
        } catch (err) {
          showToast('저장 실패', 'error', err.message);
        }
      } else {
        // 기존 덮어쓰기 모드
        const targetId = document.getElementById('select-preset-overwrite')?.value;
        const target = broadcastPresets.find(p => p.id === targetId);
        if (!target) {
          showToast('덮어쓰기 오류', 'warn', '덮어쓸 대상 프리셋을 선택해 주세요.');
          return;
        }

        const ok = await showConfirmModal({
          title: '프리셋 덮어쓰기 확인',
          message: `'${target.name}' 프리셋의 설정을 현재 방송 정보로 덮어쓰시겠습니까?`,
          submessage: '선택한 프리셋의 제목, 카테고리, 태그가 현재 설정으로 즉시 변경됩니다.',
          confirmText: '덮어쓰기',
          cancelText: '취소',
          danger: false,
          icon: '🔄'
        });
        if (!ok) return;

        target.title = curTitle;
        target.category_id = curCatId;
        target.category_type = curCatType;
        target.category_name = curCatSearch;
        target.tags = [...currentTags];

        try {
          const res = await apiFetch('/save-broadcast-presets', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ presets: broadcastPresets })
          });
          if (res.ok) {
            activePresetId = target.id;
            updatePresetToolbarLabel();
            closePresetSaveModal();
            showToast('프리셋 덮어쓰기 완료', 'success', `'${target.name}' 프리셋이 현재 설정으로 갱신되었습니다!`);
          } else {
            const data = await res.json().catch(() => ({}));
            showToast('저장 실패', 'error', data.message || '프리셋 저장 중 오류가 발생했습니다.');
          }
        } catch (err) {
          showToast('저장 실패', 'error', err.message);
        }
      }
    }

    // ===== 소프트웨어 자동 업데이트 파이프라인 =====
    let cachedUpdateInfo = null;
    let updatePollTimer = null;
    let isCheckingUpdate = false;


    async function checkForUpdate(manual = false) {
      if (!manual) {
        try {
          const sRes = await apiFetch('/auto-check-update-status');
          if (sRes.ok) {
            const sData = await sRes.json();
            if (sData && sData.auto_check_update === false) {
              return; // 자동 확인 옵션이 꺼져 있으면 시작 시 자동 확인 건너뜀
            }
          }
        } catch (_) {}
      }

      if (isCheckingUpdate) return;
      isCheckingUpdate = true;

      const btnCheck = document.getElementById('btn-check-update');
      const badgeStatus = document.getElementById('badge-update-status');
      const labelStatus = document.getElementById('label-update-status');
      const btnRun = document.getElementById('btn-run-update');
      const labelRun = document.getElementById('label-run-update');

      if (btnCheck) {
        btnCheck.disabled = true;
        if (manual) {
          btnCheck.innerHTML = '<span>⏳</span> <span>확인 중...</span>';
        }
      }

      try {
        const res = await apiFetch(`/check-update?force=${manual ? 'true' : 'false'}`);
        if (!res.ok) {
          if (manual) {
            showToast('업데이트 확인 실패', 'error', '서버 통신에 실패했습니다.');
          } else {
            console.warn('[Updater] Silent startup check failed with HTTP status:', res.status);
          }
          return;
        }
        const data = await res.json();
        if (data.code === 200 && data.info) {
          cachedUpdateInfo = data.info;
          if (data.info.has_update) {
            if (badgeStatus) {
              badgeStatus.className = 'px-2 py-0.5 rounded text-[9.5px] font-bold bg-amber-950/80 text-amber-400 border border-amber-500/30';
              badgeStatus.textContent = `${data.info.latest_version} 출시`;
            }
            if (labelStatus) {
              labelStatus.innerHTML = `현재: <b class="text-slate-300 font-mono">${data.info.current_version}</b> ➔ 최신: <b class="text-[#00FFA3] font-mono">${data.info.latest_version}</b>`;
            }
            if (btnRun) {
              btnRun.classList.remove('hidden');
              if (labelRun) labelRun.textContent = `${data.info.latest_version} 지금 업데이트`;
            }

            if (data.info.should_prompt) {
              openUpdateModal();
            }
          } else {
            if (badgeStatus) {
              badgeStatus.className = 'px-2 py-0.5 rounded text-[9.5px] font-bold bg-emerald-950/80 text-emerald-400 border border-emerald-500/30';
              badgeStatus.textContent = '최신 상태';
            }
            if (labelStatus) {
              labelStatus.innerHTML = `현재 버전: <b class="text-slate-300 font-mono">${data.info.current_version}</b> (최신)`;
            }
            if (btnRun) btnRun.classList.add('hidden');
            if (manual) {
              showToast('✅ 최신 버전 사용 중', 'info', `현재 최신 버전(${data.info.current_version})을 사용하고 있습니다.`);
            }
          }
        } else if (manual) {
          showToast('업데이트 확인 안내', 'warn', data.message || '최신 버전을 확인할 수 없습니다.');
        } else {
          console.warn('[Updater] Silent startup check returned non-200 code:', data.message);
        }
      } catch (err) {
        if (manual) {
          showToast('업데이트 확인 오류', 'error', err.message);
        } else {
          console.warn('[Updater] Silent startup check network exception:', err.message);
        }
      } finally {
        isCheckingUpdate = false;
        if (btnCheck) {
          btnCheck.disabled = false;
          btnCheck.innerHTML = '<span>🔍</span> <span>업데이트 확인</span>';
        }
      }
    }


    function skipCurrentUpdateVersion() {
      if (cachedUpdateInfo?.latest_version) {
        apiFetch('/save-skip-update', {
          method: 'POST',
          body: JSON.stringify({ skip_update_version: cachedUpdateInfo.latest_version })
        }).catch(() => {});
      }
      closeUpdateModal();
      showToast('업데이트 건너뛰기', 'info', `${cachedUpdateInfo?.latest_version || ''} 버전 알림을 건너뜁니다.`);
    }


    function startUpdateDownload() {
      const promptView = document.getElementById('update-prompt-view');
      const progressView = document.getElementById('update-progress-view');
      if (promptView) promptView.classList.add('hidden');
      if (progressView) progressView.classList.remove('hidden');
      executeInstall();
    }


    async function executeInstall() {
      const cancelBtn = document.getElementById('btn-update-cancel');
      const progressBar = document.getElementById('update-progress-bar');
      const percentText = document.getElementById('update-percent-text');
      const statusText = document.getElementById('update-status-text');
      const sizeDetail = document.getElementById('update-size-detail');
      const speedDetail = document.getElementById('update-speed-detail');
      const noticeBox = document.getElementById('update-notice-box');
      const noticeMsg = document.getElementById('update-notice-msg');

      if (cancelBtn) cancelBtn.textContent = '백그라운드에서 진행';

      if (cachedUpdateInfo?.already_downloaded) {
        if (statusText) statusText.innerHTML = `<span class="w-1.5 h-1.5 rounded-full bg-[#00FFA3]"></span> 보관된 인스톨러로 즉시 설치 실행 중...`;
        if (progressBar) progressBar.style.width = '100%';
        if (percentText) percentText.textContent = '100%';
      }

      try {
        const res = await apiFetch('/execute-update', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            download_url: cachedUpdateInfo?.download_url || '',
            latest_version: cachedUpdateInfo?.latest_version || ''
          })
        });
        if (!res.ok) {
          const data = await res.json().catch(() => ({}));
          throw new Error(data.message || '업데이트 실행 요청 실패');
        }

        if (updatePollTimer) clearInterval(updatePollTimer);
        updatePollTimer = setInterval(async () => {
          try {
            const pRes = await apiFetch('/update-status');
            if (!pRes.ok) return;
            const pData = await pRes.json();
            const p = pData.progress;
            if (!p) return;

            if (p.error) {
              clearInterval(updatePollTimer);
              if (progressBar) {
                progressBar.style.width = '100%';
                progressBar.className = 'h-full bg-rose-500 rounded-full';
              }
              if (percentText) {
                percentText.textContent = '오류';
                percentText.className = 'font-bold text-rose-400';
              }
              if (statusText) statusText.innerHTML = `<span class="w-1.5 h-1.5 rounded-full bg-rose-500"></span> 실패`;
              if (noticeBox && noticeMsg) {
                noticeBox.classList.remove('hidden');
                noticeBox.className = 'p-2.5 rounded-lg bg-rose-500/10 border border-rose-500/30 text-[11px] text-rose-300 leading-relaxed';
                noticeMsg.textContent = p.error;
              }
              if (cancelBtn) cancelBtn.textContent = '닫기';
              return;
            }

            if (progressBar) progressBar.style.width = p.percent + '%';
            if (percentText) percentText.textContent = p.percent + '%';
            if (sizeDetail && p.total > 0) {
              const recMB = (p.received / (1024 * 1024)).toFixed(1);
              const totMB = (p.total / (1024 * 1024)).toFixed(1);
              sizeDetail.textContent = `${recMB} MB / ${totMB} MB`;
            }
            if (speedDetail && p.speed_bps > 0) {
              const spdMB = (p.speed_bps / (1024 * 1024)).toFixed(1);
              speedDetail.textContent = `~ ${spdMB} MB/s`;
            }

            if (p.done) {
              clearInterval(updatePollTimer);
              if (statusText) statusText.innerHTML = `<span class="w-1.5 h-1.5 rounded-full bg-[#00FFA3]"></span> 설치 프로그램 실행 중...`;
              if (noticeBox && noticeMsg) {
                noticeBox.classList.remove('hidden');
                noticeBox.className = 'p-2.5 rounded-lg bg-[#00FFA3]/10 border border-[#00FFA3]/30 text-[11px] text-slate-200 leading-relaxed';
                noticeMsg.innerHTML = '✨ <b>다운로드가 완료되었습니다.</b><br>설치 프로그램이 실행되며 기존 독이 자동으로 닫힙니다.';
              }
              if (cancelBtn) cancelBtn.textContent = '닫기';
            }
          } catch (e) {
            console.warn('Update poll error:', e);
          }
        }, 300);
      } catch (err) {
        showToast('업데이트 시작 실패', 'error', err.message);
        if (cancelBtn) cancelBtn.textContent = '닫기';
      }
    }

    // ===== 설정창 아코디언 그룹 접기/펼치기 제어 =====

    function toggleSettingsGroup(group) {
      const content = document.getElementById(`group-${group}-content`);
      const arrow = document.getElementById(`group-${group}-arrow`);
      if (!content) return;
      const isHidden = content.classList.contains('hidden');
      if (isHidden) {
        content.classList.remove('hidden');
        if (arrow) arrow.style.transform = 'rotate(0deg)';
      } else {
        content.classList.add('hidden');
        if (arrow) arrow.style.transform = 'rotate(-90deg)';
      }
    }

    // ===== Webview Login (이벤트 기반 비동기 대기 / 더블클릭 방지) =====


    // ===== Account Logout & Session Clear (Option B: 깔끔한 계정 전환) =====

    function updateObsScriptUI() {
      const btn = document.getElementById('btn-obs-script-action');
      if (!btn) return;

      if (currentObsStatus.installed) {
        if (currentObsStatus.needs_update) {
          btn.textContent = '🔄 스크립트 업데이트';
          btn.disabled = false;
          btn.className = 'px-3 py-1.5 text-[11px] font-bold rounded transition-all cursor-pointer bg-amber-500 hover:bg-amber-400 text-slate-950 shadow-md font-extrabold';
        } else {
          btn.textContent = '✅ 설치 완료';
          btn.disabled = true;
          btn.className = 'px-3 py-1.5 text-[11px] font-bold rounded bg-emerald-950/80 border border-emerald-500/60 text-[#00FFA3] cursor-default shadow-sm';
        }
      } else {
        btn.textContent = '📥 스크립트 설치';
        btn.disabled = false;
        btn.className = 'px-3 py-1.5 text-[11px] font-bold rounded transition-all cursor-pointer bg-emerald-500 hover:bg-emerald-400 text-slate-950 shadow-md font-extrabold';
      }
    }


    async function checkObsScriptStatus() {
      try {
        const res = await apiFetch('/obs-script-status');
        if (!res.ok) return;
        const data = await res.json();
        currentObsStatus = data;
        updateObsScriptUI();
      } catch (e) {
        console.warn('OBS script status check error:', e);
      }
    }


    async function handleObsScriptAction() {
      const btn = document.getElementById('btn-obs-script-action');
      if (currentObsStatus.installed && !currentObsStatus.needs_update) return;

      if (!currentObsStatus.detected) {
        await browseAndInstallObsFolder();
        return;
      }

      const isUpdate = (currentObsStatus.installed && currentObsStatus.needs_update);
      if (btn) {
        btn.disabled = true;
        btn.textContent = isUpdate ? '업데이트 중...' : '설치 중...';
      }

      try {
        const res = await apiFetch('/install-obs-script', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ custom_dir: currentObsStatus.path || '' })
        });
        const data = await res.json();
        if (res.ok && data.code === 200) {
          if (isUpdate) {
            showToast('🎉 스크립트 업데이트 완료', 'success', '최신 버전의 chzzk_dock_launcher.lua가 적용되었습니다.');
          } else {
            showToast('🎉 스크립트 파일 설치 완료', 'success', 'OBS [도구] ➔ [스크립트] ➔ [+] 버튼으로 chzzk_dock_launcher.lua를 등록해 주세요.');
          }
          currentObsStatus.installed = true;
          currentObsStatus.needs_update = false;
          if (data.path) {
            currentObsStatus.path = data.path;
            currentObsStatus.detected = true;
          }
        } else {
          showToast(isUpdate ? '업데이트 실패' : '설치 실패', 'error', data.message || '스크립트 적용 실패');
        }
      } catch (e) {
        showToast(isUpdate ? '업데이트 오류' : '설치 오류', 'error', e.message);
      } finally {
        updateObsScriptUI();
      }
    }


    async function browseAndInstallObsFolder() {
      try {
        const res = await apiFetch('/browse-obs-folder', { method: 'POST' });
        const data = await res.json();
        if (res.ok && data.code === 200) {
          if (!data.cancelled && data.resolved_path) {
            currentObsStatus.path = data.resolved_path;
            currentObsStatus.detected = true;
            updateObsScriptUI();
            await handleObsScriptAction();
          } else {
            updateObsScriptUI();
          }
        } else {
          updateObsScriptUI();
          showToast('폴더 선택 실패', 'error', data.message || '폴더 선택 다이얼로그 실행 실패');
        }
      } catch (e) {
        updateObsScriptUI();
        showToast('폴더 선택 오류', 'error', e.message);
      }
    }


    async function loadCategoryFavorites() {
      fetchLivePopularCategories();
      try {
        const res = await apiFetch('/category-favorites');
        if (res.ok) {
          const data = await res.json();
          if (data && Array.isArray(data.favorites)) {
            categoryFavoritesList = data.favorites.map(f => {
              const id = f.category_id || f.categoryId || f.id || '';
              const val = f.category_name || f.categoryValue || id;
              if (id.toLowerCase() === 'talk' || val.toLowerCase() === 'talk') {
                return {
                  ...f,
                  category_id: 'talk',
                  categoryId: 'talk',
                  id: 'talk',
                  category_name: 'talk',
                  categoryValue: 'talk',
                  category_type: 'ETC',
                  categoryType: 'ETC'
                };
              }
              return f;
            });
          }
        }
      } catch (err) {
        console.warn('카테고리 즐겨찾기 로드 실패:', err);
      }

      updateCategoryFavoriteToggleUI();
    }


    async function saveCategoryFavorites(list) {
      categoryFavoritesList = (list || []).slice(0, MAX_CATEGORY_FAVORITES);
      updateCategoryFavoriteToggleUI();
      try {
        await apiFetch('/save-category-favorites', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ favorites: categoryFavoritesList })
        });
      } catch (err) {
        console.error('카테고리 즐겨찾기 저장 실패:', err);
      }
    }


    function getFavoriteCategories() {
      return categoryFavoritesList || [];
    }


    function isCategoryFavorite(catId) {
      if (!catId) return false;
      const target = catId.toLowerCase() === 'talk' ? 'talk' : catId;
      return getFavoriteCategories().some(f => {
        const fId = (f.category_id || f.categoryId || f.id || '');
        const fVal = (f.category_name || f.categoryValue || '');
        if (target === 'talk') {
          return fId.toLowerCase() === 'talk' || fVal.toLowerCase() === 'talk';
        }
        return fId === target || fVal === target;
      });
    }


    async function toggleCategoryFavorite(catItem, event) {
      if (event) {
        event.preventDefault();
        event.stopPropagation();
      }
      if (!catItem) return;
      let catId = catItem.categoryId || catItem.category_id || catItem.id;
      if (!catId) return;
      let catVal = catItem.categoryValue || catItem.category_name || catId;
      let catType = catItem.categoryType || catItem.category_type || 'GAME';
      let poster = catItem.posterImageUrl || catItem.poster_image_url || '';

      if (catId.toLowerCase() === 'talk' || catVal.toLowerCase() === 'talk') {
        catId = 'talk';
        catVal = 'talk';
        catType = 'ETC';
        if (!poster) poster = 'https://ssl.pstatic.net/static/nng/glive/icon/category_poster_talk.png';
      }

      let favs = [...getFavoriteCategories()];
      const existingIdx = favs.findIndex(f => {
        const fId = f.category_id || f.categoryId || f.id;
        const fVal = f.category_name || f.categoryValue || fId;
        if (catId === 'talk') return (fId && fId.toLowerCase() === 'talk') || (fVal && fVal.toLowerCase() === 'talk');
        return fId === catId;
      });

      if (existingIdx > -1) {
        favs.splice(existingIdx, 1);
        await saveCategoryFavorites(favs);
        showToast(`[${catVal}] 즐겨찾기 해제`, 'info');
      } else {
        if (favs.length >= MAX_CATEGORY_FAVORITES) {
          showToast(`즐겨찾기는 최대 ${MAX_CATEGORY_FAVORITES}개까지 등록 가능합니다.`, 'warn');
          return;
        }
        favs.push({
          category_id: catId,
          category_name: catVal,
          category_type: catType,
          poster_image_url: poster
        });
        await saveCategoryFavorites(favs);
        showToast(`[${catVal}] 즐겨찾기 등록 완료`, 'success');
      }

      // 열려있는 드롭다운 즉시 리렌더링
      const dropdown = document.getElementById('category-dropdown');
      if (dropdown && dropdown.style.display !== 'none') {
        const curQuery = document.getElementById('category-search')?.value || '';
        if (!curQuery.trim()) {
          renderFavoritesDropdown();
        } else {
          searchCategories(curQuery);
        }
      }
    }


    function toggleCategoryFavoriteRow(catId, catType, catVal, poster, event) {
      if (event) {
        event.stopPropagation();
        event.preventDefault();
      }
      toggleCategoryFavorite({
        categoryId: catId,
        categoryValue: catVal,
        categoryType: catType,
        posterImageUrl: poster
      });
    }


    function toggleCurrentCategoryFavorite(event) {
      if (event) {
        event.preventDefault();
        event.stopPropagation();
      }
      const elId = document.getElementById('category-id');
      const elType = document.getElementById('category-type');
      const elSearch = document.getElementById('category-search');
      const catId = elId ? elId.value : '';
      const catVal = elSearch ? elSearch.value : '';
      const catType = elType ? elType.value : 'ETC';
      if (!catId && !catVal) return;

      toggleCategoryFavorite({
        categoryId: catId || catVal,
        categoryValue: catVal || catId,
        categoryType: catType,
        posterImageUrl: ''
      });
    }


    function updateCategoryFavoriteToggleUI() {
      const elToggle = document.getElementById('category-favorite-toggle');
      const elId = document.getElementById('category-id');
      const elSearch = document.getElementById('category-search');
      if (!elToggle) return;

      const catId = elId ? elId.value : '';
      const catVal = elSearch ? elSearch.value : '';
      if (!catId && !catVal) {
        elToggle.style.display = 'none';
        return;
      }
      elToggle.style.display = 'block';
      if (isCategoryFavorite(catId || catVal)) {
        elToggle.className = 'absolute right-7 top-1/2 -translate-y-1/2 text-amber-400 text-xs font-bold leading-none cursor-pointer p-0.5 transition-colors';
        elToggle.title = '즐겨찾기 해제';
      } else {
        elToggle.className = 'absolute right-7 top-1/2 -translate-y-1/2 text-slate-500 hover:text-amber-400 text-xs font-bold leading-none cursor-pointer p-0.5 transition-colors';
        elToggle.title = '즐겨찾기 등록';
      }
    }


    function renderCategoryRowHtml(catType, catId, catVal, poster, dropsCampaignNo, isFav) {
      return `
        <div class="category-item flex items-center justify-between"
             data-type="${escapeHtml(catType)}"
             data-id="${escapeHtml(catId)}"
             data-value="${escapeHtml(catVal)}"
             data-drops="${escapeHtml(dropsCampaignNo ? String(dropsCampaignNo) : '')}"
             onclick="selectCategoryItem(this)">
          <div class="flex items-center gap-2 min-w-0 flex-1">
            ${poster ? `<img src="${poster}" alt="" class="w-6 h-6 rounded object-cover flex-shrink-0" onerror="this.style.display='none'" />` : '<div class="w-6 h-6 rounded bg-chzzk-border flex items-center justify-center text-[10px] text-chzzk-muted flex-shrink-0">🎮</div>'}
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-1.5">
                <span class="text-white text-xs font-medium truncate">${escapeHtml(catVal)}</span>
                ${dropsCampaignNo ? `<span class="text-xs leading-none flex-shrink-0 cursor-default" title="드롭스 활성 (캠페인 #${escapeHtml(String(dropsCampaignNo))})">🎁</span>` : ''}
              </div>
              <div class="text-[9px] text-chzzk-muted">${escapeHtml(catType)}</div>
            </div>
          </div>
          <button type="button" class="fav-star-btn ${isFav ? 'active' : ''} shrink-0 text-xs"
                  onclick="toggleCategoryFavoriteRow('${escapeHtml(catId)}', '${escapeHtml(catType)}', '${escapeHtml(catVal)}', '${escapeHtml(poster || '')}', event)"
                  title="${isFav ? '즐겨찾기 해제' : '즐겨찾기 추가'}">
            ★
          </button>
        </div>
      `;
    }


    async function renderFavoritesDropdown() {
      const dropdown = document.getElementById('category-dropdown');
      if (!dropdown) return;
      const favs = getFavoriteCategories();
      const seenIds = new Set();
      let html = '';

      if (favs.length > 0) {
        html += `
          <div class="px-2.5 py-1 text-[10px] font-bold text-amber-400 bg-[#161F2E] flex items-center justify-between border-b border-[#1E2738]">
            <span class="flex items-center gap-1">⭐ <span>즐겨찾기 목록</span></span>
            <span class="text-slate-500 font-normal">${favs.length}/${MAX_CATEGORY_FAVORITES}</span>
          </div>
        `;
        favs.forEach(f => {
          let fId = f.category_id || f.categoryId || f.id || '';
          let fVal = f.category_name || f.categoryValue || fId;
          let fType = f.category_type || f.categoryType || 'GAME';
          let poster = f.poster_image_url || f.posterImageUrl || '';
          if (fId.toLowerCase() === 'talk' || fVal.toLowerCase() === 'talk') {
            fId = 'talk';
            fVal = 'talk';
            fType = 'ETC';
            if (!poster) poster = 'https://ssl.pstatic.net/static/nng/glive/icon/category_poster_talk.png';
          }
          seenIds.add(fId);
          html += renderCategoryRowHtml(fType, fId, fVal, poster, null, true);
        });
      }

      // 추천 카테고리 (치지직 실시간 상위 인기 카테고리 풀에서 즐겨찾기 중복을 제외하고 상시 5개 유지)
      let popList = livePopularCategories;
      if (!popList || popList.length === 0) {
        popList = await fetchLivePopularCategories();
      }
      const recommendations = (popList || [])
        .filter(c => c && !seenIds.has(c.categoryId) && !seenIds.has(c.categoryValue))
        .slice(0, 5);
      if (recommendations.length > 0) {
        html += `
          <div class="px-2.5 py-1 text-[10px] font-bold text-slate-400 bg-[#161F2E] flex items-center justify-between border-b border-[#1E2738]">
            <span>🔥 <span>추천 카테고리</span></span>
          </div>
        `;
        recommendations.forEach(c => {
          html += renderCategoryRowHtml(c.categoryType, c.categoryId, c.categoryValue, c.posterImageUrl, c.dropsCampaignNo, false);
        });
      }

      dropdown.innerHTML = html;
      dropdown.style.display = 'block';
    }
