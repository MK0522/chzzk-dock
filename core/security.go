package core

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type CachedResponse struct {
	Timestamp   time.Time
	Body        []byte
	Status      int
	ContentType string
}

var (
	apiCache        = make(map[string]CachedResponse)
	apiCacheMu      sync.RWMutex
	CacheTTL        = 3 * time.Second
	MaxCacheEntries = 100
)

// CheckSecurity: [SEC-03] DNS Rebinding 방어: Host 헤더 검증 (127.0.0.1, localhost, ::1 허용)
func CheckSecurity(w http.ResponseWriter, r *http.Request) bool {
	hostHeader := r.Host
	hostName := hostHeader
	if h, _, err := net.SplitHostPort(hostHeader); err == nil {
		hostName = h
	}
	hostName = strings.ToLower(strings.TrimSpace(strings.Trim(hostName, "[]")))
	if hostName != "" && hostName != "localhost" && hostName != "127.0.0.1" && hostName != "::1" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    403,
			"message": "Forbidden Host (DNS Rebinding Protected)",
		})
		return false
	}
	return true
}

// CheckApiAuth: [SEC-01] Local CSRF 방어: 보안 검증 및 X-Requested-With 커스텀 헤더 검사
func CheckApiAuth(w http.ResponseWriter, r *http.Request) bool {
	if !CheckSecurity(w, r) {
		return false
	}
	reqWith := r.Header.Get("X-Requested-With")
	if reqWith != "ChzzkDock" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    403,
			"message": "Forbidden (Missing or invalid X-Requested-With security header)",
		})
		return false
	}
	return true
}

// GetCachedApiResponse: [LEG-01] GET 요청 인메모리 캐시 조회
func GetCachedApiResponse(method, url string) (*CachedResponse, bool) {
	if method != http.MethodGet {
		return nil, false
	}
	cacheKey := method + ":" + url

	apiCacheMu.RLock()
	item, found := apiCache[cacheKey]
	apiCacheMu.RUnlock()

	if found && time.Since(item.Timestamp) < CacheTTL {
		return &item, true
	}
	return nil, false
}

// SetCachedApiResponse: [LEG-01] GET 응답 캐시 저장 또는 상태 변경 시 캐시 무효화 (크기 제한 포함)
func SetCachedApiResponse(method, url string, respBytes []byte, status int, contentType string) {
	cacheKey := method + ":" + url

	if method == http.MethodGet && status == http.StatusOK {
		apiCacheMu.Lock()
		defer apiCacheMu.Unlock()

		now := time.Now()

		// 만료된 항목 정리
		for k, v := range apiCache {
			if now.Sub(v.Timestamp) >= CacheTTL {
				delete(apiCache, k)
			}
		}

		// 최대 엔트리 초과 시 가장 오래된 항목 제거
		if len(apiCache) >= MaxCacheEntries {
			var oldestKey string
			var oldestTime time.Time
			first := true
			for k, v := range apiCache {
				if first || v.Timestamp.Before(oldestTime) {
					oldestKey = k
					oldestTime = v.Timestamp
					first = false
				}
			}
			if oldestKey != "" {
				delete(apiCache, oldestKey)
			}
		}

		apiCache[cacheKey] = CachedResponse{
			Timestamp:   now,
			Body:        respBytes,
			Status:      status,
			ContentType: contentType,
		}
	} else if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete {
		apiCacheMu.Lock()
		clear(apiCache)
		apiCacheMu.Unlock()
	}
}
