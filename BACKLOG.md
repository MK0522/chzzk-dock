# CHZZK OBS Dock - Product Backlog & Agile Tracker

## 📌 프로젝트 개요
OBS Studio에서 치지직(Chzzk) 방송 정보 및 라이브 설정을 손쉽게 제어하고 모니터링하기 위한 초경량·고성능 로컬 백엔드 & OBS 브라우저 독(Dock) 애플리케이션입니다.

---

## 🚀 Sprint 1 (v0.2.0 완료)
**목표**: 치지직 비공식 API 연동, 단일 포트(8081) 통합 독 구축, pywebview 네이버 로그인 팝업 연동, 세부설정/후원 3종 제어

### 📋 작업 목록 (Task List)
- [x] **[BACK-101]** Agile 백로그 문서(`BACKLOG.md`) 구축
- [x] **[BACK-102]** `pywebview` 기반 네이버 로그인 팝업 및 쿠키 자동 캡처 (`server.py`, `webview_login.py`)
- [x] **[BACK-103]** 네이버 쿠키 수동 입력 API 및 Fallback 시스템 지원
- [x] **[BACK-104]** 치지직 비공식 API 프록시 엔드포인트 구축 (연령제한, 클립, 다시보기, 후원3종)
- [x] **[FRONT-201]** 단일 포트(8081) 통합 독 위젯 UI 구축 (`chzzk-obs-dock.html`)

---

## 🛡️ Sprint 2 (v0.3.1 완료 - Security & Architecture Hardening)
**목표**: 보안 결함 전면 차단 (CSRF/DNS Rebinding), OS 레벨 보안 금고 위임 (Windows Credential Manager), 동시성/원자적 I/O, Rate Limiter 및 독 비동기 렌더링 결함 해결

### 📋 작업 목록 (Task List)
- [x] **[SEC-01]** Local CSRF 방어: `X-Requested-With: ChzzkDock` 커스텀 헤더 강제 검증 및 엄격한 CORS Origin 화이트리스트 적용
- [x] **[SEC-02]** Zero-File 세션 보안: `config.json` 디스크 파일 저장 폐지 ➡️ `advapi32.dll`을 통한 **Windows 자격 증명 관리자(Windows Credential Manager)** 전담 저장
- [x] **[SEC-03]** DNS Rebinding 방어: `Host` 헤더 검증 (`localhost`, `127.0.0.1`, `::1` Dual-Stack 지원)
- [x] **[LEG-01]** 비공식 API Rate Limiting: 3초 인메모리 캐시 도입으로 단시간 과도한 트래픽 유발 차단 및 면책 고지 배너 추가
- [x] **[ARCH-01]** 동시성 락 및 원자적(Atomic) 파일 I/O: `.tmp` ➡️ `os.replace` 교체 함수(`atomic_write_json`) 및 `threading.Lock()` 구축
- [x] **[FRONT-301]** 로그인 웹뷰 비동기 타이머 누수 및 무한 루프 버그 수정: `setInterval` ➡️ 단일 재귀 `setTimeout` + `isLoggingIn` 상태 가드
- [x] **[FRONT-302]** 상시 백그라운드 폴링 제거 및 사용자 선택형 60초 자동 동기화 토글 옵션화
- [x] **[FRONT-303]** 방송 설정 로드 성공 시 상단 헤더 연동 상태 즉시 동기화 (`[초록불] 연동됨`)

---

## ⚡ Sprint 3 (v0.3.1 Go Edition 완료 - Pure Go Single Binary)
**목표**: Python 의존성 전면 탈피, Zero-CGO 기반 고성능 단일 실행 파일(`chzzk-dock.exe`) 마이그레이션, `embed.FS` 에셋 번들링, 기존 보안 및 주석 100% 보존

### 📋 작업 목록 (Task List)
- [x] **[MIG-401]** Go 언어로의 백엔드 경량 고성능 마이그레이션 (`main.go`, `core/*.go` / Single Binary 배포)
- [x] **[MIG-402]** `advapi32.dll` Win32 API 1:1 바인딩 및 Zero-CGO Pure Win32 시스템 트레이 구현 (`core/credentials.go`, `core/tray.go`)
- [x] **[MIG-403]** Edge WebView2 네이버 로그인 연동 및 Named Mutex 단일 인스턴스 가드 (`core/webview.go`)
- [x] **[PKG-404]** `embed.FS`를 통한 HTML/아이콘 일체형 단일 바이너리(`chzzk-dock.exe`) 및 빌드 스크립트(`build.bat`) 구축
- [x] **[DOC-405]** Python 레거시 코드 아카이빙(`python_legacy/`) 및 문서(`README.md`, `BACKLOG.md`) 최신화

---

## 🎯 Sprint 4 (v0.4.2 - Antivirus False-Positive Zero & UX Polish)
**목표**: 백신 정적 머신러닝(ML) 휴리스틱 오탐 제로화, Win32 표준 API 전면 전환, 사용자 피드백 반영

### 📋 작업 목록 (Task List)
- [x] **[SYS-501]** **Zero-CMD / Zero-PowerShell 아키텍처 전면 전환 (전수 조사 완료: 총 2곳)**:
  - 1) `main.go` - `OpenURL`: `cmd /c start` 및 `powershell` 제거 ➡️ Win32 공식 `ShellExecuteW` API 바인딩 (백신 오탐 제거 & 즉시 실행)
  - 2) `core/tray.go` - `CopyDockUrl`: `cmd /c echo|set /p="..."|clip` 제거 ➡️ Win32 네이티브 `OpenClipboard` / `SetClipboardData` 바인딩 (따옴표 버그 수정 & 0ms 즉각 복사)
  - *(참고: 시작프로그램 등록 등 나머지 기능은 이미 Go registry API로 CMD 없이 100% 네이티브 구동 중)*
- [ ] **[BUILD-502]** **컴파일러 플래그 정상화 (대기 중)**: `-s -w`(심볼 스트립) 제거 (SYS-501 단독으로 오탐 해결 시 스킵 예정)
- [ ] **[CI-503]** **릴리즈 노트 템플릿 연동**: 배포 시 자동 릴리즈 노트 포맷팅 개선
- [x] **[UI-504]** **사용자 추가 편의성 & 보안 UI 개선 (v0.4.2 반영)**:
  - 1) **팝업 로그인 (자동)**: "방법 A. 팝업 로그인 (자동)" 및 버튼 문구 명시화
  - 2) **쿠키 가이드 & 새창 확대**:
    - 스크린샷 2종 압축 최적화 후 바이너리 직접 임베딩 (`/guide-image/1`, `/guide-image/2`)
    - 이미지 클릭 시 Win32 네이티브 브라우저 "새창"으로 고해상도 확대 기능 구현
  - 3) **쿠키 보안 강화 (Zero-DOM Leakage)**:
    - 로그인 연동 시 `/config` 응답 및 화면 입력창에 실제 쿠키 평문 대신 더미 마스킹(`••••••••••••••••••••••••••••••••`) 표시
    - DOM 검사(F12)나 화면 캡처 시 실제 쿠키 유출 원천 방지
  - 4) **후원 & 채팅 제어 UI 정리**:
    - 우측 상단 닫기(X) 버튼 옆 중복 불러오기(새로고침) 버튼 제거 (하단 단일화)

