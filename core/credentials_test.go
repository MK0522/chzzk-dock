package core

import (
	"os"
	"testing"
)

func TestCredentialsLifecycle(t *testing.T) {
	// 테스트 전용 격리된 Credential Target 설정
	testTarget := "ChzzkObsDockTest/UnitTest"
	os.Setenv("CHZZK_CRED_TARGET", testTarget)
	defer func() {
		ClearConfig()
		os.Unsetenv("CHZZK_CRED_TARGET")
	}()

	// 1. 초기 상태 확인
	ClearConfig()
	cfg := LoadConfig()
	if cfg.NidAut != "" || cfg.NidSes != "" {
		t.Fatalf("expected empty config, got %+v", cfg)
	}

	// 2. 저장 테스트
	saveData := map[string]interface{}{
		"nid_aut": "test_aut_token_12345",
		"nid_ses": "test_ses_token_67890",
	}
	saved := SaveConfig(saveData)
	if saved.NidAut != "test_aut_token_12345" || saved.NidSes != "test_ses_token_67890" {
		t.Fatalf("unexpected save result: %+v", saved)
	}

	// 3. 다시 로드하여 확인 (Windows Credential Manager 영속성 검증)
	cachedConfigMu.Lock()
	cachedConfig = nil // 캐시 강제 무효화 후 OS 금고에서 직접 로드 테스트
	cachedConfigMu.Unlock()

	loaded := LoadConfig()
	if loaded.NidAut != "test_aut_token_12345" || loaded.NidSes != "test_ses_token_67890" {
		t.Fatalf("unexpected load result from OS vault: %+v", loaded)
	}

	// 4. 삭제(로그아웃) 테스트
	if !ClearConfig() {
		t.Fatalf("failed to clear config")
	}

	cachedConfigMu.Lock()
	cachedConfig = nil
	cachedConfigMu.Unlock()

	cleared := LoadConfig()
	if cleared.NidAut != "" || cleared.NidSes != "" {
		t.Fatalf("expected cleared config, got %+v", cleared)
	}
}
