package core

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckSecurityDNSRebinding(t *testing.T) {
	// 1. 유효한 Host 검증
	validHosts := []string{"localhost", "localhost:8081", "127.0.0.1", "127.0.0.1:8081", "::1", "[::1]:8081"}
	for _, host := range validHosts {
		req := httptest.NewRequest(http.MethodGet, "http://"+host+"/config", nil)
		req.Host = host
		w := httptest.NewRecorder()
		if !CheckSecurity(w, req) {
			t.Fatalf("expected valid host %s to pass security check", host)
		}
	}

	// 2. 악의적인 DNS Rebinding 공격 Host 검증
	invalidHosts := []string{"attacker.com", "evil.attacker.com:8081", "192.168.1.5:8081", "10.0.0.1"}
	for _, host := range invalidHosts {
		req := httptest.NewRequest(http.MethodGet, "http://"+host+"/config", nil)
		req.Host = host
		w := httptest.NewRecorder()
		if CheckSecurity(w, req) {
			t.Fatalf("expected invalid host %s to be blocked by DNS rebinding protection", host)
		}
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d", w.Code)
		}
	}
}

func TestCheckApiAuthLocalCSRF(t *testing.T) {
	// 1. 정상 커스텀 헤더 포함 요청
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8081/config", nil)
	req.Header.Set("X-Requested-With", "ChzzkDock")
	w := httptest.NewRecorder()
	if !CheckApiAuth(w, req) {
		t.Fatalf("expected request with X-Requested-With: ChzzkDock to pass")
	}

	// 2. 헤더 누락 요청 (CSRF 차단)
	reqWithoutHeader := httptest.NewRequest(http.MethodGet, "http://localhost:8081/config", nil)
	w2 := httptest.NewRecorder()
	if CheckApiAuth(w2, reqWithoutHeader) {
		t.Fatalf("expected request without X-Requested-With header to be blocked")
	}
	if w2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w2.Code)
	}

	// 3. 잘못된 헤더 값 요청
	reqWithWrongHeader := httptest.NewRequest(http.MethodGet, "http://localhost:8081/config", nil)
	reqWithWrongHeader.Header.Set("X-Requested-With", "SomeMaliciousApp")
	w3 := httptest.NewRecorder()
	if CheckApiAuth(w3, reqWithWrongHeader) {
		t.Fatalf("expected request with invalid header value to be blocked")
	}
	if w3.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w3.Code)
	}
}

func TestRateLimiterCache(t *testing.T) {
	testURL := "https://api.chzzk.naver.com/manage/v1/live/settings"
	testBody := []byte(`{"code": 200, "data": "test_cache"}`)

	// 1. 초기 상태: 캐시 미스
	if _, found := GetCachedApiResponse(http.MethodGet, testURL); found {
		t.Fatalf("expected initial cache miss")
	}

	// 2. 캐시 저장
	SetCachedApiResponse(http.MethodGet, testURL, testBody, http.StatusOK, "application/json")

	// 3. 캐시 조회 성공 확인
	cached, found := GetCachedApiResponse(http.MethodGet, testURL)
	if !found || string(cached.Body) != string(testBody) {
		t.Fatalf("expected cache hit with matching body")
	}

	// 4. 상태 변경(POST/PUT/PATCH/DELETE) 발생 시 캐시 무효화 확인
	SetCachedApiResponse(http.MethodPatch, testURL, nil, http.StatusOK, "application/json")
	if _, found := GetCachedApiResponse(http.MethodGet, testURL); found {
		t.Fatalf("expected cache to be invalidated after PATCH request")
	}
}
