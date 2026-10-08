package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"chzzk-obs-dock/core"
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
	wrap("GET /unofficial-user", handleUnofficialUser, true)
	wrap("POST /save-config", handleSaveConfig, true)
	wrap("POST /logout", handleLogout, true)
	wrap("POST /switch-session", handleSwitchSession, true)
	wrap("POST /remove-session", handleRemoveSession, true)

	// 3. 방송 설정 & 프리셋 & 썸네일 라우트
	wrap("GET /broadcast-presets", handleGetBroadcastPresets, true)
	wrap("POST /save-broadcast-presets", handleSaveBroadcastPresets, true)
	wrap("GET /category-favorites", handleGetCategoryFavorites, true)
	wrap("POST /save-category-favorites", handleSaveCategoryFavorites, true)
	wrap("GET /categories-recommended", handleGetRecommendedCategories, true)
	wrap("POST /upload-thumbnail", handleUploadThumbnail, true)

	// 4. 시스템 설정 & 윈도우 웹뷰 제어 라우트
	wrap("GET /open-browser", handleOpenBrowser, true)
	wrap("GET /remote-webview", handleRemoteWebview, true)
	wrap("GET /chat-webview", handleChatWebview, true)
	wrap("GET /watchdog-timeout", handleGetWatchdogTimeout, true)
	wrap("POST /watchdog-timeout", handleSaveWatchdogTimeout, true)
	wrap("GET /port-status", handlePortStatus, true)
	wrap("POST /check-port", handleCheckPort, true)
	wrap("POST /save-port", handleSavePort, true)
	wrap("GET /startup-popup-status", handleGetStartupPopupStatus, true)
	wrap("POST /save-startup-popup", handleSaveStartupPopup, true)
	wrap("GET /shutdown-notify-status", handleGetShutdownNotifyStatus, true)
	wrap("POST /save-shutdown-notify", handleSaveShutdownNotify, true)
	wrap("GET /gpu-status", handleGetGpuStatus, true)
	wrap("POST /save-gpu", handleSaveGpu, true)
	wrap("GET /external-browser-status", handleGetExternalBrowserStatus, true)
	wrap("POST /save-external-browser", handleSaveExternalBrowser, true)
	wrap("GET /beta-updates-status", handleGetBetaUpdatesStatus, true)
	wrap("POST /save-beta-updates", handleSaveBetaUpdates, true)
	wrap("GET /auto-check-update-status", handleGetAutoCheckUpdateStatus, true)
	wrap("POST /save-auto-check-update", handleSaveAutoCheckUpdate, true)
	wrap("GET /remote-tester-status", handleGetRemoteTesterStatus, true)
	wrap("POST /remote-tester-auth", handleRemoteTesterAuth, true)

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

	// 6. 정적 자산 & 공개 문서 (API Auth 불필요)
	wrap("GET /guide-image/1", handleGuideImage1, false)
	wrap("GET /guide-image/2", handleGuideImage2, false)
	wrap("GET /obs-script", handleObsScriptFile, false)
	wrap("GET /obs-launcher.lua", handleObsScriptFile, false)
	wrap("GET /chzzk_dock_launcher.lua", handleObsScriptFile, false)
	wrap("GET /show-ui", handleShowUI, false)
	wrap("GET /api/show-ui", handleShowUI, false)
	wrap("GET /stats", handleStatsHtml, false)
	wrap("GET /stats.html", handleStatsHtml, false)
	wrap("GET /obs-stats.html", handleStatsHtml, false)
	wrap("GET /{$}", handleIndexHtml, false)
	wrap("GET /index.html", handleIndexHtml, false)
	wrap("GET /chzzk-obs-dock.html", handleIndexHtml, false)

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
		"nid_aut":                autMask,
		"nid_ses":                sesMask,
		"remote_tester_unlocked": core.GetRemoteTesterUnlocked(),
	})
}

func handleSessions(w http.ResponseWriter, r *http.Request) {
	core.InvalidateConfigCache()
	cfg := core.LoadConfig()

	type SessionView struct {
		Index           int    `json:"index"`
		Active          bool   `json:"active"`
		ChannelID       string `json:"channel_id"`
		ChannelName     string `json:"channel_name"`
		ProfileImageURL string `json:"profile_image_url"`
		Expired         bool   `json:"expired"`
	}

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
	bodyMap["auth_method"] = "manual"
	core.SaveConfig(bodyMap)
	respondOK(w, map[string]any{
		"code":    200,
		"message": "성공적으로 저장되었습니다.",
		"config": map[string]string{
			"nid_aut": "••••••••••••••••••••••••••••••••",
			"nid_ses": "••••••••••••••••••••••••••••••••",
		},
	})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	core.ClearConfig()
	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	core.ClearWebViewSession(appData)
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
	trackSubProcess(cmd)
	waitCh := make(chan struct{})
	webviewWaitCh = waitCh
	go func(c *exec.Cmd, ch chan struct{}) {
		_ = c.Wait()
		close(ch)
	}(cmd, waitCh)
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
		core.LogInfo("[HTTP] /login-wait: 네이버 로그인 세션 쿠키 연동 성공")
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

func handleUnofficialUser(w http.ResponseWriter, r *http.Request) {
	proxyUnofficialRequest(w, r, "GET", "", nil, naverGameApiBaseURL+"/nng_main/v1/user/getUserStatus")
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

// handleGetRecommendedCategories: 치지직 상위 5개 카테고리 실시간 조회 (백엔드 캐싱 & 프론트엔드 전송 필드 최소화)
func handleGetRecommendedCategories(w http.ResponseWriter, r *http.Request) {
	targetURL := "https://api.chzzk.naver.com/service/v1/categories/live?size=5"
	if cached, found := core.GetCachedApiResponse("GET", targetURL); found {
		sendBytes(w, cached.Body, cached.Status, cached.ContentType)
		return
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "요청 생성 실패")
		return
	}
	req.Header.Set("User-Agent", USER_AGENT)

	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		respondErr(w, http.StatusBadGateway, "치지직 API 응답 실패")
		return
	}
	defer resp.Body.Close()

	var apiRes struct {
		Content struct {
			Data []struct {
				CategoryType     string `json:"categoryType"`
				CategoryID       string `json:"categoryId"`
				CategoryValue    string `json:"categoryValue"`
				PosterImageURL   string `json:"posterImageUrl"`
				DropsCampaignNos []int  `json:"dropsCampaignNos"`
			} `json:"data"`
		} `json:"content"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiRes); err != nil {
		respondErr(w, http.StatusInternalServerError, "응답 파싱 실패")
		return
	}

	type CleanCategory struct {
		CategoryType    string `json:"categoryType"`
		CategoryID      string `json:"categoryId"`
		CategoryValue   string `json:"categoryValue"`
		PosterImageURL  string `json:"posterImageUrl"`
		DropsCampaignNo *int   `json:"dropsCampaignNo"`
	}

	cleaned := make([]CleanCategory, 0, len(apiRes.Content.Data))
	for _, item := range apiRes.Content.Data {
		var drops *int
		if len(item.DropsCampaignNos) > 0 {
			drops = &item.DropsCampaignNos[0]
		}
		catVal := item.CategoryValue
		if catVal == "" {
			catVal = item.CategoryID
		}
		cleaned = append(cleaned, CleanCategory{
			CategoryType:    item.CategoryType,
			CategoryID:      item.CategoryID,
			CategoryValue:   catVal,
			PosterImageURL:  item.PosterImageURL,
			DropsCampaignNo: drops,
		})
	}

	respData := map[string]any{"code": 200, "categories": cleaned}
	if b, err := json.Marshal(respData); err == nil {
		core.SetCachedApiResponse("GET", targetURL, b, http.StatusOK, "application/json")
	}

	respondOK(w, respData)
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

	core.LogError("[Thumbnail] 치지직 서버 업로드 응답 실패: %s", string(respBytes))
	sendJSON(w, map[string]any{
		"code":    500,
		"message": "치지직 서버에서 이미지 URL을 반환하지 않았습니다.",
		"raw":     string(respBytes),
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

func handleRemoteWebview(w http.ResponseWriter, r *http.Request) {
	channelId := r.URL.Query().Get("channelId")
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
	channelId := r.URL.Query().Get("channelId")
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

func handleGetWatchdogTimeout(w http.ResponseWriter, r *http.Request) {
	timeoutSec := core.GetWatchdogTimeoutSec()
	respondOK(w, map[string]any{"code": 200, "timeout_sec": timeoutSec})
}

func handleSaveWatchdogTimeout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TimeoutSec int `json:"timeout_sec"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if req.TimeoutSec != -1 && req.TimeoutSec != 0 && (req.TimeoutSec < 5 || req.TimeoutSec > 86400) {
		respondErr(w, http.StatusBadRequest, "대기 시간은 -1, 0 또는 5초에서 86400초 사이여야 합니다.")
		return
	}
	core.SetWatchdogTimeoutSec(req.TimeoutSec)
	st := core.LoadSettings()
	st.WatchdogTimeoutSec = req.TimeoutSec
	st.WatchdogDisabled = (req.TimeoutSec == -1)
	_ = core.SaveSettings(st)
	if trayInstance != nil {
		trayInstance.UpdateTooltip(getTrayTooltip())
	}
	msg := "대기 시간이 성공적으로 변경되었습니다."
	if req.TimeoutSec == 0 {
		msg = "OBS 종료 시 즉시 종료되도록 설정되었습니다."
	} else if req.TimeoutSec == -1 {
		msg = "OBS 자동 종료가 비활성화되었습니다. (상시 실행 유지)"
	}
	respondOK(w, map[string]any{"code": 200, "message": msg, "timeout_sec": req.TimeoutSec})
}

func handlePortStatus(w http.ResponseWriter, r *http.Request) {
	respondOK(w, map[string]any{
		"code":            200,
		"active_port":     activeHttpPort,
		"configured_port": core.GetConfiguredPort(),
		"is_fallback":     isFallbackPort,
		"fallback_reason": portFallbackReason,
		"default_port":    DEFAULT_HTTP_PORT,
	})
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

func handleGetStartupPopupStatus(w http.ResponseWriter, r *http.Request) {
	respondOK(w, map[string]any{"code": 200, "popup_on_start": core.GetPopupOnStart()})
}

func handleSaveStartupPopup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PopupOnStart bool `json:"popup_on_start"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if err := core.SetPopupOnStartSetting(req.PopupOnStart); err != nil {
		respondErr(w, http.StatusInternalServerError, "설정 저장에 실패했습니다.")
		return
	}
	respondOK(w, map[string]any{"code": 200, "popup_on_start": req.PopupOnStart, "message": "시작 팝업 설정이 저장되었습니다."})
}

func handleGetShutdownNotifyStatus(w http.ResponseWriter, r *http.Request) {
	respondOK(w, map[string]any{"code": 200, "notify_on_shutdown": core.GetNotifyOnShutdown()})
}

func handleSaveShutdownNotify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NotifyOnShutdown bool `json:"notify_on_shutdown"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if err := core.SetNotifyOnShutdownSetting(req.NotifyOnShutdown); err != nil {
		respondErr(w, http.StatusInternalServerError, "설정 저장에 실패했습니다.")
		return
	}
	respondOK(w, map[string]any{"code": 200, "notify_on_shutdown": req.NotifyOnShutdown, "message": "자동 종료 알림 설정이 저장되었습니다."})
}

func handleGetGpuStatus(w http.ResponseWriter, r *http.Request) {
	respondOK(w, map[string]any{"code": 200, "enable_gpu": core.GetEnableGPU()})
}

func handleSaveGpu(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EnableGPU bool `json:"enable_gpu"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if err := core.SetEnableGPUSetting(req.EnableGPU); err != nil {
		respondErr(w, http.StatusInternalServerError, "설정 저장에 실패했습니다.")
		return
	}
	respondOK(w, map[string]any{"code": 200, "enable_gpu": req.EnableGPU, "message": "웹뷰 GPU 가속 설정이 저장되었습니다."})
}

func handleGetExternalBrowserStatus(w http.ResponseWriter, r *http.Request) {
	respondOK(w, map[string]any{
		"code":                   200,
		"external_browser_guard": core.GetExternalBrowserGuard(),
		"browser_name":           core.GetDefaultBrowserName(),
	})
}

func handleSaveExternalBrowser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExternalBrowserGuard bool `json:"external_browser_guard"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if err := core.SetExternalBrowserGuardSetting(req.ExternalBrowserGuard); err != nil {
		respondErr(w, http.StatusInternalServerError, "설정 저장에 실패했습니다.")
		return
	}
	respondOK(w, map[string]any{
		"code":                   200,
		"external_browser_guard": req.ExternalBrowserGuard,
		"message":                "외부 링크 브라우저 열기 설정이 저장되었습니다.",
	})
}

func handleGetBetaUpdatesStatus(w http.ResponseWriter, r *http.Request) {
	respondOK(w, map[string]any{"code": 200, "enable_beta_updates": core.LoadSettings().IsEnableBetaUpdates()})
}

func handleSaveBetaUpdates(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EnableBetaUpdates bool `json:"enable_beta_updates"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	st := core.LoadSettings()
	st.SetEnableBetaUpdates(req.EnableBetaUpdates)
	if err := core.SaveSettings(st); err != nil {
		respondErr(w, http.StatusInternalServerError, "설정 저장에 실패했습니다.")
		return
	}
	respondOK(w, map[string]any{"code": 200, "enable_beta_updates": req.EnableBetaUpdates, "message": "테스트 버전 업데이트 설정이 저장되었습니다."})
}

func handleGetAutoCheckUpdateStatus(w http.ResponseWriter, r *http.Request) {
	respondOK(w, map[string]any{"code": 200, "auto_check_update": core.LoadSettings().IsAutoCheckUpdate()})
}

func handleSaveAutoCheckUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AutoCheckUpdate bool `json:"auto_check_update"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	st := core.LoadSettings()
	st.SetAutoCheckUpdate(req.AutoCheckUpdate)
	if err := core.SaveSettings(st); err != nil {
		respondErr(w, http.StatusInternalServerError, "설정 저장에 실패했습니다.")
		return
	}
	respondOK(w, map[string]any{"code": 200, "auto_check_update": req.AutoCheckUpdate, "message": "업데이트 자동 확인 설정이 저장되었습니다."})
}

func handleGetRemoteTesterStatus(w http.ResponseWriter, r *http.Request) {
	respondOK(w, map[string]any{"code": 200, "unlocked": core.GetRemoteTesterUnlocked()})
}

func handleRemoteTesterAuth(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code   string `json:"code"`
		Action string `json:"action"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if req.Action == "revoke" {
		_ = core.SetRemoteTesterUnlocked(false)
		respondOK(w, map[string]any{"code": 200, "unlocked": false, "message": "테스터 모드가 해제되었습니다."})
		return
	}
	cleanCode := strings.TrimSpace(strings.ToLower(req.Code))
	if cleanCode == "chzzk" || cleanCode == "tester" || cleanCode == "test" || cleanCode == "0600" {
		_ = core.SetRemoteTesterUnlocked(true)
		respondOK(w, map[string]any{"code": 200, "unlocked": true, "message": "테스터 인증이 완료되었습니다."})
		return
	}
	respondErr(w, http.StatusUnauthorized, "유효하지 않은 인증 코드입니다.")
}

// --- [OBS 연동 & 업데이트 & 텔레메트리 핸들러] ---

func handleObsScriptStatus(w http.ResponseWriter, r *http.Request) {
	scriptsDir, detected := core.DetectObsScriptsDir()
	installed, needsUpdate := core.CheckScriptStatus(scriptsDir, getLauncherScriptData())
	respondOK(w, map[string]any{
		"code":         200,
		"detected":     detected,
		"path":         scriptsDir,
		"installed":    installed,
		"needs_update": needsUpdate,
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
	var req struct {
		DownloadURL   string `json:"download_url"`
		LatestVersion string `json:"latest_version"`
	}
	_ = bindJSON(w, r, &req)
	if req.DownloadURL == "" || req.LatestVersion == "" {
		info, err := core.CheckForUpdate(APP_VERSION, false)
		if err == nil && info != nil {
			if req.DownloadURL == "" {
				req.DownloadURL = info.DownloadURL
			}
			if req.LatestVersion == "" {
				req.LatestVersion = info.LatestVersion
			}
		}
	}
	if req.DownloadURL == "" {
		respondErr(w, http.StatusBadRequest, "다운로드 URL이 제공되지 않았습니다.")
		return
	}
	if err := core.StartDownloadAndInstall(req.LatestVersion, req.DownloadURL, 0, killAllSubProcesses); err != nil {
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

// --- [정적 페이지 및 스크립트 서빙 핸들러] ---

func handleIndexHtml(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	sendBytes(w, getRenderedHTML(), http.StatusOK, "text/html; charset=utf-8")
}

func handleStatsHtml(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	sendBytes(w, getRenderedStatsHTML(), http.StatusOK, "text/html; charset=utf-8")
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
