package core

// ==============================================================================
// [CHZZK OBS DOCK - Error Code Constants]
// - 규격: [도메인]-[코드번호 3자리]
// - BEN (백엔드 서버), AUTH (계정 인증), API (치지직 통신), SYS (시스템 & 파일)
// ==============================================================================

const (
	// [BEN: 100 ~ 199] 백엔드 엔진 & 포트 바인딩
	ErrBenDefaultPortCollision = "BEN-101" // 기본 포트(8081) 충돌로 대체 포트 자동 전환
	ErrBenCustomPortCollision  = "BEN-102" // 수동 지정 포트 충돌
	ErrBenInvalidPortRange     = "BEN-103" // 유효하지 않은 포트 번호 범위 (1024 ~ 65535 외)
	ErrBenPortScanFailed       = "BEN-104" // 대체 포트 탐색 실패

	// [AUTH: 200 ~ 299] 계정 & 인증
	ErrAuthSessionExpired    = "AUTH-201" // 네이버 로그인 세션 만료 또는 미로그인
	ErrAuthVaultAccessFailed = "AUTH-202" // Windows 자격 증명 금고 접근 실패
	ErrAuthDuplicateLogin    = "AUTH-203" // 로그인 창 중복 실행 시도

	// [API: 300 ~ 399] 치지직 API 통신 과정
	ErrApiRateLimited    = "API-301" // 3초 레이트 리밋 차단 (자동 캐시 응답)
	ErrApiUnauthorized   = "API-302" // 치지직 인증 실패 (HTTP 401 Unauthorized)
	ErrApiPermissionWait = "API-303" // 방송 설정 변경 권한 없음 (HTTP 403 Forbidden)
	ErrApiServerDown     = "API-304" // 치지직 공식 서버 장애 (HTTP 5xx)
	ErrApiOtherError     = "API-305" // 기타 치지직 API 응답 오류 (지정 외 HTTP 상태 코드)

	// [SYS: 400 ~ 499] 시스템 환경 & 파일 I/O
	ErrSysWebviewRuntime  = "SYS-401" // WebView2 런타임 미설치 또는 실행 장애
	ErrSysObsPathNotFound = "SYS-402" // OBS 설치 경로 또는 scripts 폴더 탐색 실패
	ErrSysFileIoFailed    = "SYS-403" // 파일 읽기/쓰기 실패 (Lua 스크립트, settings.json, 로그 저장 등)
)
