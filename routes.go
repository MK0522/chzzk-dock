package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"chzzk-dock/core"
)

// ============================================================
//  Go 1.22+ 표준 보일러플레이트 제거 헬퍼 (Server Helpers)
// ============================================================

// bindJSON: [A. 보일러플레이트 제거] 제네릭 기반 JSON 요청 파싱 헬퍼
// 매 엔드포인트마다 반복되던 10줄의 디코딩 + 400 에러 처리 코드를 단 1줄로 단축합니다.
func bindJSON[T any](w http.ResponseWriter, r *http.Request, dst *T) bool {
	if r.Body == nil {
		respondErr(w, http.StatusBadRequest, "요청 본문이 비어있습니다.")
		return false
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		respondErr(w, http.StatusBadRequest, "잘못된 요청 형식입니다.")
		return false
	}
	return true
}

// respondOK: 200 OK 성공 응답 헬퍼
func respondOK(w http.ResponseWriter, data any) {
	sendJSON(w, data, http.StatusOK)
}

// respondErr: 에러 응답 표준 헬퍼
func respondErr(w http.ResponseWriter, status int, msg string) {
	sendJSON(w, map[string]any{
		"code":    status,
		"message": msg,
	}, status)
}

// ============================================================
//  Go 1.22+ 표준 라우터 구축 (B. Method Routing Table)
// ============================================================

func buildRouter() http.Handler {
	mux := http.NewServeMux()

	// 1. 공통 미들웨어 가드 (보안 헤더, CORS, OPTIONS 프리플라이트)
	wrap := func(pattern string, handler http.HandlerFunc, isApi bool) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if !core.CheckSecurity(w, r) {
				return
			}
			setCORSHeaders(w, r)
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			if isApi && !core.CheckApiAuth(w, r) {
				return
			}
			handler(w, r)
		})
	}

	// 2. 인증 & 다중 계정 세션 라우트
	wrap("GET /config", handleConfig, true)
	wrap("GET /sessions", handleSessions, true)
	wrap("GET /login-webview", handleLoginWebview, true)
	wrap("GET /login-wait", handleLoginWait, true)
	wrap("POST /save-config", handleSaveConfig, true)
	wrap("POST /logout", handleLogout, true)
	wrap("POST /switch-session", handleSwitchSession, true)
	wrap("POST /remove-session", handleRemoveSession, true)

	// 3. 방송 설정 & 프리셋 & 썸네일 라우트
	wrap("GET /broadcast-presets", handleGetBroadcastPresets, true)
	wrap("POST /save-broadcast-presets", handleSaveBroadcastPresets, true)
	wrap("GET /category-favorites", handleGetCategoryFavorites, true)
	wrap("POST /save-category-favorites", handleSaveCategoryFavorites, true)
	wrap("POST /upload-thumbnail", handleUploadThumbnail, true)

	// 4. 시스템 설정 & 윈도우 웹뷰 제어 라우트
	wrap("GET /open-browser", handleOpenBrowser, true)
	wrap("GET /remote-webview", handleRemoteWebview, true)
	wrap("GET /chat-webview", handleChatWebview, true)
	wrap("POST /check-port", handleCheckPort, true)
	wrap("POST /save-port", handleSavePort, true)
	wrap("GET /app-settings", handleAppSettingsGet, true)
	wrap("PUT /app-settings", handleAppSettingsPut, true)

	// 5. OBS 연동 & 업데이트 & 텔레메트리 라우트
	wrap("GET /obs-script-status", handleObsScriptStatus, true)
	wrap("POST /browse-obs-folder", handleBrowseObsFolder, true)
	wrap("POST /install-obs-script", handleInstallObsScript, true)
	wrap("POST /export-script", handleExportScript, true)
	wrap("GET /check-update", handleCheckUpdate, true)
	wrap("GET /update-status", handleUpdateStatus, true)
	wrap("POST /execute-update", handleExecuteUpdate, true)
	wrap("GET /api/disk-total", handleDiskTotal, true)
	wrap("GET /api/open-folder", handleOpenFolder, true)
	wrap("POST /client-log", handleClientLog, true)

	// 5.5. ChzzkDock BFF (Backend-For-Frontend) 신설 라우트
	wrap("GET /chzzkdock/chat-mode", handleChzzkChatModeGet, true)
	wrap("PUT /chzzkdock/chat-mode/emoji", handleChzzkChatEmojiPut, true)
	wrap("PUT /chzzkdock/chat-mode/slow", handleChzzkChatSlowPut, true)

	wrap("GET /chzzkdock/donations", handleChzzkDonationsGet, true)
	wrap("PUT /chzzkdock/donations/chat", handleChzzkDonationToggle("chat"), true)
	wrap("PUT /chzzkdock/donations/video", handleChzzkDonationToggle("video"), true)
	wrap("PUT /chzzkdock/donations/mission", handleChzzkDonationToggle("mission"), true)
	wrap("PUT /chzzkdock/donations/subscription", handleChzzkDonationToggle("subscription"), true)
	wrap("GET /chzzkdock/donations/details", handleChzzkDonationDetailsGet, true)
	wrap("PUT /chzzkdock/donations/details/chat", handleChzzkDonationDetailChatPut, true)
	wrap("PUT /chzzkdock/donations/details/video", handleChzzkDonationDetailVideoPut, true)
	wrap("PUT /chzzkdock/donations/details/mission", handleChzzkDonationDetailMissionPut, true)
	wrap("PATCH /chzzkdock/donations/details/subscription", handleChzzkDonationDetailSubscriptionPatch, true)

	wrap("GET /chzzkdock/streaming-urls", handleChzzkStreamingUrlsGet, true)
	wrap("GET /chzzkdock/streamkey", handleChzzkStreamKeyGet, true)
	wrap("PUT /chzzkdock/streamkey", handleChzzkStreamKeyPut, true)
	wrap("GET /chzzkdock/revenue/summary", handleChzzkRevenueSummaryGet, true)
	wrap("GET /chzzkdock/profile", handleChzzkProfileGet, true)                // 치지직 쿠키 불필요 (공개 프로필)
	wrap("GET /chzzkdock/live-status", handleChzzkLiveStatusGet, true)          // 치지직 쿠키 불필요 (공개 라이브 상태)
	wrap("GET /chzzkdock/user-status", handleChzzkUserStatusGet, true)
	wrap("GET /chzzkdock/session/bootstrap", handleChzzkSessionBootstrap, true)
	wrap("GET /chzzkdock/chat-rule", handleChzzkChatRuleGet, true)
	wrap("PUT /chzzkdock/chat-rule", handleChzzkChatRulePut, true)
	wrap("GET /chzzkdock/broadcast", handleChzzkBroadcastGet, true)
	wrap("PUT /chzzkdock/broadcast", handleChzzkBroadcastPut, true)
	wrap("GET /chzzkdock/categories/search", handleChzzkCategoriesSearchGet, true)        // 치지직 쿠키 불필요 (공개 검색)
	wrap("GET /chzzkdock/categories/recommended", handleChzzkCategoriesRecommendedGet, true)  // 치지직 쿠키 불필요 (공개 추천)
	wrap("GET /chzzkdock/drops/campaigns", handleChzzkDropsCampaignsGet, true)
	wrap("GET /chzzkdock/drops/match", handleChzzkDropsMatchGet, true)                    // 치지직 쿠키 불필요 (공개 드롭스)
	wrap("GET /chzzkdock/watch-parties", handleChzzkWatchPartiesGet, true)
	wrap("GET /chzzkdock/snapshot", handleChzzkWatchPartySnapshotGet, true)
	wrap("POST /chzzkdock/snapshot", handleChzzkWatchPartySnapshotPost, true)
	wrap("DELETE /chzzkdock/snapshot", handleChzzkWatchPartySnapshotDelete, true)

	// 6. 정적 자산 & 공개 문서 (API Auth 불필요)
	wrap("GET /guide-image/1", handleGuideImage1, false)
	wrap("GET /guide-image/2", handleGuideImage2, false)
	wrap("GET /obs-script", handleObsScriptFile, false)
	wrap("GET /chzzk_dock_launcher.lua", handleObsScriptFile, false)
	wrap("GET /show-ui", handleShowUI, true)
	wrap("GET /api/show-ui", handleShowUI, true)
	wrap("GET /stats", handleStatsHtml, false)
	wrap("GET /stat", handleStatsHtml, false)
	wrap("GET /mini", handleMiniHtml, false)
	wrap("GET /{$}", handleIndexHtml, false)
	wrap("GET /index.html", handleIndexHtml, false)
	wrap("GET /ui/", handleStaticUI, false)

	// 7. 치지직 비공식 API 프록시 라우트 (C. 영역 - 기존 프록시 핸들러와 100% 호환 보존)
	proxyHandler := func(w http.ResponseWriter, r *http.Request) {
		if !core.CheckSecurity(w, r) {
			return
		}
		setCORSHeaders(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		if !core.CheckApiAuth(w, r) {
			return
		}
		if !proxyDispatch(w, r, r.Method) {
			respondErr(w, http.StatusNotFound, "Not Found")
		}
	}
	mux.HandleFunc("/unofficial/", proxyHandler)

	return mux
}

// ============================================================
//  분해된 개별 도메인 핸들러 함수들 (15~20줄 내외의 독립 함수)
// ============================================================

// --- [인증 및 세션 핸들러] ---

func handleConfig(w http.ResponseWriter, r *http.Request) {
	core.InvalidateConfigCache()
	cfg := core.LoadConfig()
	autMask, sesMask := "", ""
	if cfg.NidAut != "" {
		autMask = "••••••••••••••••••••••••••••••••"
	}
	if cfg.NidSes != "" {
		sesMask = "••••••••••••••••••••••••••••••••"
	}
	respondOK(w, map[string]any{
		"nid_aut": autMask,
		"nid_ses": sesMask,
	})
}

type SessionView struct {
	Index           int    `json:"index"`
	Active          bool   `json:"active"`
	ChannelID       string `json:"channel_id"`
	ChannelName     string `json:"channel_name"`
	ProfileImageURL string `json:"profile_image_url"`
	Expired         bool   `json:"expired"`
}

func buildSessionViews(cfg core.Config) []SessionView {
	views := make([]SessionView, len(cfg.Sessions))
	var wg sync.WaitGroup
	for i, sess := range cfg.Sessions {
		wg.Add(1)
		go func(idx int, s core.SessionToken) {
			defer wg.Done()
			v := SessionView{Index: idx, Active: (idx == 0)}
			cid, cname, avatar, ok := fetchSessionUserStatus(s.NidAut, s.NidSes)
			if !ok {
				v.Expired = true
				v.ChannelName = "세션 만료됨"
			} else {
				v.ChannelID = cid
				v.ChannelName = cname
				v.ProfileImageURL = avatar
			}
			views[idx] = v
		}(i, sess)
	}
	wg.Wait()
	return views
}

func handleSessions(w http.ResponseWriter, r *http.Request) {
	core.InvalidateConfigCache()
	cfg := core.LoadConfig()
	views := buildSessionViews(cfg)
	respondOK(w, map[string]any{"sessions": views})
}

func handleSwitchSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Index int `json:"index"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if !core.SwitchSession(req.Index) {
		respondErr(w, http.StatusNotFound, "해당 세션을 찾을 수 없습니다.")
		return
	}
	cfg := core.LoadConfig()
	cid, cname, _, ok := fetchSessionUserStatus(cfg.NidAut, cfg.NidSes)
	if ok && cid != "" {
		SetCachedActiveChannelId(cid)
	}
	if ok && cname != "" {
		core.LogInfo("[HTTP] 계정 전환 완료 -> 스트리머 '%s' (채널 ID: %s)", cname, cid)
	} else {
		core.LogInfo("[HTTP] 계정 전환 완료 (인덱스: %d)", req.Index)
	}
	respondOK(w, map[string]any{"code": 200, "message": "계정 전환 완료"})
}

func handleRemoveSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Index int `json:"index"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if !core.RemoveSession(req.Index) {
		respondErr(w, http.StatusNotFound, "해당 세션을 찾을 수 없습니다.")
		return
	}
	respondOK(w, map[string]any{"code": 200, "message": "계정 삭제 완료"})
}

func handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var bodyMap map[string]any
	if !bindJSON(w, r, &bodyMap) {
		return
	}
	aut, _ := bodyMap["nid_aut"].(string)
	ses, _ := bodyMap["nid_ses"].(string)
	if strings.Contains(aut, "•") || strings.Contains(aut, "*") || strings.Contains(ses, "•") || strings.Contains(ses, "*") {
		respondErr(w, http.StatusBadRequest, "더미 마스킹 값이 아닌 실제 쿠키 값을 입력하세요.")
		return
	}
	core.SaveConfig(bodyMap)
	if aut, ok := bodyMap["nid_aut"].(string); ok && aut != "" {
		ses, _ := bodyMap["nid_ses"].(string)
		go func(a, s string) {
			if cid, _, _, ok := fetchSessionUserStatus(a, s); ok && cid != "" {
				SetCachedActiveChannelId(cid)
			}
		}(aut, ses)
	}
	respondOK(w, map[string]any{
		"code":    200,
		"message": "성공적으로 저장되었습니다.",
		"config": map[string]string{
			"nid_aut": "••••••••••••••••••••••••••••••••",
			"nid_ses": "••••••••••••••••••••••••••••••••",
		},
	})
}

// handleLogout: 세션 로그아웃 처리
// [보안/UX 설계] 네이버 2차 인증 기기 기억(NID_JST) 토큰을 보존하기 위해 webview_profile 폴더는 삭제하지 않습니다.
// 폴더 삭제 시 로그인할 때마다 2차 인증(OTP)을 매번 반복해야 하는 치명적 UX 퇴행이 발생하므로,
// 자격 증명 초기화(ClearConfig) 및 서브 프로세스 일괄 종료(killAllSubProcesses)로 안전하게 세션을 종료합니다.
func handleLogout(w http.ResponseWriter, r *http.Request) {
	killAllSubProcesses()
	core.ClearConfig()
	SetCachedActiveChannelId("")
	respondOK(w, map[string]any{"code": 200, "message": "성공적으로 로그아웃되었습니다."})
}

func handleLoginWebview(w http.ResponseWriter, r *http.Request) {
	core.LogInfo("[HTTP] /login-webview 요청 수신")
	webviewLock.Lock()
	if webviewProcess != nil && webviewProcess.ProcessState == nil {
		webviewLock.Unlock()
		core.LogWarn("[HTTP] /login-webview: 이미 네이버 로그인 창이 열려 있습니다.")
		respondOK(w, map[string]any{
			"status":  "already_open",
			"message": "이미 네이버 로그인 창이 열려 있습니다.",
		})
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		webviewLock.Unlock()
		core.LogError("[HTTP] /login-webview: 실행 파일 경로 확인 실패: %v", err)
		respondErr(w, http.StatusInternalServerError, "실행 파일 경로를 찾을 수 없습니다.")
		return
	}

	cmd := exec.Command(exePath, "--login")
	if err := cmd.Start(); err != nil {
		webviewLock.Unlock()
		core.LogError("[HTTP] /login-webview: 로그인 서브프로세스 시작 실패: %v", err)
		respondErr(w, http.StatusInternalServerError, "로그인 웹뷰를 시작할 수 없습니다.")
		return
	}
	procAllowSetForegroundWindow.Call(uintptr(cmd.Process.Pid))
	webviewProcess = cmd
	waitCh := make(chan struct{})
	webviewWaitCh = waitCh
	trackSubProcess(cmd, func() {
		close(waitCh)
		cfg := core.LoadConfig()
		if cfg.NidAut != "" && cfg.NidSes != "" {
			if cid, _, _, ok := fetchSessionUserStatus(cfg.NidAut, cfg.NidSes); ok && cid != "" {
				SetCachedActiveChannelId(cid)
			}
		}
	})
	webviewLock.Unlock()
	core.LogInfo("[HTTP] /login-webview: 로그인 서브프로세스 시작 완료 (PID: %d)", cmd.Process.Pid)

	respondOK(w, map[string]any{
		"status":  "started",
		"message": "네이버 로그인 웹뷰 창이 열렸습니다.",
	})
}

func handleLoginWait(w http.ResponseWriter, r *http.Request) {
	webviewLock.Lock()
	waitCh := webviewWaitCh
	webviewWaitCh = nil
	webviewLock.Unlock()

	if waitCh == nil {
		core.InvalidateConfigCache()
		cfg := core.LoadConfig()
		if cfg.NidAut != "" && cfg.NidSes != "" {
			respondOK(w, map[string]any{
				"status": "completed",
				"config": map[string]string{
					"nid_aut": "••••••••••••••••••••••••••••••••",
					"nid_ses": "••••••••••••••••••••••••••••••••",
				},
			})
		} else {
			respondOK(w, map[string]any{
				"status":  "closed",
				"message": "로그인 창이 열려있지 않습니다.",
			})
		}
		return
	}

	core.LogInfo("[HTTP] /login-wait 대기 시작")
	select {
	case <-waitCh:
		core.LogInfo("[HTTP] /login-wait: 로그인 프로세스 종료 감지")
	case <-time.After(180 * time.Second):
		core.LogWarn("[HTTP] /login-wait: 180초 대기 타임아웃")
	}

	core.InvalidateConfigCache()
	cfg := core.LoadConfig()
	if cfg.NidAut != "" && cfg.NidSes != "" {
		cid, cname, _, ok := fetchSessionUserStatus(cfg.NidAut, cfg.NidSes)
		if ok && cname != "" {
			core.LogInfo("[HTTP] /login-wait: 네이버 로그인 연동 성공 -> 스트리머 '%s' (채널 ID: %s)", cname, cid)
		} else {
			core.LogInfo("[HTTP] /login-wait: 네이버 로그인 세션 쿠키 연동 성공")
		}
		respondOK(w, map[string]any{
			"status": "completed",
			"config": map[string]string{
				"nid_aut": "••••••••••••••••••••••••••••••••",
				"nid_ses": "••••••••••••••••••••••••••••••••",
			},
		})
	} else {
		core.LogWarn("[HTTP] /login-wait: 쿠키 미취득 상태로 창 닫힘")
		respondOK(w, map[string]any{
			"status":  "closed",
			"message": "로그인 창이 닫혔습니다.",
		})
	}
}

// --- [방송 설정 & 프리셋 핸들러] ---

func handleGetBroadcastPresets(w http.ResponseWriter, r *http.Request) {
	presets := core.GetBroadcastPresets()
	respondOK(w, map[string]any{"code": 200, "presets": presets})
}

func handleSaveBroadcastPresets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Presets []core.BroadcastPreset `json:"presets"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if len(req.Presets) > 10 {
		respondErr(w, http.StatusBadRequest, "프리셋은 최대 10개까지만 저장할 수 있습니다.")
		return
	}
	for i, p := range req.Presets {
		name := strings.TrimSpace(p.Name)
		title := strings.TrimSpace(p.Title)
		if name == "" || len([]rune(name)) > 10 {
			respondErr(w, http.StatusBadRequest, fmt.Sprintf("프리셋 #%d의 이름은 1~10자 이내여야 합니다.", i+1))
			return
		}
		if title == "" || len([]rune(title)) > 50 {
			respondErr(w, http.StatusBadRequest, fmt.Sprintf("프리셋 #%d의 제목은 1~50자 이내여야 합니다.", i+1))
			return
		}
		if len(p.Tags) > 5 {
			respondErr(w, http.StatusBadRequest, fmt.Sprintf("프리셋 #%d의 태그는 최대 5개까지 가능합니다.", i+1))
			return
		}
		for _, t := range p.Tags {
			if len([]rune(t)) > 15 {
				respondErr(w, http.StatusBadRequest, fmt.Sprintf("프리셋 #%d의 각 태그는 최대 15자까지 가능합니다.", i+1))
				return
			}
		}
	}
	if err := core.SaveBroadcastPresets(req.Presets); err != nil {
		core.LogError("[Settings] Failed to save presets: %v", err)
		respondErr(w, http.StatusInternalServerError, "프리셋 저장에 실패했습니다.")
		return
	}
	respondOK(w, map[string]any{"code": 200, "presets": req.Presets, "message": "프리셋이 성공적으로 저장되었습니다."})
}

func handleGetCategoryFavorites(w http.ResponseWriter, r *http.Request) {
	favs := core.GetCategoryFavorites()
	respondOK(w, map[string]any{"code": 200, "favorites": favs})
}

func handleSaveCategoryFavorites(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Favorites []core.CategoryFavorite `json:"favorites"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if len(req.Favorites) > 8 {
		respondErr(w, http.StatusBadRequest, "카테고리 즐겨찾기는 최대 8개까지만 저장할 수 있습니다.")
		return
	}
	if err := core.SaveCategoryFavorites(req.Favorites); err != nil {
		core.LogError("[Settings] 카테고리 즐겨찾기 저장 실패: %v", err)
		respondErr(w, http.StatusInternalServerError, "설정 저장에 실패했습니다.")
		return
	}
	respondOK(w, map[string]any{"code": 200, "favorites": req.Favorites, "message": "카테고리 즐겨찾기가 저장되었습니다."})
}

func handleUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(6 * 1024 * 1024); err != nil {
		respondErr(w, http.StatusBadRequest, "파일을 읽을 수 없습니다 (최대 5MB).")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		respondErr(w, http.StatusBadRequest, "업로드할 파일이 없습니다.")
		return
	}
	defer file.Close()

	if header.Size > 5*1024*1024 {
		respondErr(w, http.StatusBadRequest, "이미지 파일 크기는 5MB 이하여야 합니다.")
		return
	}

	cfg := core.LoadConfig()
	if cfg.NidAut == "" || cfg.NidSes == "" {
		respondErr(w, http.StatusUnauthorized, "로그인 쿠키가 설정되지 않았습니다. 설정에서 로그인하세요.")
		return
	}

	bodyBuf := &bytes.Buffer{}
	mpWriter := multipart.NewWriter(bodyBuf)
	part, err := mpWriter.CreateFormFile("file", header.Filename)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "요청 생성 실패")
		return
	}
	if _, err := io.Copy(part, file); err != nil {
		respondErr(w, http.StatusInternalServerError, "파일 버퍼 복사 실패")
		return
	}
	mpWriter.Close()

	uploadReq, err := http.NewRequest("POST", "https://comm-api.game.naver.com/nng_main/v1/remote/photo/upload", bodyBuf)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "업로드 요청 생성 실패")
		return
	}
	uploadReq.Header.Set("User-Agent", USER_AGENT)
	uploadReq.Header.Set("Content-Type", mpWriter.FormDataContentType())
	uploadReq.Header.Set("Cookie", fmt.Sprintf("NID_AUT=%s; NID_SES=%s", cfg.NidAut, cfg.NidSes))
	uploadReq.Header.Set("Origin", "https://chzzk.naver.com")
	uploadReq.Header.Set("Referer", "https://chzzk.naver.com/")

	resp, err := httpClient.Do(uploadReq)
	if err != nil {
		respondErr(w, http.StatusBadGateway, "치지직 업로드 서버 연결 실패")
		return
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	var uploadResult struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Content []struct {
			URL        string `json:"url"`
			ResultCode int    `json:"resultCode"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBytes, &uploadResult); err == nil && len(uploadResult.Content) > 0 && uploadResult.Content[0].URL != "" {
		core.LogInfo("[Thumbnail] 치지직 서버에 썸네일 업로드 완료: %s", uploadResult.Content[0].URL)
		respondOK(w, map[string]any{
			"code":     200,
			"imageUrl": uploadResult.Content[0].URL,
			"message":  "썸네일이 성공적으로 업로드되었습니다.",
		})
		return
	}

	core.LogError("[Thumbnail] 치지직 서버 업로드 응답 실패 (HTTP %d, 바이트 크기: %d)", resp.StatusCode, len(respBytes))
	sendJSON(w, map[string]any{
		"code":    500,
		"message": "치지직 서버에서 이미지 URL을 반환하지 않았습니다.",
	}, http.StatusInternalServerError)
}

// --- [시스템 설정 & 윈도우 웹뷰 제어 핸들러] ---

func handleOpenBrowser(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" || (!strings.HasPrefix(rawURL, "https://") && !strings.HasPrefix(rawURL, "http://")) {
		respondErr(w, http.StatusBadRequest, "잘못된 URL입니다.")
		return
	}
	core.OpenBrowser(rawURL)
	respondOK(w, map[string]any{"status": "ok"})
}

var (
	channelIdRegex        = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,64}$`)
	activeChannelIdLock   sync.RWMutex
	cachedActiveChannelId string
)

// SetCachedActiveChannelId: 현재 활성 계정의 채널 ID를 백엔드 메모리에 캐시
func SetCachedActiveChannelId(cid string) {
	activeChannelIdLock.Lock()
	cachedActiveChannelId = cid
	activeChannelIdLock.Unlock()
}

// GetCachedActiveChannelId: 백엔드 메모리에 캐시된 활성 계정의 채널 ID 반환
func GetCachedActiveChannelId() string {
	activeChannelIdLock.RLock()
	defer activeChannelIdLock.RUnlock()
	return cachedActiveChannelId
}

// resolveChannelIdFromRequest: 클라이언트 전달 channelId 또는 백엔드 캐시된 활성 채널 ID 해석 (외부 통신 지연 제거)
func resolveChannelIdFromRequest(r *http.Request) string {
	// 1. 요청 파라미터로 명시 전달된 channelId 우선 (영숫자/언더스코어/하이픈 정규식 형식 검증)
	channelId := strings.TrimSpace(r.URL.Query().Get("channelId"))
	if channelId != "" && channelIdRegex.MatchString(channelId) {
		return channelId
	}

	// 2. 백엔드 메모리에 캐시된 활성 계정의 채널 ID 반환 (0ms, 외부 통신 없음)
	if cached := GetCachedActiveChannelId(); cached != "" {
		return cached
	}

	// 3. 캐시가 비어있는 경우에만 폴백 조회 및 캐싱
	cfg := core.LoadConfig()
	if cfg.NidAut != "" && cfg.NidSes != "" {
		if cid, _, _, ok := fetchSessionUserStatus(cfg.NidAut, cfg.NidSes); ok && cid != "" {
			SetCachedActiveChannelId(cid)
			return cid
		}
	}

	return ""
}

func handleRemoteWebview(w http.ResponseWriter, r *http.Request) {
	channelId := resolveChannelIdFromRequest(r)
	core.LogInfo("[HTTP] /remote-webview 요청 수신 (channelId: %s)", channelId)
	if core.BringRemoteWindowToFront() {
		respondOK(w, map[string]any{
			"status":  "started",
			"message": "이미 실행 중인 치지직 리모컨 창을 앞으로 가져왔습니다.",
		})
		return
	}
	exePath, err := os.Executable()
	if err != nil {
		core.LogError("[HTTP] /remote-webview executable lookup failed: %v", err)
		respondErr(w, http.StatusInternalServerError, "실행 파일 경로를 찾을 수 없습니다.")
		return
	}
	args := []string{"--remote"}
	if channelId != "" {
		args = append(args, channelId)
	}
	cmd := exec.Command(exePath, args...)
	if err := cmd.Start(); err != nil {
		core.LogError("[HTTP] /remote-webview start failed: %v", err)
		respondErr(w, http.StatusInternalServerError, "리모컨 창을 시작할 수 없습니다.")
		return
	}
	procAllowSetForegroundWindow.Call(uintptr(cmd.Process.Pid))
	trackSubProcess(cmd)
	respondOK(w, map[string]any{"status": "started", "message": "치지직 리모컨 창이 열렸습니다."})
}

func handleChatWebview(w http.ResponseWriter, r *http.Request) {
	channelId := resolveChannelIdFromRequest(r)
	core.LogInfo("[HTTP] /chat-webview 요청 수신 (channelId: %s)", channelId)
	if core.BringChatWindowToFront() {
		respondOK(w, map[string]any{
			"status":  "started",
			"message": "이미 실행 중인 치지직 채팅창을 앞으로 가져왔습니다.",
		})
		return
	}
	exePath, err := os.Executable()
	if err != nil {
		core.LogError("[HTTP] /chat-webview executable lookup failed: %v", err)
		respondErr(w, http.StatusInternalServerError, "실행 파일 경로를 찾을 수 없습니다.")
		return
	}
	args := []string{"--chat"}
	if channelId != "" {
		args = append(args, channelId)
	}
	cmd := exec.Command(exePath, args...)
	if err := cmd.Start(); err != nil {
		core.LogError("[HTTP] /chat-webview start failed: %v", err)
		respondErr(w, http.StatusInternalServerError, "채팅 창을 시작할 수 없습니다.")
		return
	}
	procAllowSetForegroundWindow.Call(uintptr(cmd.Process.Pid))
	trackSubProcess(cmd)
	respondOK(w, map[string]any{"status": "started", "message": "치지직 채팅 창이 열렸습니다."})
}



func handleCheckPort(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Port int `json:"port"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if req.Port < 1024 || req.Port > 65535 {
		respondErr(w, http.StatusBadRequest, fmt.Sprintf("포트 번호는 1024 ~ 65535 사이여야 합니다 (입력값: %d).", req.Port))
		return
	}
	if req.Port == activeHttpPort {
		respondOK(w, map[string]any{
			"code":      200,
			"available": true,
			"port":      req.Port,
			"message":   fmt.Sprintf("현재 CHZZK OBS Dock에서 정상 작동 중인 포트(%d)입니다.", req.Port),
		})
		return
	}
	available, pid, procName, err := core.CheckPortAvailable(req.Port)
	if !available {
		displayName := procName
		if displayName == "" {
			displayName = "알 수 없는 프로그램"
		}
		if err != nil {
			core.LogWarn("[PortCheck] Port %d unavailable: %v", req.Port, err)
		}
		respondOK(w, map[string]any{
			"code":        409,
			"available":   false,
			"port":        req.Port,
			"occupied_by": displayName,
			"pid":         pid,
			"message":     fmt.Sprintf("포트 %d번은 이미 다른 프로그램('%s', PID %d)에서 사용 중입니다.", req.Port, displayName, pid),
		})
		return
	}
	respondOK(w, map[string]any{"code": 200, "available": true, "port": req.Port, "message": fmt.Sprintf("포트 %d번은 사용 가능합니다.", req.Port)})
}

func handleSavePort(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Port    int  `json:"port"`
		Restart bool `json:"restart"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if req.Port < 1024 || req.Port > 65535 {
		respondErr(w, http.StatusBadRequest, fmt.Sprintf("포트 번호는 1024 ~ 65535 사이여야 합니다 (입력값: %d).", req.Port))
		return
	}
	if err := core.SaveConfiguredPort(req.Port); err != nil {
		core.LogError("[Settings] Failed to save port: %v", err)
		respondErr(w, http.StatusInternalServerError, "포트 설정을 저장하는 중 오류가 발생했습니다.")
		return
	}
	if req.Restart && req.Port != activeHttpPort {
		respondOK(w, map[string]any{
			"code":             200,
			"port":             req.Port,
			"restart_required": true,
			"restarting":       true,
			"message":          fmt.Sprintf("기본 포트가 %d번으로 설정되었습니다.\n프로그램을 재시작합니다.", req.Port),
		})
		if restartExecutor != nil {
			go func() {
				time.Sleep(300 * time.Millisecond)
				restartExecutor()
			}()
		}
		return
	}
	respondOK(w, map[string]any{
		"code":             200,
		"port":             req.Port,
		"restart_required": req.Port != activeHttpPort,
		"message":          fmt.Sprintf("기본 포트가 %d번으로 설정되었습니다.\n프로그램을 재시작하면 새 포트로 작동합니다.", req.Port),
	})
}



// handleAppSettingsGet: GET /app-settings
// settings.json의 모든 일반 설정 및 런타임 상태(포트, 기본 브라우저)를 단일 JSON 객체로 반환
func handleAppSettingsGet(w http.ResponseWriter, r *http.Request) {
	st := core.LoadSettings()
	respondOK(w, map[string]any{
		"code":                   200,
		"settings":               st,
		"http_port":              st.HttpPort,
		"active_port":            activeHttpPort,
		"configured_port":        core.GetConfiguredPort(),
		"is_fallback":            isFallbackPort,
		"fallback_reason":        portFallbackReason,
		"default_browser":        core.GetDefaultBrowserName(),
		"font_scale":             st.FontScale,
		"show_viewer_count":      st.IsShowViewerCount(),
		"popup_on_start":         st.IsPopupOnStart(),
		"notify_on_shutdown":     st.IsNotifyOnShutdown(),
		"enable_gpu":             st.IsEnableGPU(),
		"external_browser_guard": st.IsExternalBrowserGuard(),
		"enable_beta_updates":    st.IsEnableBetaUpdates(),
		"auto_check_update":      st.IsAutoCheckUpdate(),
		"watchdog_timeout_sec":   st.WatchdogTimeoutSec,
	})
}

// handleAppSettingsPut: PUT /app-settings
// 부분 설정(JSON map)을 기존 설정에 안전하게 머지하고 %LOCALAPPDATA%\ChzzkObsDock\settings.json에 즉시 디스크 영속화
func handleAppSettingsPut(w http.ResponseWriter, r *http.Request) {
	var patch map[string]any
	if !bindJSON(w, r, &patch) {
		return
	}
	st := core.LoadSettings()

	if v, ok := patch["font_scale"].(float64); ok && v > 0 {
		st.SetFontScale(v)
	}
	if v, ok := patch["show_viewer_count"].(bool); ok {
		st.SetShowViewerCount(v)
	}
	if v, ok := patch["auto_check_update"].(bool); ok {
		st.SetAutoCheckUpdate(v)
	}
	if v, ok := patch["enable_beta_updates"].(bool); ok {
		st.SetEnableBetaUpdates(v)
	}
	if v, ok := patch["enable_gpu"].(bool); ok {
		st.SetEnableGPU(v)
	}
	if v, ok := patch["external_browser_guard"].(bool); ok {
		st.SetExternalBrowserGuard(v)
	}
	if v, ok := patch["popup_on_start"].(bool); ok {
		st.SetPopupOnStart(v)
	}
	if v, ok := patch["notify_on_shutdown"].(bool); ok {
		st.SetNotifyOnShutdown(v)
	}
	if v, ok := patch["watchdog_timeout_sec"].(float64); ok {
		timeout := int(v)
		st.WatchdogTimeoutSec = timeout
		st.WatchdogDisabled = (timeout == -1)
		core.SetWatchdogTimeoutSec(timeout)
		if trayInstance != nil {
			trayInstance.UpdateTooltip(getTrayTooltip())
		}
	}
	if v, ok := patch["skip_update_version"].(string); ok {
		st.SetSkipUpdateVersion(v)
	}
	if v, ok := patch["skip_script_version"].(float64); ok {
		st.SkipScriptVersion = int(v)
	}

	if err := core.SaveSettings(st); err != nil {
		respondErr(w, http.StatusInternalServerError, "설정 저장에 실패했습니다: "+err.Error())
		return
	}
	respondOK(w, map[string]any{"code": 200, "message": "설정이 성공적으로 저장되었습니다.", "settings": st})
}

// --- [OBS 연동 & 업데이트 & 텔레메트리 핸들러] ---

func handleObsScriptStatus(w http.ResponseWriter, r *http.Request) {
	scriptsDir, detected := core.DetectObsScriptsDir()
	installed, needsUpdate, installedVer, latestVer := core.CheckScriptStatusWithVersion(scriptsDir, getLauncherScriptData())

	st := core.LoadSettings()
	shouldPrompt := installed && needsUpdate && (st.SkipScriptVersion < latestVer)

	respondOK(w, map[string]any{
		"code":              200,
		"detected":          detected,
		"path":              scriptsDir,
		"installed":         installed,
		"needs_update":      needsUpdate,
		"installed_version": installedVer,
		"latest_version":    latestVer,
		"should_prompt":     shouldPrompt,
	})
}

func handleBrowseObsFolder(w http.ResponseWriter, r *http.Request) {
	folder, err := core.BrowseForObsFolder("OBS Studio 설치 폴더 또는 scripts 폴더를 선택하세요")
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "폴더 선택 중 오류가 발생했습니다.")
		return
	}
	if folder == "" {
		respondOK(w, map[string]any{"code": 200, "cancelled": true})
		return
	}
	resolved := core.ResolveObsScriptsDir(folder)
	respondOK(w, map[string]any{"code": 200, "cancelled": false, "selected_path": folder, "resolved_path": resolved})
}

func handleInstallObsScript(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CustomDir string `json:"custom_dir"`
	}
	_ = bindJSON(w, r, &req)
	installedPath, err := core.InstallLauncherScriptToObs(req.CustomDir, getLauncherScriptData())
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "OBS 스크립트 설치에 실패했습니다.")
		return
	}
	respondOK(w, map[string]any{"code": 200, "message": "OBS 스크립트 폴더에 성공적으로 추가되었습니다.", "path": installedPath})
}

func handleExportScript(w http.ResponseWriter, r *http.Request) {
	createdPath, err := core.ExportLauncherScript(getLauncherScriptData())
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "스크립트 내보내기에 실패했습니다.")
		return
	}
	respondOK(w, map[string]any{"code": 200, "message": "스크립트 파일이 생성되고 코드가 클립보드에 복사되었습니다.", "path": createdPath})
}

func handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "true"
	if !force && !core.LoadSettings().IsAutoCheckUpdate() {
		respondOK(w, map[string]any{
			"code": 200,
			"info": &core.UpdateInfo{
				CurrentVersion: APP_VERSION,
				LatestVersion:  APP_VERSION,
				HasUpdate:      false,
				ShouldPrompt:   false,
			},
		})
		return
	}
	info, err := core.CheckForUpdate(APP_VERSION, force)
	if err != nil {
		respondOK(w, map[string]any{"code": 500, "message": err.Error()})
		return
	}
	respondOK(w, map[string]any{"code": 200, "info": info})
}

func handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	progress := core.GetUpdateProgress()
	respondOK(w, map[string]any{"code": 200, "progress": progress})
}

func handleExecuteUpdate(w http.ResponseWriter, r *http.Request) {
	info, err := core.CheckForUpdate(APP_VERSION, false)
	if err != nil || info == nil || info.DownloadURL == "" {
		respondErr(w, http.StatusBadRequest, "사용 가능한 업데이트 정보를 찾을 수 없습니다.")
		return
	}
	if err := core.ExecuteUpdate(info.LatestVersion, info.DownloadURL, info.AssetSize, exitApp); err != nil {
		respondErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(w, map[string]any{"code": 200, "message": "업데이트 다운로드를 시작했습니다."})
}

func handleClientLog(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Level   string `json:"level"`
		Source  string `json:"source"`
		Message string `json:"message"`
		Detail  string `json:"detail"`
	}
	if bindJSON(w, r, &req) {
		// 단순 UI Toast 알림은 백엔드 중복 로깅 방지를 위해 무시
		if strings.EqualFold(req.Source, "toast") {
			respondOK(w, map[string]string{"status": "ok"})
			return
		}
		msg := req.Message
		if req.Detail != "" {
			msg += " - " + req.Detail
		}
		tag := "[UI Toast]"
		if req.Source != "" {
			tag = fmt.Sprintf("[UI %s]", req.Source)
		}
		switch strings.ToLower(req.Level) {
		case "warn", "warning":
			core.LogWarn("%s %s", tag, msg)
		case "error":
			core.LogError("%s %s", tag, msg)
		default:
			core.LogInfo("%s %s", tag, msg)
		}
	}
	respondOK(w, map[string]string{"status": "ok"})
}

func serveDockPage(w http.ResponseWriter, content []byte) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	sendBytes(w, content, http.StatusOK, "text/html; charset=utf-8")
}

func handleIndexHtml(w http.ResponseWriter, r *http.Request) {
	serveDockPage(w, getRenderedHTML())
}

func handleStatsHtml(w http.ResponseWriter, r *http.Request) {
	serveDockPage(w, getRenderedStatsHTML())
}

func handleMiniHtml(w http.ResponseWriter, r *http.Request) {
	serveDockPage(w, getRenderedMiniHTML())
}

func handleGuideImage1(w http.ResponseWriter, r *http.Request) {
	sendBytes(w, embeddedGuide1, http.StatusOK, "image/png")
}

func handleGuideImage2(w http.ResponseWriter, r *http.Request) {
	sendBytes(w, embeddedGuide2, http.StatusOK, "image/png")
}

func handleObsScriptFile(w http.ResponseWriter, r *http.Request) {
	sendBytes(w, getLauncherScriptData(), http.StatusOK, "text/plain; charset=utf-8")
}

func handleShowUI(w http.ResponseWriter, r *http.Request) {
	core.ShowDockWindow()
	respondOK(w, map[string]any{"code": 200, "message": "UI 표시 완료"})
}

// handleStaticUI: GET /ui/* 정적 자산(CSS, JS, 이미지 등) 내장 embed.FS 서빙
func handleStaticUI(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/ui/")
	if strings.Contains(relPath, "..") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	cleanRel := filepath.Clean(relPath)
	if cleanRel == "." || cleanRel == "/" {
		http.NotFound(w, r)
		return
	}

	slashPath := "ui/" + filepath.ToSlash(cleanRel)
	if data, err := embeddedUI.ReadFile(slashPath); err == nil {
		sendStaticAsset(w, cleanRel, data)
		return
	}

	http.NotFound(w, r)
}

func sendStaticAsset(w http.ResponseWriter, path string, data []byte) {
	ctype := mime.TypeByExtension(filepath.Ext(path))
	if ctype == "" {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".css":
			ctype = "text/css; charset=utf-8"
		case ".js":
			ctype = "application/javascript; charset=utf-8"
		case ".html":
			ctype = "text/html; charset=utf-8"
		case ".json":
			ctype = "application/json; charset=utf-8"
		case ".png":
			ctype = "image/png"
		case ".svg":
			ctype = "image/svg+xml"
		default:
			ctype = "application/octet-stream"
		}
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
