package core

import (
	"fmt"
	"strings"
)

// ==============================================================================
// [CHZZK OBS DOCK - Error Code Management Architecture]
// - 규격: ERR-[카테고리]-[코드번호]
// - 사용자 화면: 이해하기 쉬운 한글 설명 + 친절한 조치 가이드
// - 개발자 진단 로그: 오류 코드 + 정밀 파라미터 + 시스템 오류 원문
// ==============================================================================

// Error Code Constants
const (
	// [NET: 100 ~ 199] 네트워크 & 포트 바인딩 도메인
	ErrNetDefaultPortCollision = "ERR-NET-101" // 기본 포트(8081) 점유 충돌 및 자동 Fallback
	ErrNetCustomPortCollision  = "ERR-NET-102" // 수동 지정 포트 충돌
	ErrNetInvalidPortRange     = "ERR-NET-103" // 유효하지 않은 포트 번호 범위 (1024 ~ 65535 외)
	ErrNetFallbackExhausted    = "ERR-NET-104" // 대체 안전 포트 탐색 실패
	ErrNetSecurityBlocked      = "ERR-NET-105" // CSRF 또는 Host 헤더 보안 정책 차단

	// [AUTH: 200 ~ 299] 계정 인증 & 보안 금고 도메인
	ErrAuthSessionExpired      = "ERR-AUTH-201" // 네이버 로그인 세션 쿠키 만료/미보유
	ErrAuthVaultAccessFailed   = "ERR-AUTH-202" // Windows 자격 증명 관리자 암호화 금고 접근 실패
	ErrAuthWebviewRuntimeError = "ERR-AUTH-203" // WebView2 런타임 미설치 또는 렌더러 장애
	ErrAuthDuplicateLogin      = "ERR-AUTH-204" // 로그인 팝업 중복 실행 방지 가드 작동

	// [OBS: 300 ~ 399] OBS 연동 & 와치독 도메인
	ErrObsPathNotFound       = "ERR-OBS-301" // OBS 설치 경로 또는 scripts 디렉터리 자동 탐색 실패
	ErrObsScriptWriteFailed  = "ERR-OBS-302" // Lua 스크립트 파일 생성/설치 실패 (권한 부족)
	ErrObsProcessCheckFailed = "ERR-OBS-303" // 와치독 OBS 프로세스 조회 실패

	// [API: 400 ~ 499] 치지직 스튜디오 API 통신 도메인
	ErrApiRateLimited    = "ERR-API-401" // 치지직 비공식 API 3초 레이트 리밋 차단
	ErrApiUnauthorized   = "ERR-API-402" // 치지직 인증 실패 (401 Unauthorized / 재로그인 필요)
	ErrApiPermissionWait = "ERR-API-403" // 방송 설정 변경 권한 없음 (403 Forbidden)
	ErrApiServerDown     = "ERR-API-500" // 치지직 스튜디오 공식 서버 장애 (5xx)

	// [SYS: 500 ~ 599] 시스템 & 파일 I/O 도메인
	ErrSysSettingsIoFailed = "ERR-SYS-501" // settings.json 파일 입출력 실패
	ErrSysTrayInitFailed   = "ERR-SYS-502" // Windows 시스템 트레이 생성 실패
)

// AppError: 사용자 친화적 메시지와 개발자용 정밀 로그를 이원화한 표준 에러 구조체
type AppError struct {
	Code       string // e.g. "ERR-NET-101"
	Title      string // 사용자 팝업/알림 제목
	UserMsg    string // 사용자용 친절하고 쉬운 한글 메시지
	DevDetails string // 개발자 디버깅용 기술 상세 (점유 프로세스, PID, 소켓 에러 등)
	Cause      error  // 기저 Go error 원문
}

func (e *AppError) Error() string {
	if e.DevDetails != "" {
		return fmt.Sprintf("[%s] %s (%s)", e.Code, e.UserMsg, e.DevDetails)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.UserMsg)
}

// UserFormat: 사용자 화면(팝업 대화상자 또는 웹 UI 토스트)에 보여줄 텍스트 포맷
func (e *AppError) UserFormat() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] %s\n\n", e.Code, e.Title))
	sb.WriteString(e.UserMsg)
	return sb.String()
}

// DevFormat: 개발자 진단 로그에 남길 정밀한 한 줄 포맷
func (e *AppError) DevFormat() string {
	causeStr := ""
	if e.Cause != nil {
		causeStr = fmt.Sprintf(" | Cause: %v", e.Cause)
	}
	if e.DevDetails != "" {
		return fmt.Sprintf("[%s] %s | Details: %s%s", e.Code, e.Title, e.DevDetails, causeStr)
	}
	return fmt.Sprintf("[%s] %s%s", e.Code, e.Title, causeStr)
}

// NewAppError: 새로운 AppError 인스턴스 생성
func NewAppError(code, title, userMsg, devDetails string, cause error) *AppError {
	return &AppError{
		Code:       code,
		Title:      title,
		UserMsg:    userMsg,
		DevDetails: devDetails,
		Cause:      cause,
	}
}
