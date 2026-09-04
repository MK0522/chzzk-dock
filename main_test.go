package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestHttpDockHandlerRouting(t *testing.T) {
	os.Setenv("CHZZK_CRED_TARGET", "ChzzkObsDockTest/HttpTest")
	defer func() {
		os.Unsetenv("CHZZK_CRED_TARGET")
	}()

	// 1. Static HTML Serving 테스트
	reqHTML := httptest.NewRequest(http.MethodGet, "http://localhost:8081/", nil)
	wHTML := httptest.NewRecorder()
	HttpDockHandler(wHTML, reqHTML)

	if wHTML.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for static HTML, got %d", wHTML.Code)
	}
	if wHTML.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html Content-Type, got %s", wHTML.Header().Get("Content-Type"))
	}

	// 2. CORS Preflight OPTIONS 요청 테스트
	reqOptions := httptest.NewRequest(http.MethodOptions, "http://localhost:8081/config", nil)
	reqOptions.Header.Set("Origin", "http://localhost:8081")
	wOptions := httptest.NewRecorder()
	HttpDockHandler(wOptions, reqOptions)

	if wOptions.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for OPTIONS, got %d", wOptions.Code)
	}
	if wOptions.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8081" {
		t.Fatalf("expected CORS header set, got %s", wOptions.Header().Get("Access-Control-Allow-Origin"))
	}

	// 3. /config 보안 차단 테스트 (X-Requested-With 미포함)
	reqConfigNoAuth := httptest.NewRequest(http.MethodGet, "http://localhost:8081/config", nil)
	wConfigNoAuth := httptest.NewRecorder()
	HttpDockHandler(wConfigNoAuth, reqConfigNoAuth)

	if wConfigNoAuth.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for unauthenticated /config, got %d", wConfigNoAuth.Code)
	}

	// 4. /config 정상 요청 테스트 (X-Requested-With 포함)
	reqConfigAuth := httptest.NewRequest(http.MethodGet, "http://localhost:8081/config", nil)
	reqConfigAuth.Header.Set("X-Requested-With", "ChzzkDock")
	wConfigAuth := httptest.NewRecorder()
	HttpDockHandler(wConfigAuth, reqConfigAuth)

	if wConfigAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for authenticated /config, got %d", wConfigAuth.Code)
	}

	// 5. /save-config 저장 및 /config 확인
	savePayload := map[string]string{
		"nid_aut": "saved_aut_val",
		"nid_ses": "saved_ses_val",
	}
	saveBytes, _ := json.Marshal(savePayload)
	reqSave := httptest.NewRequest(http.MethodPost, "http://localhost:8081/save-config", bytes.NewReader(saveBytes))
	reqSave.Header.Set("X-Requested-With", "ChzzkDock")
	wSave := httptest.NewRecorder()
	HttpDockHandler(wSave, reqSave)

	if wSave.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /save-config, got %d", wSave.Code)
	}

	// 6. /logout 테스트
	reqLogout := httptest.NewRequest(http.MethodPost, "http://localhost:8081/logout", nil)
	reqLogout.Header.Set("X-Requested-With", "ChzzkDock")
	wLogout := httptest.NewRecorder()
	HttpDockHandler(wLogout, reqLogout)

	if wLogout.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /logout, got %d", wLogout.Code)
	}

	// 7. 404 Not Found 테스트
	req404 := httptest.NewRequest(http.MethodGet, "http://localhost:8081/invalid_unknown_route", nil)
	w404 := httptest.NewRecorder()
	HttpDockHandler(w404, req404)

	if w404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown route, got %d", w404.Code)
	}

	// 8. /open-browser 보안 및 파라미터 검증 테스트
	reqOpenNoAuth := httptest.NewRequest(http.MethodGet, "http://localhost:8081/open-browser?url=https://chzzk.naver.com", nil)
	wOpenNoAuth := httptest.NewRecorder()
	HttpDockHandler(wOpenNoAuth, reqOpenNoAuth)
	if wOpenNoAuth.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unauthenticated /open-browser, got %d", wOpenNoAuth.Code)
	}

	reqOpenBadURL := httptest.NewRequest(http.MethodGet, "http://localhost:8081/open-browser?url=ftp://bad.com", nil)
	reqOpenBadURL.Header.Set("X-Requested-With", "ChzzkDock")
	wOpenBadURL := httptest.NewRecorder()
	HttpDockHandler(wOpenBadURL, reqOpenBadURL)
	if wOpenBadURL.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad url scheme in /open-browser, got %d", wOpenBadURL.Code)
	}
}
