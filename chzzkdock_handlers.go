package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"chzzk-dock/core"
)

// ============================================================
//  ChzzkDock 전용 BFF (Backend-For-Frontend) 서비스 & 핸들러
//  - 네임스페이스: /chzzkdock/...
//  - 역할: 치지직 비공식 API의 깊은 중첩 및 다단계 호출을
//         독 화면에 필요한 최소 규격의 깔끔한 JSON으로 통합 제공
// ============================================================

// --- [공통 DTO 및 내부 헬퍼] ---

type CleanChatMode struct {
	EmojiOnly bool `json:"emojiOnly"`
	SlowSec   int  `json:"slowSec"`
}

type CleanDonations struct {
	Chat         bool `json:"chat"`
	Video        bool `json:"video"`
	Mission      bool `json:"mission"`
	Subscription bool `json:"subscription"`
}

type CleanChannelProfile struct {
	ChannelID             string `json:"channelId"`
	Nickname              string `json:"nickname"`
	ProfileImageURL       string `json:"profileImageUrl"`
	FollowerCount         int    `json:"followerCount"`
	IsLive                bool   `json:"isLive"`
	SubscriptionAvailable bool   `json:"subscriptionAvailable"`
}

type CleanLiveStatus struct {
	Status              string `json:"status"`
	IsLive              bool   `json:"isLive"`
	ConcurrentUserCount int    `json:"concurrentUserCount"`
}

type CleanChatRule struct {
	ChatAvailableGroup            string `json:"chatAvailableGroup"`
	MinFollowerMinute             int    `json:"minFollowerMinute"`
	AllowSubscriberInFollowerMode bool   `json:"allowSubscriberInFollowerMode"`
}

// resolveChzzkChannelId: 현재 활성 세션 쿠키에서 채널 ID 자동 추출
func resolveChzzkChannelId(_ *http.Request) (string, error) {
	if cached := GetCachedActiveChannelId(); cached != "" {
		return cached, nil
	}
	cfg := core.LoadConfig()
	if cfg.NidAut != "" && cfg.NidSes != "" {
		if id, _, _, ok := fetchSessionUserStatus(cfg.NidAut, cfg.NidSes); ok && id != "" {
			SetCachedActiveChannelId(id)
			return id, nil
		}
	}
	return "", fmt.Errorf("로그인이 필요하거나 채널 식별자를 확인할 수 없습니다.")
}

// callChzzk: 치지직 API 통합 호출 클라이언트 (단일 우체부)
// withAuth가 true이면 네이버 로그인 쿠키(NID_AUT, NID_SES)와 보안 헤더를 자동 주입합니다.
func callChzzk(method, path string, bodyBytes []byte, withAuth bool) (int, []byte, error) {
	targetURL := chzzkApiBaseURL + path
	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, targetURL, bodyReader)
	if err != nil {
		return http.StatusInternalServerError, nil, err
	}
	req.Header.Set("User-Agent", USER_AGENT)

	if withAuth {
		cfg := core.LoadConfig()
		if cfg.NidAut == "" || cfg.NidSes == "" {
			return http.StatusUnauthorized, nil, fmt.Errorf("네이버 로그인 쿠키가 설정되지 않았습니다.")
		}
		if len(bodyBytes) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Cookie", fmt.Sprintf("NID_AUT=%s; NID_SES=%s", cfg.NidAut, cfg.NidSes))
		req.Header.Set("Origin", "https://chzzk.naver.com")
		req.Header.Set("Referer", "https://chzzk.naver.com/")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return http.StatusBadGateway, nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, respBytes, nil
}

// --- [1. 채팅 모드 서비스 로직] ---

func fetchChatModeFromChzzk(channelId string) (*CleanChatMode, error) {
	path := fmt.Sprintf("/manage/v1/channels/%s/live-setting/chat-mode", channelId)
	status, data, err := callChzzk("GET", path, nil, true)
	if err != nil || status != http.StatusOK {
		return nil, fmt.Errorf("채팅 설정 조회 실패 (status: %d, err: %v)", status, err)
	}

	var raw struct {
		Content struct {
			ChatEmojiMode   bool `json:"chatEmojiMode"`
			ChatSlowModeSec int  `json:"chatSlowModeSec"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("채팅 설정 파싱 실패: %w", err)
	}

	return &CleanChatMode{
		EmojiOnly: raw.Content.ChatEmojiMode,
		SlowSec:   raw.Content.ChatSlowModeSec,
	}, nil
}

func updateChatEmojiOnChzzk(channelId string, emojiOnly bool) error {
	path := fmt.Sprintf("/manage/v1/channels/%s/live-setting/chat-mode/emoji", channelId)
	body, _ := json.Marshal(map[string]bool{"chatEmojiMode": emojiOnly})
	status, _, err := callChzzk("PATCH", path, body, true)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("이모티콘 전용 모드 변경 실패 (status: %d)", status)
	}
	return nil
}

func updateChatSlowOnChzzk(channelId string, slowSec int) error {
	path := fmt.Sprintf("/manage/v1/channels/%s/live-setting/chat-mode/slow", channelId)
	body, _ := json.Marshal(map[string]int{"chatSlowModeSec": slowSec})
	status, _, err := callChzzk("PATCH", path, body, true)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("저속 모드 변경 실패 (status: %d)", status)
	}
	return nil
}

// --- [2. 후원 제어 서비스 로직] ---

func fetchDonationsFromChzzk(channelId string) (*CleanDonations, error) {
	// A. 후원 3종(채팅, 영상, 미션) 조회
	pathDon := fmt.Sprintf("/manage/v1/channels/%s/donations/setting", channelId)
	statusDon, dataDon, errDon := callChzzk("GET", pathDon, nil, true)
	if errDon != nil || statusDon != http.StatusOK {
		return nil, fmt.Errorf("후원 설정 조회 실패 (status: %d, err: %v)", statusDon, errDon)
	}

	var rawDon struct {
		Content struct {
			ChatDonationActive    bool `json:"chatDonationActive"`
			VideoDonationActive   bool `json:"videoDonationActive"`
			MissionDonationActive bool `json:"missionDonationActive"`
		} `json:"content"`
	}
	_ = json.Unmarshal(dataDon, &rawDon)

	// B. 구독 알림 설정 조회 (루키 회원은 권한 없음 등으로 조회가 안 되므로 기본값 false)
	subActive := false
	pathSub := fmt.Sprintf("/manage/v1/channels/%s/donations/subscription-alert-setting", channelId)
	statusSub, dataSub, errSub := callChzzk("GET", pathSub, nil, true)
	if errSub == nil && statusSub == http.StatusOK {
		var rawSub struct {
			Content struct {
				AlertActive *bool `json:"alertActive"`
			} `json:"content"`
		}
		if err := json.Unmarshal(dataSub, &rawSub); err == nil && rawSub.Content.AlertActive != nil {
			subActive = *rawSub.Content.AlertActive
		}
	}

	return &CleanDonations{
		Chat:         rawDon.Content.ChatDonationActive,
		Video:        rawDon.Content.VideoDonationActive,
		Mission:      rawDon.Content.MissionDonationActive,
		Subscription: subActive,
	}, nil
}

func updateDonationActiveOnChzzk(channelId string, dType string, active bool) error {
	path := fmt.Sprintf("/manage/v1/channels/%s/donations/active-setting", channelId)
	body, _ := json.Marshal(map[string]any{
		"donationType": dType,
		"active":       active,
	})
	status, _, err := callChzzk("PUT", path, body, true)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("후원 설정 토글 실패 (status: %d)", status)
	}
	return nil
}

func updateSubscriptionAlertOnChzzk(channelId string, active bool) error {
	path := fmt.Sprintf("/manage/v1/channels/%s/donations/subscription-alert-setting", channelId)
	// 기존 설정 조회하여 필드 병합 후 PATCH
	status, data, err := callChzzk("GET", path, nil, true)
	payload := map[string]any{"alertActive": active}
	if err == nil && status == http.StatusOK {
		var raw struct {
			Content map[string]any `json:"content"`
		}
		if err := json.Unmarshal(data, &raw); err == nil && raw.Content != nil {
			payload = raw.Content
			payload["alertActive"] = active
		}
	}

	body, _ := json.Marshal(payload)
	patchStatus, _, patchErr := callChzzk("PATCH", path, body, true)
	if patchErr != nil || patchStatus != http.StatusOK {
		return fmt.Errorf("구독 알림 설정 변경 실패 (status: %d)", patchStatus)
	}
	return nil
}

// ============================================================
//  HTTP 핸들러 구현체
// ============================================================

// --- [채팅 모드 핸들러] ---

// handleChzzkChatModeGet: GET /chzzkdock/chat-mode (일괄 조회)
func handleChzzkChatModeGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	mode, err := fetchChatModeFromChzzk(cid)
	if err != nil {
		respondErr(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, mode)
}

// handleChzzkChatEmojiPut: PUT /chzzkdock/chat-mode/emoji (이모티콘 전용 모드 명시적 변경)
func handleChzzkChatEmojiPut(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if err := updateChatEmojiOnChzzk(cid, req.Enabled); err != nil {
		respondErr(w, http.StatusBadGateway, err.Error())
		return
	}
	mode, _ := fetchChatModeFromChzzk(cid)
	respondOK(w, mode)
}

// handleChzzkChatSlowPut: PUT /chzzkdock/chat-mode/slow (저속 모드 딜레이 시간 명시적 변경)
func handleChzzkChatSlowPut(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req struct {
		SlowSec int `json:"slowSec"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if err := updateChatSlowOnChzzk(cid, req.SlowSec); err != nil {
		respondErr(w, http.StatusBadGateway, err.Error())
		return
	}
	mode, _ := fetchChatModeFromChzzk(cid)
	respondOK(w, mode)
}

// --- [후원 제어 핸들러] ---

// handleChzzkDonationsGet: GET /chzzkdock/donations (4종 일괄 조회)
func handleChzzkDonationsGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	don, err := fetchDonationsFromChzzk(cid)
	if err != nil {
		respondErr(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, don)
}

// handleChzzkDonationToggle: 공통 개별 후원 토글 (chat, video, mission, subscription)
func handleChzzkDonationToggle(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid, err := resolveChzzkChannelId(r)
		if err != nil {
			respondErr(w, http.StatusUnauthorized, err.Error())
			return
		}
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if !bindJSON(w, r, &req) {
			return
		}

		var updateErr error
		switch strings.ToLower(target) {
		case "chat":
			updateErr = updateDonationActiveOnChzzk(cid, "CHAT", req.Enabled)
		case "video":
			updateErr = updateDonationActiveOnChzzk(cid, "VIDEO", req.Enabled)
		case "mission":
			updateErr = updateDonationActiveOnChzzk(cid, "MISSION", req.Enabled)
		case "subscription", "sub":
			updateErr = updateSubscriptionAlertOnChzzk(cid, req.Enabled)
		default:
			respondErr(w, http.StatusBadRequest, "알 수 없는 후원 유형입니다.")
			return
		}

		if updateErr != nil {
			respondErr(w, http.StatusBadGateway, updateErr.Error())
			return
		}

		// 최신 4종 전체 상태 반환
		don, err := fetchDonationsFromChzzk(cid)
		if err != nil {
			respondErr(w, http.StatusBadGateway, "최신 후원 설정 동기화 실패: "+err.Error())
			return
		}
		respondOK(w, don)
	}
}
// --- [후원 세부 설정 (모달) & 정산 수익 서비스 & 핸들러] ---

type CleanDonationDetails struct {
	Chat         any `json:"chat"`
	Video        any `json:"video"`
	Mission      any `json:"mission"`
	Subscription any `json:"subscription,omitempty"`
}

func fetchDonationDetailsFromChzzk(channelId string) (*CleanDonationDetails, error) {
	details := &CleanDonationDetails{}
	targets := []struct {
		path string
		dest *any
	}{
		{fmt.Sprintf("/manage/v1/channels/%s/donations/chat-setting", channelId), &details.Chat},
		{fmt.Sprintf("/manage/v1/channels/%s/donations/video-setting", channelId), &details.Video},
		{fmt.Sprintf("/manage/v1/channels/%s/donations/mission/setting", channelId), &details.Mission},
		{fmt.Sprintf("/manage/v1/channels/%s/donations/subscription-alert-setting", channelId), &details.Subscription},
	}

	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Add(1)
		go func(path string, dest *any) {
			defer wg.Done()
			status, data, err := callChzzk("GET", path, nil, true)
			if err == nil && status == http.StatusOK {
				var raw struct {
					Content any `json:"content"`
				}
				if json.Unmarshal(data, &raw) == nil && raw.Content != nil {
					*dest = raw.Content
				}
			}
		}(t.path, t.dest)
	}
	wg.Wait()
	return details, nil
}

// handleChzzkDonationDetailsGet: GET /chzzkdock/donations/details (후원 4종 세부 설정 일괄 조회)
func handleChzzkDonationDetailsGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	details, err := fetchDonationDetailsFromChzzk(cid)
	if err != nil {
		respondErr(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, details)
}

// handleChzzkDonationDetailChatPut: PUT /chzzkdock/donations/details/chat (채팅 후원 세부 설정 저장)
func handleChzzkDonationDetailChatPut(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var body map[string]any
	if !bindJSON(w, r, &body) {
		return
	}

	// 1) 노출 최소 금액 단독 수정 처리
	if exp, ok := body["exposureDonationAmount"]; ok {
		expPath := fmt.Sprintf("/manage/v1/channels/%s/donations/chat-setting/exposure_amount", cid)
		expBody, _ := json.Marshal(map[string]any{"exposureDonationAmount": exp})
		status, _, err := callChzzk("PUT", expPath, expBody, true)
		if err != nil || status != http.StatusOK {
			respondErr(w, http.StatusBadGateway, "노출 금액 설정 실패")
			return
		}
		// exposure_amount 전용 호출인 경우 즉시 응답
		if len(body) == 1 {
			respondOK(w, map[string]string{"status": "ok"})
			return
		}
	}

	// 2) 일반 채팅 후원 조건 저장
	chatPath := fmt.Sprintf("/manage/v1/channels/%s/donations/chat-setting", cid)
	chatPayload, _ := json.Marshal(body)
	status, data, err := callChzzk("PUT", chatPath, chatPayload, true)
	if err != nil || status != http.StatusOK {
		respondErr(w, http.StatusBadGateway, "채팅 후원 조건 저장 실패")
		return
	}

	// 3) donationActive가 명시되어 있다면 active-setting도 동기화
	if activeVal, ok := body["donationActive"].(bool); ok {
		_ = updateDonationActiveOnChzzk(cid, "CHAT", activeVal)
	}

	var resObj any
	_ = json.Unmarshal(data, &resObj)
	respondOK(w, resObj)
}

// handleChzzkDonationDetailVideoPut: PUT /chzzkdock/donations/details/video (영상 후원 세부 설정 저장)
func handleChzzkDonationDetailVideoPut(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var body map[string]any
	if !bindJSON(w, r, &body) {
		return
	}

	videoPath := fmt.Sprintf("/manage/v1/channels/%s/donations/video-setting", cid)
	payload, _ := json.Marshal(body)
	status, data, err := callChzzk("PUT", videoPath, payload, true)
	if err != nil || status != http.StatusOK {
		respondErr(w, http.StatusBadGateway, "영상 후원 설정 저장 실패")
		return
	}

	if activeVal, ok := body["donationActive"].(bool); ok {
		_ = updateDonationActiveOnChzzk(cid, "VIDEO", activeVal)
	}

	var resObj any
	_ = json.Unmarshal(data, &resObj)
	respondOK(w, resObj)
}

// handleChzzkDonationDetailMissionPut: PUT /chzzkdock/donations/details/mission (미션 후원 세부 설정 저장)
func handleChzzkDonationDetailMissionPut(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var body map[string]any
	if !bindJSON(w, r, &body) {
		return
	}
	body["channelId"] = cid

	missionPath := fmt.Sprintf("/manage/v1/channels/%s/donations/mission/setting", cid)
	payload, _ := json.Marshal(body)
	status, data, err := callChzzk("PUT", missionPath, payload, true)
	if err != nil || status != http.StatusOK {
		respondErr(w, http.StatusBadGateway, "미션 후원 설정 저장 실패")
		return
	}

	if activeVal, ok := body["donationActive"].(bool); ok {
		_ = updateDonationActiveOnChzzk(cid, "MISSION", activeVal)
	}

	var resObj any
	_ = json.Unmarshal(data, &resObj)
	respondOK(w, resObj)
}

// handleChzzkDonationDetailSubscriptionPatch: PATCH /chzzkdock/donations/details/subscription (구독 알림 세부 설정 저장)
func handleChzzkDonationDetailSubscriptionPatch(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var body map[string]any
	if !bindJSON(w, r, &body) {
		return
	}

	subPath := fmt.Sprintf("/manage/v1/channels/%s/donations/subscription-alert-setting", cid)
	payload, _ := json.Marshal(body)
	status, data, err := callChzzk("PATCH", subPath, payload, true)
	if err != nil || status != http.StatusOK {
		respondErr(w, http.StatusBadGateway, "구독 알림 설정 저장 실패")
		return
	}

	var resObj any
	_ = json.Unmarshal(data, &resObj)
	respondOK(w, resObj)
}

// handleChzzkRevenueSummaryGet: GET /chzzkdock/revenue/summary (월간 정산 수익 요약)
func handleChzzkRevenueSummaryGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	ym := r.URL.Query().Get("yearMonth")
	if ym == "" {
		ym = r.URL.Query().Get("fromYearMonth")
	}
	if ym == "" {
		ym = time.Now().Format("200601")
	}

	path := fmt.Sprintf("/manage/v2/channels/%s/revenue/summary?channelId=%s&fromYearMonth=%s&toYearMonth=%s", cid, cid, ym, ym)
	status, data, err := callChzzk("GET", path, nil, true)
	if err != nil {
		respondErr(w, http.StatusBadGateway, "정산 수익 조회 실패: "+err.Error())
		return
	}
	if status == http.StatusForbidden {
		respondErr(w, http.StatusForbidden, "수익 창출 계약(프로/파트너) 이후 확인 가능합니다.")
		return
	}
	if status != http.StatusOK {
		respondErr(w, status, fmt.Sprintf("정산 수익 조회 오류 (status: %d)", status))
		return
	}

	var raw struct {
		Content any `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err == nil && raw.Content != nil {
		respondOK(w, raw.Content)
	} else {
		var rawObj any
		_ = json.Unmarshal(data, &rawObj)
		respondOK(w, rawObj)
	}
}
// --- [스트림 키 & 방송 위젯 정보 서비스 & 핸들러] ---

// CleanStreamingUrls: 방송 8대 위젯 오버레이 URL
type CleanStreamingUrls struct {
	ChatUrl        string `json:"chatUrl,omitempty"`
	DonationUrl    string `json:"donationUrl,omitempty"`
	VideoUrl       string `json:"videoUrl,omitempty"`
	MissionUrl     string `json:"missionUrl,omitempty"`
	PartyAmountUrl string `json:"partyAmountUrl,omitempty"`
	PartyTickerUrl string `json:"partyTickerUrl,omitempty"`
	GiftUrl        string `json:"giftUrl,omitempty"`
	ShopUrl        string `json:"shopUrl,omitempty"`
}

// CleanStreamingInfo: 스트림 키 + 8대 위젯 URL 통합
type CleanStreamingInfo struct {
	StreamKey          string `json:"streamKey"`
	StreamUrl          string `json:"streamUrl,omitempty"`
	CleanStreamingUrls        // 임베딩
}

func fetchStreamingRawFromChzzk(channelId string) (*CleanStreamingInfo, error) {
	path := fmt.Sprintf("/manage/v1/channels/%s/streaming-info", channelId)
	status, data, err := callChzzk("GET", path, nil, true)
	if err != nil || status != http.StatusOK {
		return nil, fmt.Errorf("스트리밍 정보 조회 실패 (status: %d)", status)
	}
	var raw struct {
		Content struct {
			StreamKey                      string `json:"streamKey"`
			StreamUrl                      string `json:"streamUrl"`
			ChatSourceHash                 string `json:"chatSourceHash"`
			DonationSessionIOChannelId     string `json:"donationSessionIOChannelId"`
			VideoSessionIOChannelId        string `json:"videoSessionIOChannelId"`
			MissionSessionIOChannelId      string `json:"missionSessionIOChannelId"`
			PartyDonationSessionIOChannelId string `json:"partyDonationSessionIOChannelId"`
			GiftSessionIOChannelId         string `json:"giftSessionIOChannelId"`
			StreamerShopSessionIOChannelId string `json:"streamerShopSessionIOChannelId"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	c := raw.Content
	info := &CleanStreamingInfo{
		StreamKey: c.StreamKey,
		StreamUrl: c.StreamUrl,
	}
	if c.ChatSourceHash != "" {
		info.ChatUrl = "https://chzzk.naver.com/chat/" + c.ChatSourceHash
	}
	if c.DonationSessionIOChannelId != "" {
		info.DonationUrl = "https://chzzk.naver.com/donation/" + c.DonationSessionIOChannelId
	}
	if c.VideoSessionIOChannelId != "" {
		info.VideoUrl = "https://chzzk.naver.com/video-donation/" + c.VideoSessionIOChannelId
	}
	if c.MissionSessionIOChannelId != "" {
		info.MissionUrl = "https://chzzk.naver.com/mission-donation/" + c.MissionSessionIOChannelId
	}
	if c.PartyDonationSessionIOChannelId != "" {
		info.PartyAmountUrl = "https://chzzk.naver.com/party-donation/amount/" + c.PartyDonationSessionIOChannelId
		info.PartyTickerUrl = "https://chzzk.naver.com/party-donation/ticker/" + c.PartyDonationSessionIOChannelId
	}
	if c.GiftSessionIOChannelId != "" {
		info.GiftUrl = "https://chzzk.naver.com/gift-effect/" + c.GiftSessionIOChannelId
	}
	if c.StreamerShopSessionIOChannelId != "" {
		info.ShopUrl = "https://chzzk.naver.com/product-purchase/" + c.StreamerShopSessionIOChannelId
	}

	return info, nil
}

// handleChzzkStreamingUrlsGet: GET /chzzkdock/streaming-urls (방송 위젯 URL 8개 일괄 조회)
func handleChzzkStreamingUrlsGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	info, err := fetchStreamingRawFromChzzk(cid)
	if err != nil {
		respondErr(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, info.CleanStreamingUrls)
}

// handleChzzkStreamKeyGet: GET /chzzkdock/streamkey (스트림 키 단독 조회)
func handleChzzkStreamKeyGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	info, err := fetchStreamingRawFromChzzk(cid)
	if err != nil {
		respondErr(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, map[string]string{
		"streamKey": info.StreamKey,
		"streamUrl": info.StreamUrl,
	})
}

func reissueStreamKeyOnChzzk(channelId string) (string, error) {
	path := fmt.Sprintf("/manage/v1/channels/%s/streams/change-key", channelId)
	status, data, err := callChzzk("PUT", path, nil, true)
	if err != nil || status != http.StatusOK {
		return "", fmt.Errorf("스트림 키 재발급 실패 (status: %d)", status)
	}
	var raw struct {
		Content struct {
			StreamKey string `json:"streamKey"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", err
	}
	return raw.Content.StreamKey, nil
}

// handleChzzkStreamKeyPut: PUT /chzzkdock/streamkey (스트림 키 즉시 재발급)
func handleChzzkStreamKeyPut(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	newKey, err := reissueStreamKeyOnChzzk(cid)
	if err != nil {
		respondErr(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, map[string]string{"streamKey": newKey})
}

// --- [채널 프로필 서비스 & 핸들러] ---

func fetchChannelProfileFromChzzk(channelId string) (*CleanChannelProfile, error) {
	path := fmt.Sprintf("/service/v1/channels/%s", channelId)
	status, data, err := callChzzk("GET", path, nil, false)
	if err != nil || status != http.StatusOK {
		return nil, fmt.Errorf("채널 프로필 조회 실패 (status: %d)", status)
	}

	var raw struct {
		Content struct {
			ChannelID                string `json:"channelId"`
			ChannelName              string `json:"channelName"`
			ChannelImageURL          string `json:"channelImageUrl"`
			FollowerCount            int    `json:"followerCount"`
			OpenLive                 bool   `json:"openLive"`
			SubscriptionAvailability bool   `json:"subscriptionAvailability"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("채널 프로필 파싱 실패: %w", err)
	}

	return &CleanChannelProfile{
		ChannelID:             raw.Content.ChannelID,
		Nickname:              raw.Content.ChannelName,
		ProfileImageURL:       raw.Content.ChannelImageURL,
		FollowerCount:         raw.Content.FollowerCount,
		IsLive:                raw.Content.OpenLive,
		SubscriptionAvailable: raw.Content.SubscriptionAvailability,
	}, nil
}

// handleChzzkProfileGet: GET /chzzkdock/profile (채널 공개 프로필 조회)
func handleChzzkProfileGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	profile, err := fetchChannelProfileFromChzzk(cid)
	if err != nil {
		respondErr(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, profile)
}

// fetchLiveStatusFromChzzk: 치지직 공식 실시간 라이브 상태 조회 (쿠키 불필요)
func fetchLiveStatusFromChzzk(channelID string) (*CleanLiveStatus, error) {
	status, data, err := callChzzk("GET", fmt.Sprintf("/polling/v2/channels/%s/live-status", channelID), nil, false)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("치지직 라이브 상태 조회 실패 (HTTP %d)", status)
	}

	var raw struct {
		Content struct {
			Status              string `json:"status"`
			ConcurrentUserCount int    `json:"concurrentUserCount"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("라이브 상태 파싱 실패: %w", err)
	}

	isOpen := strings.ToUpper(raw.Content.Status) == "OPEN"
	return &CleanLiveStatus{
		Status:              raw.Content.Status,
		IsLive:              isOpen,
		ConcurrentUserCount: raw.Content.ConcurrentUserCount,
	}, nil
}

// handleChzzkLiveStatusGet: GET /chzzkdock/live-status (실시간 방송 상태 및 시청자 수 조회)
func handleChzzkLiveStatusGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	liveStatus, err := fetchLiveStatusFromChzzk(cid)
	if err != nil {
		respondErr(w, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(w, liveStatus)
}

// handleChzzkUserStatusGet: GET /chzzkdock/user-status (현재 로그인 세션 상태 및 채널 ID 단독 조회)
func handleChzzkUserStatusGet(w http.ResponseWriter, r *http.Request) {
	cfg := core.LoadConfig()
	if cfg.NidAut == "" || cfg.NidSes == "" {
		respondOK(w, map[string]any{
			"loggedIn": false,
		})
		return
	}
	cid, cname, avatar, ok := fetchSessionUserStatus(cfg.NidAut, cfg.NidSes)
	if !ok {
		respondOK(w, map[string]any{
			"loggedIn": false,
		})
		return
	}
	SetCachedActiveChannelId(cid)
	respondOK(w, map[string]any{
		"loggedIn":        true,
		"channelId":       cid,
		"nickname":        cname,
		"profileImageUrl": avatar,
	})
}

// handleChzzkSessionBootstrap: GET /chzzkdock/session/bootstrap
// 프론트엔드 기동 시 단 1회 왕복으로 인증 상태, 서버 설정 마스크, 세션 목록, 채널 프로필을 일괄 제공
func handleChzzkSessionBootstrap(w http.ResponseWriter, r *http.Request) {
	core.InvalidateConfigCache()
	cfg := core.LoadConfig()

	autMask, sesMask := "", ""
	if cfg.NidAut != "" {
		autMask = "••••••••••••••••••••••••••••••••"
	}
	if cfg.NidSes != "" {
		sesMask = "••••••••••••••••••••••••••••••••"
	}

	configMap := map[string]any{
		"nid_aut": autMask,
		"nid_ses": sesMask,
	}

	views := buildSessionViews(cfg)

	loggedIn := false
	channelID := ""
	channelName := ""
	profileImageURL := ""

	if len(views) > 0 && !views[0].Expired && views[0].ChannelID != "" {
		loggedIn = true
		channelID = views[0].ChannelID
		channelName = views[0].ChannelName
		profileImageURL = views[0].ProfileImageURL
	} else if cfg.NidAut != "" && cfg.NidSes != "" {
		cid, cname, avatar, ok := fetchSessionUserStatus(cfg.NidAut, cfg.NidSes)
		if ok {
			loggedIn = true
			channelID = cid
			channelName = cname
			profileImageURL = avatar
		}
	}

	if loggedIn && channelID != "" {
		SetCachedActiveChannelId(channelID)
	}

	// 프로필 이미지/닉네임 보강 (필요한 경우)
	if loggedIn && (channelName == "" || profileImageURL == "") && channelID != "" {
		if prof, err := fetchChannelProfileFromChzzk(channelID); err == nil && prof != nil {
			if channelName == "" {
				channelName = prof.Nickname
			}
			if profileImageURL == "" {
				profileImageURL = prof.ProfileImageURL
			}
		}
	}

	respondOK(w, map[string]any{
		"loggedIn":        loggedIn,
		"channelId":       channelID,
		"nickname":        channelName,
		"profileImageUrl": profileImageURL,
		"config":          configMap,
		"sessions":        views,
	})
}

// handleChzzkChatRuleGet: GET /chzzkdock/chat-rule (채팅 참여 권한 규칙 조회)
func handleChzzkChatRuleGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	path := fmt.Sprintf("/manage/v1/channels/%s/live-setting/chat", cid)
	status, data, err := callChzzk("GET", path, nil, true)
	if err != nil || status != http.StatusOK {
		respondErr(w, http.StatusBadGateway, "채팅 규칙 조회 실패")
		return
	}
	var raw struct {
		Content CleanChatRule `json:"content"`
	}
	_ = json.Unmarshal(data, &raw)
	respondOK(w, raw.Content)
}

// handleChzzkChatRulePut: PUT /chzzkdock/chat-rule (채팅 참여 권한 규칙 즉시 변경)
func handleChzzkChatRulePut(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req CleanChatRule
	if !bindJSON(w, r, &req) {
		return
	}
	path := fmt.Sprintf("/manage/v1/channels/%s/live-setting/chat", cid)
	body, _ := json.Marshal(req)
	status, data, err := callChzzk("PUT", path, body, true)
	if err != nil || status != http.StatusOK {
		respondErr(w, http.StatusBadGateway, "채팅 규칙 변경 실패")
		return
	}
	var raw struct {
		Content CleanChatRule `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err == nil && raw.Content.ChatAvailableGroup != "" {
		respondOK(w, raw.Content)
	} else {
		respondOK(w, req)
	}
}

// handleChzzkBroadcastGet: GET /chzzkdock/broadcast (방송 기본 설정 원본 조회)
func handleChzzkBroadcastGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	path := fmt.Sprintf("/manage/v1/channels/%s/live-setting/normal", cid)
	status, data, err := callChzzk("GET", path, nil, true)
	if err != nil || status != http.StatusOK {
		respondErr(w, http.StatusBadGateway, "방송 설정 조회 실패")
		return
	}
	var raw struct {
		Content json.RawMessage `json:"content"`
	}
	_ = json.Unmarshal(data, &raw)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw.Content)
}

// handleChzzkBroadcastPut: PUT /chzzkdock/broadcast (방송 기본 설정 저장)
func handleChzzkBroadcastPut(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "요청 바디 읽기 실패")
		return
	}
	path := fmt.Sprintf("/manage/v1/channels/%s/live-setting/normal", cid)
	status, data, err := callChzzk("PUT", path, body, true)
	if err != nil || status != http.StatusOK {
		// [드롭스 Fallback 자동 복구] 치지직이 드롭스 관련 에러를 반환한 경우,
		// dropsCampaignNo를 null로 변경하여 즉시 1회 자동 재시도
		if strings.Contains(string(data), "드롭스") {
			var retryPayload map[string]any
			if jErr := json.Unmarshal(body, &retryPayload); jErr == nil {
				retryPayload["dropsCampaignNo"] = nil
				retryBody, _ := json.Marshal(retryPayload)
				retryStatus, retryData, retryErr := callChzzk("PUT", path, retryBody, true)
				if retryErr == nil && retryStatus == http.StatusOK {
					core.LogInfo("[BFF] Broadcast PUT 드롭스 에러 감지 -> dropsCampaignNo: null 자동 복구 성공")
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-ChzzkDock-Fallback", "drops_disabled")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(retryData)
					return
				}
			}
		}

		msg := "방송 설정 저장 실패"
		if len(data) > 0 {
			var errRes struct {
				Message string `json:"message"`
			}
			if jErr := json.Unmarshal(data, &errRes); jErr == nil && errRes.Message != "" {
				msg = errRes.Message
			}
		}
		summary := strings.TrimSpace(string(data))
		if len(summary) > 120 {
			summary = summary[:120] + "..."
		}
		core.LogWarn("[BFF] Broadcast PUT failed (status %d): %s (err: %v)", status, summary, err)
		respondErr(w, http.StatusBadGateway, msg)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

var categoryAliases = map[string]string{
	"롤":                 "리그 오브 레전드",
	"lol":               "리그 오브 레전드",
	"리그":                "리그 오브 레전드",
	"league":            "리그 오브 레전드",
	"league of legends": "리그 오브 레전드",
	"leagueoflegends":   "리그 오브 레전드",
	"리그오브레전드":           "리그 오브 레전드",
	"배그":                "PUBG: 배틀그라운드",
	"pubg":              "PUBG: 배틀그라운드",
	"배틀그라운드":            "PUBG: 배틀그라운드",
	"옵치":                "오버워치",
	"옵치2":               "오버워치 2",
	"오버워치2":             "오버워치 2",
	"overwatch":         "오버워치",
	"로아":                "로스트아크",
	"lostark":           "로스트아크",
	"lost ark":          "로스트아크",
	"로스트아크":             "로스트아크",
	"발로":                "발로란트",
	"발로란트":              "발로란트",
	"valorant":          "발로란트",
	"저챗":                "Talk",
	"저스트채팅":             "Talk",
	"토크":                "Talk",
	"소통":                "Talk",
	"chat":              "Talk",
	"talk":              "Talk",
	"마크":                "마인크래프트",
	"minecraft":         "마인크래프트",
	"마인크래프트":            "마인크래프트",
	"멮":                 "메이플스토리",
	"메플":                "메이플스토리",
	"메이플":               "메이플스토리",
	"메이플스토리":            "메이플스토리",
	"maplestory":        "메이플스토리",
	"던파":                "던전앤파이터",
	"dnf":               "던전앤파이터",
	"던전앤파이터":            "던전앤파이터",
	"스타":                "스타크래프트",
	"starcraft":         "스타크래프트",
	"스타크래프트":            "스타크래프트",
	"에펙":                "Apex 레전드",
	"apex":              "Apex 레전드",
	"apex레전드":           "Apex 레전드",
	"철권":                "철권",
	"철권8":               "철권 8",
	"tekken":            "철권",
	"몬헌":                "몬스터 헌터",
	"몬스터헌터":             "몬스터 헌터",
	"원신":                "원신",
	"genshin":           "원신",
	"붕스":                "붕괴: 스타레일",
	"스타레일":              "붕괴: 스타레일",
	"붕괴스타레일":            "붕괴: 스타레일",
	"블아":                "블루 아카이브",
	"블루아카":              "블루 아카이브",
	"블루아카이브":            "블루 아카이브",
	"명조":                "명조: 워더링 웨이브",
	"팰월드":               "팰월드",
	"palworld":          "팰월드",
	"스팀":                "스팀",
}

// handleChzzkCategoriesSearchGet: GET /chzzkdock/categories/search?keyword=... (공식 카테고리 자동완성 검색 + ALIAS 지원)
func handleChzzkCategoriesSearchGet(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("query"))
	}
	if q == "" {
		respondOK(w, []any{})
		return
	}
	qLower := strings.ToLower(q)
	if target, exists := categoryAliases[qLower]; exists {
		q = target
	}
	size := strings.TrimSpace(r.URL.Query().Get("size"))
	if size == "" {
		size = "30"
	}
	path := fmt.Sprintf("/manage/v1/auto-complete/categories?keyword=%s&size=%s", url.QueryEscape(q), size)
	status, data, err := callChzzk("GET", path, nil, false)
	if err != nil || status != http.StatusOK {
		respondErr(w, http.StatusBadGateway, "카테고리 검색 실패")
		return
	}
	var raw struct {
		Content struct {
			Results []json.RawMessage `json:"results"`
		} `json:"content"`
	}
	_ = json.Unmarshal(data, &raw)
	respondOK(w, raw.Content.Results)
}

// handleChzzkCategoriesRecommendedGet: GET /chzzkdock/categories/recommended (치지직 실시간 인기 카테고리 13개 풀 제공: 최대 즐겨찾기 8개 + 추천 5개)
func handleChzzkCategoriesRecommendedGet(w http.ResponseWriter, r *http.Request) {
	fetchSize := 13
	targetURL := fmt.Sprintf("https://api.chzzk.naver.com/service/v1/categories/live?size=%d", fetchSize)
	if cached, found := core.GetCachedApiResponse("GET", targetURL); found {
		sendBytes(w, cached.Body, cached.Status, cached.ContentType)
		return
	}

	path := fmt.Sprintf("/service/v1/categories/live?size=%d", fetchSize)
	status, data, err := callChzzk("GET", path, nil, false)
	if err != nil || status != http.StatusOK {
		respondErr(w, http.StatusBadGateway, "치지직 API 응답 실패")
		return
	}

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

	if err := json.Unmarshal(data, &apiRes); err != nil {
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

// handleChzzkWatchPartiesGet: GET /chzzkdock/watch-parties (현재 채널의 같이보기 목록 조회)
func handleChzzkWatchPartiesGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	path := fmt.Sprintf("/manage/v1/channels/%s/watch-parties", cid)
	status, data, err := callChzzk("GET", path, nil, true)
	if err != nil || status != http.StatusOK {
		respondOK(w, map[string]any{"watchParties": []any{}})
		return
	}
	var raw struct {
		Content struct {
			WatchParties []json.RawMessage `json:"watchParties"`
		} `json:"content"`
	}
	_ = json.Unmarshal(data, &raw)
	respondOK(w, map[string]any{"watchParties": raw.Content.WatchParties})
}

// handleChzzkWatchPartySnapshotGet: GET /chzzkdock/snapshot (영속화된 같이보기 직전 원본 방송 설정 스냅샷 조회)
func handleChzzkWatchPartySnapshotGet(w http.ResponseWriter, r *http.Request) {
	snap := core.GetWatchPartySnapshot()
	respondOK(w, map[string]any{"snapshot": snap})
}

// handleChzzkWatchPartySnapshotPost: POST /chzzkdock/snapshot (같이보기 직전 원본 방송 설정 스냅샷 저장)
func handleChzzkWatchPartySnapshotPost(w http.ResponseWriter, r *http.Request) {
	var snap core.WatchPartySnapshot
	if !bindJSON(w, r, &snap) {
		return
	}
	if err := core.SaveWatchPartySnapshot(&snap); err != nil {
		core.LogError("[Settings] 같이보기 스냅샷 저장 실패: %v", err)
		respondErr(w, http.StatusInternalServerError, "스냅샷 저장에 실패했습니다.")
		return
	}
	respondOK(w, map[string]any{"snapshot": snap, "saved": true})
}

// handleChzzkWatchPartySnapshotDelete: DELETE /chzzkdock/snapshot (일반 방송 복귀 후 스냅샷 초기화)
func handleChzzkWatchPartySnapshotDelete(w http.ResponseWriter, r *http.Request) {
	if err := core.SaveWatchPartySnapshot(nil); err != nil {
		core.LogError("[Settings] 같이보기 스냅샷 삭제 실패: %v", err)
		respondErr(w, http.StatusInternalServerError, "스냅샷 삭제에 실패했습니다.")
		return
	}
	respondOK(w, map[string]any{"deleted": true})
}

// handleChzzkDropsCampaignsGet: GET /chzzkdock/drops/campaigns (채널 실시간 활성 드롭스 캠페인 목록)
func handleChzzkDropsCampaignsGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	path := fmt.Sprintf("/manage/v1/channels/%s/drops/campaigns", cid)
	status, data, err := callChzzk("GET", path, nil, true)
	if err != nil || status != http.StatusOK {
		respondOK(w, []any{})
		return
	}
	var raw struct {
		Content struct {
			CampaignList []json.RawMessage `json:"campaignList"`
		} `json:"content"`
	}
	_ = json.Unmarshal(data, &raw)
	respondOK(w, raw.Content.CampaignList)
}

type dropsRewardItem struct {
	Name          string `json:"name"`
	ImageURL      string `json:"imageUrl"`
	ConditionText string `json:"conditionText"`
}

type dropsMatchResponse struct {
	Supported    bool              `json:"supported"`
	CampaignNo   int64             `json:"campaignNo,omitempty"`
	Title        string            `json:"title,omitempty"`
	LinkURL      string            `json:"linkUrl,omitempty"`
	IsTerminated bool              `json:"isTerminated,omitempty"`
	Rewards      []dropsRewardItem `json:"rewards,omitempty"`
}

func formatDropsConditionText(rawSec int) string {
	if rawSec <= 0 {
		return ""
	}
	min := rawSec / 60
	if rawSec < 60 {
		min = rawSec
	}
	h := min / 60
	m := min % 60
	if h > 0 && m > 0 {
		return fmt.Sprintf("%d시간 %d분 시청", h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%d시간 시청", h)
	}
	return fmt.Sprintf("%d분 시청", m)
}

// handleChzzkDropsMatchGet: GET /chzzkdock/drops/match?campaignNo=... 또는 ?category=... (드롭스 상세 보상 정보 직통 조회)
func handleChzzkDropsMatchGet(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("campaignNo"))
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("category"))
	}
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("categoryId"))
	}
	if q == "" {
		respondOK(w, dropsMatchResponse{Supported: false})
		return
	}

	var matchedCampaignNo int64
	var matchedTitle string

	// 1. 숫자가 전달된 경우: 불필요한 채널 목록 역추적 없이 치지직 공개 상세 API 직통 조회
	if no, err := strconv.ParseInt(q, 10, 64); err == nil && no > 0 {
		matchedCampaignNo = no
	} else {
		// 2. 카테고리 문자열이 전달된 경우의 폴백: 채널 활성 목록에서 카테고리명/ID 대조
		cid, err := resolveChzzkChannelId(r)
		if err != nil {
			respondOK(w, dropsMatchResponse{Supported: false})
			return
		}

		listPath := fmt.Sprintf("/manage/v1/channels/%s/drops/campaigns", cid)
		status, data, err := callChzzk("GET", listPath, nil, true)
		if err == nil && status == http.StatusOK {
			var rawList struct {
				Content struct {
					CampaignList []struct {
						CampaignNo    int64  `json:"campaignNo"`
						CampaignName  string `json:"campaignName"`
						CategoryID    string `json:"categoryId"`
						CategoryValue string `json:"categoryValue"`
						Category      *struct {
							CategoryID    string `json:"categoryId"`
							CategoryValue string `json:"categoryValue"`
						} `json:"category"`
					} `json:"campaignList"`
				} `json:"content"`
			}
			if json.Unmarshal(data, &rawList) == nil {
				qNorm := strings.ToLower(q)
				for _, c := range rawList.Content.CampaignList {
					id := strings.ToLower(strings.TrimSpace(c.CategoryID))
					val := strings.ToLower(strings.TrimSpace(c.CategoryValue))
					if c.Category != nil {
						if id == "" {
							id = strings.ToLower(strings.TrimSpace(c.Category.CategoryID))
						}
						if val == "" {
							val = strings.ToLower(strings.TrimSpace(c.Category.CategoryValue))
						}
					}
					if (id != "" && id == qNorm) || (val != "" && val == qNorm) || strconv.FormatInt(c.CampaignNo, 10) == qNorm {
						matchedCampaignNo = c.CampaignNo
						matchedTitle = c.CampaignName
						break
					}
				}
			}
		}
	}

	if matchedCampaignNo == 0 {
		respondOK(w, dropsMatchResponse{Supported: false})
		return
	}

	// 치지직 공개 캠페인 상세 API 직통 호출
	detailPath := fmt.Sprintf("/service/v2/drops/campaigns/%d", matchedCampaignNo)
	status, data, err := callChzzk("GET", detailPath, nil, false)
	if err != nil || status != http.StatusOK {
		detailPath = fmt.Sprintf("/service/v1/drops/campaigns/%d", matchedCampaignNo)
		status, data, err = callChzzk("GET", detailPath, nil, false)
	}

	res := dropsMatchResponse{
		Supported:  true,
		CampaignNo: matchedCampaignNo,
		Title:      matchedTitle,
	}

	if err == nil && status == http.StatusOK {
		var rawDetail struct {
			Content struct {
				CampaignName string `json:"campaignName"`
				Title        string `json:"title"`
				PCLinkURL    string `json:"pcLinkUrl"`
				State        string `json:"state"`
				RewardList   []struct {
					Title               string `json:"title"`
					ImageURL            string `json:"imageUrl"`
					ConditionForMinutes int    `json:"conditionForMinutes"`
				} `json:"rewardList"`
				Rewards []struct {
					Name           string `json:"name"`
					RewardName     string `json:"rewardName"`
					Title          string `json:"title"`
					ImageURL       string `json:"imageUrl"`
					RewardImageURL string `json:"rewardImageUrl"`
					IconURL        string `json:"iconUrl"`
					Condition      struct {
						ConditionType  string `json:"conditionType"`
						ConditionValue int    `json:"conditionValue"`
					} `json:"condition"`
					Item *struct {
						Name     string `json:"name"`
						ImageURL string `json:"imageUrl"`
					} `json:"item"`
				} `json:"rewards"`
			} `json:"content"`
		}
		if json.Unmarshal(data, &rawDetail) == nil && (rawDetail.Content.Title != "" || rawDetail.Content.CampaignName != "") {
			if rawDetail.Content.Title != "" {
				res.Title = rawDetail.Content.Title
			} else if rawDetail.Content.CampaignName != "" {
				res.Title = rawDetail.Content.CampaignName
			}
			res.LinkURL = rawDetail.Content.PCLinkURL
			res.IsTerminated = rawDetail.Content.State == "TERMINATED"

			if len(rawDetail.Content.RewardList) > 0 {
				for _, r := range rawDetail.Content.RewardList {
					condText := ""
					if r.ConditionForMinutes > 0 {
						condText = formatDropsConditionText(r.ConditionForMinutes * 60)
					}
					res.Rewards = append(res.Rewards, dropsRewardItem{
						Name:          r.Title,
						ImageURL:      r.ImageURL,
						ConditionText: condText,
					})
				}
			} else {
				for _, r := range rawDetail.Content.Rewards {
					name := r.Name
					if name == "" {
						name = r.RewardName
					}
					if name == "" {
						name = r.Title
					}
					if name == "" && r.Item != nil {
						name = r.Item.Name
					}
					img := r.ImageURL
					if img == "" {
						img = r.RewardImageURL
					}
					if img == "" {
						img = r.IconURL
					}
					if img == "" && r.Item != nil {
						img = r.Item.ImageURL
					}
					condText := formatDropsConditionText(r.Condition.ConditionValue)
					res.Rewards = append(res.Rewards, dropsRewardItem{
						Name:          name,
						ImageURL:      img,
						ConditionText: condText,
					})
				}
			}
		}
	}

	respondOK(w, res)
}

// ============================================================
//  [FEAT-01] 리모컨 · 파티 · 멤버 채널 열기 BFF 핸들러
// ============================================================

var (
	openBrowserFn   = core.OpenBrowser
	partyTokenRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{4,128}$`)
)

func extractChzzkMessage(data []byte, fallback string) string {
	var errRes struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &errRes) == nil && strings.TrimSpace(errRes.Message) != "" {
		return errRes.Message
	}
	return fallback
}

func callChzzkChecked(w http.ResponseWriter, method, path string, bodyBytes []byte, fallbackErr string) (int, []byte, bool) {
	status, data, err := callChzzk(method, path, bodyBytes, true)
	if err != nil {
		respondErr(w, http.StatusBadGateway, fallbackErr)
		return status, data, false
	}
	if status >= 400 {
		respondErr(w, status, extractChzzkMessage(data, fallbackErr))
		return status, data, false
	}
	return status, data, true
}

func handleChzzkRemoteStateGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}

	var (
		paused              bool
		videoDonationActive = true
		ttsVol              = 70
		videoVol            = 70
		chatAlertVol        = 70
		videoAlertVol       = 70
		missionAlertVol     = 70
		unauthCount         int
		mu                  sync.Mutex
		wg                  sync.WaitGroup
	)

	fetchJSON := func(subPath string, fn func(content []byte)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, data, callErr := callChzzk("GET", fmt.Sprintf("/manage/v1/channels/%s/%s", cid, subPath), nil, true)
			if status == http.StatusUnauthorized {
				mu.Lock()
				unauthCount++
				mu.Unlock()
				return
			}
			if callErr != nil || status != http.StatusOK {
				return
			}
			var wrap struct {
				Content json.RawMessage `json:"content"`
			}
			if json.Unmarshal(data, &wrap) == nil && len(wrap.Content) > 0 && string(wrap.Content) != "null" {
				mu.Lock()
				fn(wrap.Content)
				mu.Unlock()
			}
		}()
	}

	fetchJSON("donations/pause-active", func(c []byte) {
		var v struct {
			Active bool `json:"active"`
		}
		if json.Unmarshal(c, &v) == nil {
			paused = v.Active
		}
	})
	fetchJSON("donations/chat-alert-setting", func(c []byte) {
		var v struct {
			TtsVolume   *int `json:"ttsVolume"`
			AlertVolume *int `json:"alertVolume"`
		}
		if json.Unmarshal(c, &v) == nil {
			if v.TtsVolume != nil {
				ttsVol = *v.TtsVolume
			}
			if v.AlertVolume != nil {
				chatAlertVol = *v.AlertVolume
			}
		}
	})
	fetchJSON("donations/video-alert-setting", func(c []byte) {
		var v struct {
			VideoVolume *int `json:"videoVolume"`
			AlertVolume *int `json:"alertVolume"`
		}
		if json.Unmarshal(c, &v) == nil {
			if v.VideoVolume != nil {
				videoVol = *v.VideoVolume
			}
			if v.AlertVolume != nil {
				videoAlertVol = *v.AlertVolume
			}
		}
	})
	fetchJSON("donations/mission-alert-setting", func(c []byte) {
		var v struct {
			AlertVolume *int `json:"alertVolume"`
		}
		if json.Unmarshal(c, &v) == nil && v.AlertVolume != nil {
			missionAlertVol = *v.AlertVolume
		}
	})
	fetchJSON("donations/video-setting", func(c []byte) {
		var v struct {
			DonationActive *bool `json:"donationActive"`
		}
		if json.Unmarshal(c, &v) == nil && v.DonationActive != nil {
			videoDonationActive = *v.DonationActive
		}
	})

	wg.Wait()
	if unauthCount == 5 {
		respondErr(w, http.StatusUnauthorized, "네이버 세션이 만료되었습니다.")
		return
	}

	respondOK(w, map[string]any{
		"paused":              paused,
		"videoDonationActive": videoDonationActive,
		"volumes": map[string]int{
			"tts":          ttsVol,
			"video":        videoVol,
			"chatAlert":    chatAlertVol,
			"videoAlert":   videoAlertVol,
			"missionAlert": missionAlertVol,
		},
	})
}

func handleChzzkRemoteFeedsGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}

	size := 20
	if rawSize := strings.TrimSpace(r.URL.Query().Get("size")); rawSize != "" {
		if parsed, convErr := strconv.Atoi(rawSize); convErr == nil && parsed > 0 && parsed <= 100 {
			size = parsed
		}
	}
	typeFilters := strings.TrimSpace(r.URL.Query().Get("typeFilters"))

	path := fmt.Sprintf("/manage/v1/channels/%s/news-feeds?size=%d", cid, size)
	if typeFilters != "" {
		path += "&typeFilters=" + url.QueryEscape(typeFilters)
	}

	_, data, ok := callChzzkChecked(w, "GET", path, nil, "알림 피드 조회 실패")
	if !ok {
		return
	}

	var raw struct {
		Content json.RawMessage `json:"content"`
	}
	feeds := []json.RawMessage{}
	if json.Unmarshal(data, &raw) == nil && len(raw.Content) > 0 && string(raw.Content) != "null" {
		var paged struct {
			Data []json.RawMessage `json:"data"`
		}
		if json.Unmarshal(raw.Content, &paged) == nil && paged.Data != nil {
			feeds = paged.Data
		} else {
			var direct []json.RawMessage
			if json.Unmarshal(raw.Content, &direct) == nil && direct != nil {
				feeds = direct
			}
		}
	}

	respondOK(w, map[string]any{"feeds": feeds})
}

func handleChzzkRemotePausePut(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req struct {
		Active bool `json:"active"`
	}
	if !bindJSON(w, r, &req) {
		return
	}

	body, _ := json.Marshal(map[string]bool{"active": req.Active})
	path := fmt.Sprintf("/manage/v1/channels/%s/donations/pause-active", cid)
	if _, _, ok := callChzzkChecked(w, "PUT", path, body, "알림 정지/재개 변경 실패"); !ok {
		return
	}
	respondOK(w, map[string]any{"paused": req.Active})
}

func handleChzzkRemoteVolumePatch(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req struct {
		Target string `json:"target"`
		Volume int    `json:"volume"`
	}
	if !bindJSON(w, r, &req) {
		return
	}

	vol := req.Volume
	if vol < 0 {
		vol = 0
	} else if vol > 100 {
		vol = 100
	}

	target := strings.ToUpper(strings.TrimSpace(req.Target))
	var endpoint, field string
	var isChatAlert bool
	switch target {
	case "TTS":
		endpoint, field, isChatAlert = "chat-alert-setting", "ttsVolume", true
	case "CHAT":
		endpoint, field, isChatAlert = "chat-alert-setting", "alertVolume", true
	case "VIDEO":
		endpoint, field = "video-alert-setting", "videoVolume"
	case "VIDEO_ALERT":
		endpoint, field = "video-alert-setting", "alertVolume"
	case "MISSION":
		endpoint, field = "mission-alert-setting", "alertVolume"
	default:
		respondErr(w, http.StatusBadRequest, "지원하지 않는 볼륨 대상입니다.")
		return
	}

	path := fmt.Sprintf("/manage/v1/channels/%s/donations/%s", cid, endpoint)
	_, getData, ok := callChzzkChecked(w, "GET", path, nil, "볼륨 설정 조회 실패")
	if !ok {
		return
	}

	var raw struct {
		Content map[string]any `json:"content"`
	}
	if err := json.Unmarshal(getData, &raw); err != nil || raw.Content == nil {
		respondErr(w, http.StatusBadGateway, "볼륨 설정 파싱 실패")
		return
	}

	payload := raw.Content
	payload[field] = vol
	if isChatAlert {
		if _, exists := payload["blockTtsSpeakerTypes"]; !exists {
			if list, ok := payload["blockTtsList"]; ok && list != nil {
				payload["blockTtsSpeakerTypes"] = list
			} else {
				payload["blockTtsSpeakerTypes"] = []string{}
			}
		}
		delete(payload, "blockTtsList")
	}

	patchBody, _ := json.Marshal(payload)
	if _, _, ok := callChzzkChecked(w, "PATCH", path, patchBody, "볼륨 설정 저장 실패"); !ok {
		return
	}
	respondOK(w, map[string]any{"target": target, "volume": vol})
}

func handleChzzkRemoteCommandPost(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req struct {
		Command    string `json:"command"`
		DonationID string `json:"donationId"`
		NewsFeedNo any    `json:"newsFeedNo"`
	}
	if !bindJSON(w, r, &req) {
		return
	}

	cmd := strings.ToUpper(strings.TrimSpace(req.Command))
	var subPath string
	switch cmd {
	case "DONATION_TTS_REPLAY", "SUBSCRIPTION_TTS_REPLAY":
		subPath = "tts/replay-command"
	case "SKIP", "PLAY", "STOP", "IGNORE":
		subPath = "donations/command"
	default:
		respondErr(w, http.StatusBadRequest, "지원하지 않는 리모컨 명령입니다.")
		return
	}

	payload := map[string]any{"command": cmd}
	if strings.TrimSpace(req.DonationID) != "" {
		payload["donationId"] = strings.TrimSpace(req.DonationID)
	}
	if req.NewsFeedNo != nil {
		payload["newsFeedNo"] = req.NewsFeedNo
	}

	body, _ := json.Marshal(payload)
	path := fmt.Sprintf("/manage/v1/channels/%s/%s", cid, subPath)
	if _, _, ok := callChzzkChecked(w, "POST", path, body, "리모컨 명령 전송 실패"); !ok {
		return
	}
	respondOK(w, map[string]any{"status": "ok", "command": cmd})
}

func handleChzzkRemoteAdPostponePost(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	path := fmt.Sprintf("/manage/v2/channels/%s/live-ad-schedule/postpone", cid)
	if _, _, ok := callChzzkChecked(w, "POST", path, nil, "중간 광고 미루기 실패"); !ok {
		return
	}
	respondOK(w, map[string]any{"status": "ok"})
}

func handleChzzkRemoteMissionSettlePut(outcome string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid, err := resolveChzzkChannelId(r)
		if err != nil {
			respondErr(w, http.StatusUnauthorized, err.Error())
			return
		}
		missionID := strings.TrimSpace(r.URL.Query().Get("missionDonationId"))
		if missionID == "" && r.Body != nil {
			var req struct {
				MissionDonationID string `json:"missionDonationId"`
			}
			if json.NewDecoder(r.Body).Decode(&req) == nil {
				missionID = strings.TrimSpace(req.MissionDonationID)
			}
		}
		if missionID == "" {
			respondErr(w, http.StatusBadRequest, "미션 ID가 필요합니다.")
			return
		}

		path := fmt.Sprintf("/manage/v1/channels/%s/donations/mission/%s?missionDonationId=%s", cid, outcome, url.QueryEscape(missionID))
		if _, _, ok := callChzzkChecked(w, "PUT", path, nil, "미션 판정 처리 실패"); !ok {
			return
		}
		respondOK(w, map[string]any{"status": "ok", "outcome": outcome})
	}
}

type chzzkPartyMemberRaw struct {
	PartyMemberNo   int64  `json:"partyMemberNo"`
	Type            string `json:"type"`
	ChannelID       string `json:"channelId"`
	ChannelImageURL string `json:"channelImageUrl"`
	ChannelName     string `json:"channelName"`
	VerifiedMark    bool   `json:"verifiedMark"`
}

type CleanPartyMember struct {
	PartyMemberNo   int64  `json:"partyMemberNo"`
	ChannelID       string `json:"channelId"`
	ChannelImageURL string `json:"channelImageUrl"`
	ChannelName     string `json:"channelName"`
	VerifiedMark    bool   `json:"verifiedMark"`
	IsHost          bool   `json:"isHost"`
	IsMe            bool   `json:"isMe"`
}

func extractInviteTokenFromContent(data []byte) string {
	var raw struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(data, &raw) != nil || len(raw.Content) == 0 {
		return ""
	}
	var obj struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(raw.Content, &obj) == nil && obj.Token != "" {
		return obj.Token
	}
	var direct string
	if json.Unmarshal(raw.Content, &direct) == nil {
		return direct
	}
	return ""
}

func handleChzzkRemotePartyGet(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}

	path := fmt.Sprintf("/manage/v1/channels/%s/party", cid)
	_, data, ok := callChzzkChecked(w, "GET", path, nil, "파티 상태 조회 실패")
	if !ok {
		return
	}

	var raw struct {
		Content struct {
			Participating bool `json:"participating"`
			Info          *struct {
				PartyNo     int64                 `json:"partyNo"`
				PartyName   string                `json:"partyName"`
				Status      string                `json:"status"`
				IsHost      bool                  `json:"isHost"`
				MemberCount int                   `json:"memberCount"`
				Host        *chzzkPartyMemberRaw  `json:"host"`
				Guests      []chzzkPartyMemberRaw `json:"guests"`
			} `json:"info"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		respondErr(w, http.StatusBadGateway, "파티 응답 파싱 실패")
		return
	}

	if !raw.Content.Participating || raw.Content.Info == nil {
		respondOK(w, map[string]any{
			"participating": false,
			"partyNo":       0,
			"partyName":     "",
			"status":        "",
			"isHost":        false,
			"memberCount":   0,
			"inviteToken":   "",
			"inviteUrl":     "",
			"members":       []CleanPartyMember{},
			"myChannelId":   cid,
		})
		return
	}

	info := raw.Content.Info
	isHost := info.IsHost || (info.Host != nil && info.Host.ChannelID == cid)
	members := make([]CleanPartyMember, 0, 1+len(info.Guests))
	if info.Host != nil {
		members = append(members, CleanPartyMember{
			PartyMemberNo:   info.Host.PartyMemberNo,
			ChannelID:       info.Host.ChannelID,
			ChannelImageURL: info.Host.ChannelImageURL,
			ChannelName:     info.Host.ChannelName,
			VerifiedMark:    info.Host.VerifiedMark,
			IsHost:          true,
			IsMe:            info.Host.ChannelID == cid,
		})
	}
	for _, g := range info.Guests {
		members = append(members, CleanPartyMember{
			PartyMemberNo:   g.PartyMemberNo,
			ChannelID:       g.ChannelID,
			ChannelImageURL: g.ChannelImageURL,
			ChannelName:     g.ChannelName,
			VerifiedMark:    g.VerifiedMark,
			IsHost:          false,
			IsMe:            g.ChannelID == cid,
		})
	}

	memberCount := info.MemberCount
	if memberCount <= 0 {
		memberCount = len(members)
	}

	inviteToken := ""
	inviteURL := ""
	if isHost && info.PartyNo > 0 {
		tokPath := fmt.Sprintf("/manage/v1/channels/%s/parties/%d/invitation/token", cid, info.PartyNo)
		if tStatus, tData, tErr := callChzzk("GET", tokPath, nil, true); tErr == nil && tStatus == http.StatusOK {
			inviteToken = extractInviteTokenFromContent(tData)
			if inviteToken != "" {
				inviteURL = "https://studio.chzzk.naver.com/party-invite/" + inviteToken
			}
		}
	}

	respondOK(w, map[string]any{
		"participating": true,
		"partyNo":       info.PartyNo,
		"partyName":     info.PartyName,
		"status":        info.Status,
		"isHost":        isHost,
		"memberCount":   memberCount,
		"inviteToken":   inviteToken,
		"inviteUrl":     inviteURL,
		"members":       members,
		"myChannelId":   cid,
	})
}

func handleChzzkRemotePartyCreatePost(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req struct {
		PartyName string `json:"partyName"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.PartyName)
	if name == "" {
		respondErr(w, http.StatusBadRequest, "파티명을 입력해 주세요.")
		return
	}

	path := fmt.Sprintf("/manage/v1/channels/%s/party?partyName=%s", cid, url.QueryEscape(name))
	if _, _, ok := callChzzkChecked(w, "POST", path, []byte("{}"), "파티 생성 실패"); !ok {
		return
	}
	respondOK(w, map[string]any{"status": "ok", "partyName": name})
}

func handleChzzkRemotePartyNamePut(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req struct {
		PartyName string `json:"partyName"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.PartyName)
	if name == "" {
		respondErr(w, http.StatusBadRequest, "변경할 파티명을 입력해 주세요.")
		return
	}

	path := fmt.Sprintf("/manage/v1/channels/%s/party/name?partyName=%s", cid, url.QueryEscape(name))
	if _, _, ok := callChzzkChecked(w, "PUT", path, []byte("{}"), "파티명 변경 실패"); !ok {
		return
	}
	respondOK(w, map[string]any{"status": "ok", "partyName": name})
}

func handlePartyTokenRequest(w http.ResponseWriter, r *http.Request, method string) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	partyNo, convErr := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("partyNo")), 10, 64)
	if convErr != nil || partyNo <= 0 {
		respondErr(w, http.StatusBadRequest, "유효한 파티 번호가 필요합니다.")
		return
	}

	var body []byte
	if method == http.MethodPatch {
		body = []byte("{}")
	}
	path := fmt.Sprintf("/manage/v1/channels/%s/parties/%d/invitation/token", cid, partyNo)
	_, data, ok := callChzzkChecked(w, method, path, body, "초대 링크 처리 실패")
	if !ok {
		return
	}

	token := extractInviteTokenFromContent(data)
	inviteURL := ""
	if token != "" {
		inviteURL = "https://studio.chzzk.naver.com/party-invite/" + token
	}
	respondOK(w, map[string]any{"token": token, "inviteUrl": inviteURL})
}

func handleChzzkRemotePartyTokenGet(w http.ResponseWriter, r *http.Request) {
	handlePartyTokenRequest(w, r, http.MethodGet)
}

func handleChzzkRemotePartyTokenPatch(w http.ResponseWriter, r *http.Request) {
	handlePartyTokenRequest(w, r, http.MethodPatch)
}

func handleChzzkRemotePartyJoinPost(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	token := strings.TrimSpace(req.Token)
	if !partyTokenRegex.MatchString(token) {
		respondErr(w, http.StatusBadRequest, "유효한 파티 초대 코드가 아닙니다.")
		return
	}

	partyTitle := ""
	sumPath := fmt.Sprintf("/manage/v1/channels/%s/party/invitation/%s/summary", cid, url.PathEscape(token))
	if sStatus, sData, sErr := callChzzk("GET", sumPath, nil, true); sErr == nil && sStatus == http.StatusOK {
		var sum struct {
			Content struct {
				PartyName string `json:"partyName"`
				PartyInfo *struct {
					PartyName string `json:"partyName"`
				} `json:"partyInfo"`
			} `json:"content"`
		}
		if json.Unmarshal(sData, &sum) == nil {
			if sum.Content.PartyInfo != nil && sum.Content.PartyInfo.PartyName != "" {
				partyTitle = sum.Content.PartyInfo.PartyName
			} else {
				partyTitle = sum.Content.PartyName
			}
		}
	}

	acceptPath := fmt.Sprintf("/manage/v1/channels/%s/party/invitation/%s/accept", cid, url.PathEscape(token))
	if _, _, ok := callChzzkChecked(w, "POST", acceptPath, []byte("{}"), "파티 합류 실패"); !ok {
		return
	}
	respondOK(w, map[string]any{"status": "ok", "partyName": partyTitle})
}

func handleChzzkRemotePartyLeavePost(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req struct {
		PartyNo int64 `json:"partyNo"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	if req.PartyNo <= 0 {
		respondErr(w, http.StatusBadRequest, "유효한 파티 번호가 필요합니다.")
		return
	}

	path := fmt.Sprintf("/manage/v1/channels/%s/parties/%d/member/leave", cid, req.PartyNo)
	if _, _, ok := callChzzkChecked(w, "POST", path, []byte("{}"), "파티 퇴장 실패"); !ok {
		return
	}
	respondOK(w, map[string]any{"status": "ok"})
}

func handleChzzkRemotePartyEndPost(w http.ResponseWriter, r *http.Request) {
	cid, err := resolveChzzkChannelId(r)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	st := strings.ToUpper(strings.TrimSpace(req.Status))
	if st == "WAITING" {
		renewPath := fmt.Sprintf("/manage/v1/channels/%s/party/renew", cid)
		if _, _, ok := callChzzkChecked(w, "POST", renewPath, []byte("{}"), "파티 종료 실패"); !ok {
			return
		}
		respondOK(w, map[string]any{"status": "ok", "method": "renew"})
		return
	}

	finishPath := fmt.Sprintf("/manage/v1/channels/%s/party/finish", cid)
	status, data, callErr := callChzzk("PUT", finishPath, []byte("{}"), true)
	if callErr != nil {
		respondErr(w, http.StatusBadGateway, "파티 종료 실패")
		return
	}
	if status == http.StatusOK {
		respondOK(w, map[string]any{"status": "ok", "method": "finish"})
		return
	}
	if status == http.StatusBadRequest || status == http.StatusNotFound || status == http.StatusConflict {
		renewPath := fmt.Sprintf("/manage/v1/channels/%s/party/renew", cid)
		if _, _, ok := callChzzkChecked(w, "POST", renewPath, []byte("{}"), extractChzzkMessage(data, "파티 종료 실패")); !ok {
			return
		}
		respondOK(w, map[string]any{"status": "ok", "method": "renew"})
		return
	}

	respondErr(w, status, extractChzzkMessage(data, "파티 종료 실패"))
}

func handleChzzkOpenChannelPost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChannelID string `json:"channelId"`
	}
	if !bindJSON(w, r, &req) {
		return
	}
	cid := strings.TrimSpace(req.ChannelID)
	if !channelIdRegex.MatchString(cid) {
		respondErr(w, http.StatusBadRequest, "유효한 채널 ID가 아닙니다.")
		return
	}
	openBrowserFn("https://chzzk.naver.com/live/" + cid)
	respondOK(w, map[string]any{"status": "ok"})
}
