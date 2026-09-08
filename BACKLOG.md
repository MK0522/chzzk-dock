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
- [x] **[BUILD-502]** **컴파일러 플래그 정상화 및 백신 오탐 방어**: `-trimpath` 적용 및 디버그 심볼 유지 빌드 구축
- [x] **[CI-503]** **릴리즈 파이프라인 정비**: Inno Setup 기반 Windows 설치 파일(`installer.iss`) 및 포터블 zip 동시 패키징 구축
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
- [x] **[OBS-505]** **OBS 연동 자동화 & UX 간소화 (v0.4.3 반영)**:
  - 1) **단일 토글 스위치 개편**: `Chzzk-Dock 자동시작` ON/OFF 토글 및 `자동 감지됨` 뱃지 적용
  - 2) **원클릭 설치/제거 연동**: 토글 ON 시 스크립트 자동 생성 및 실제 실행 파일 절대 경로(`BAKED_EXE_PATH`) 자동 주입, OFF 시 자동 삭제
  - 3) **Topmost 폴더 브라우저**: OBS 폴더 직접 선택 시 OBS 창 뒤로 숨지 않도록 `HWND_TOPMOST` 강제 활성화
  - 4) **자동 실행 실패 시 Win32 에러 팝업**: LuaJIT FFI 기반 `MessageBoxA`로 실행 파일 누락 시 0ms 즉시 알림

---

## 🎯 Sprint 5 (v0.5.0 완료 - Remote Control, Party Management & Project Polish)
**목표**: 치지직 공식 리모컨 WebView2 독립창 이식, 스튜디오 규격 파티 관리 & 초대 링크 원클릭 합류, 구독 설정 연동 및 파일트리 정돈

### 📋 작업 목록 (Task List)
- [x] **[REMOTE-601]** **치지직 리모컨 (Remote Control) 전면 이식**:
  - 1) **무로그인 독립 리모컨 미니 창 (WebView2)**: Windows Credential Manager 쿠키 자동 주입, DWM 다크 테두리 일체화, 상단 커스텀 툴바(📌 항상 위 고정 토글, 새로고침), 창 위치/크기/Topmost 상태 영속성 지원, Named Mutex 단일 인스턴스 가드
  - 2) **미디어 단축 컨트롤러**: OBS 독 내부 영상 재생/정지, 이전 영상 다시 재생, -10s, +10s, 다음 영상 제어 버튼 라벨 단축
- [x] **[PARTY-602]** **치지직 스튜디오 공식 규격 파티 관리 & 초대 링크 원클릭 합류**:
  - 1) **스튜디오 규격 파티 관리**: 파티 생성/이름 변경, 실시간 글자수 카운터, 한글 IME 조합 깨짐 방지 보호, 누적 치즈액 및 참여자 현황
  - 2) **초대 링크 원클릭 합류**: 초대 URL 전체/토큰 지능형 파서(`extractPartyInviteToken`), 요약 확인 및 수락 API 연동
  - 3) **역할 자동 분기 & 권한 제어**: `👑 방장` vs `🤝 게스트` 자동 판별, 게스트 전용 UI(`파티 나가기`, `다른 파티 합류`) 및 파티 전환 보호
- [x] **[DONAT-603]** **후원 세부 설정 이전 & 구독(Subscription)/팔로우 전면 지원**:
  - 1) **진입점 이전**: 리모컨 헤더에서 [후원/채팅 제어] 패널 내부로 `[⚙️ 세부 설정]` 버튼 올바르게 이전
  - 2) **구독/팔로우 피드 및 테스트**: 실시간 6종 필터, `[⭐ 구독]` 빠른 테스트 알림 버튼, 구독 알림/볼륨/TTS 세부 설정 연동
- [x] **[AV-604]** **백신 머신러닝 오탐 완화 (Anti-Virus False-Positive Mitigation)**:
  - 1) **PE 리소스 메타데이터 정상화**: `versioninfo.json`에 `CompanyName`, `Comments`, `VarFileInfo`(한국어 0412 / UTF-16) 명시로 헤더 이상 징후 제거
  - 2) **UAC 권한 상승 옵션 완화**: `ShellExecuteW` 호출 시 `SW_HIDE (0)` ➔ `SW_SHOWNORMAL (1)` 변경으로 악성 드롭퍼 휴리스틱 회피
- [x] **[TREE-605]** **프로젝트 파일트리 정리 & 문서 전면 복원**:
  - 1) **`scripts/` 디렉터리 분리**: `chzzk_dock_launcher.lua` 전용 디렉터리 이관 및 임베딩 헬퍼 함수 구현
  - 2) **임시 파일 정리**: 대용량 `index.js`, 임시 백업 `*.exe~` 제거
  - 3) **`README.md` 전면 복원**: 트위치 비교 테이블 및 이미지 에셋(`docs/preview.png`, `docs/twitch.jpg`) 온전 복원
- [x] **[REL-606]** **v0.5.0 릴리즈 준비 완료**: 소스코드, UI, 리소스, 문서 버전 전수 동기화 및 듀얼 테스트 패스

---

## 🎯 Sprint 6 (예정 - Lifecycle Notification & Installer Polish)
- [ ] **[NOTI-701]** **OBS 감지 실패/자동 종료 시 Windows 알림 연동**:
  - 1) **종료 알림 발생**: OBS가 켜져 있지 않거나 감지되지 않아 백엔드 서버가 자폭/자동 종료될 때 Windows 알림창(또는 시스템 토스트) 표시
  - 2) **UI 설정 토글 지원**: Chzzk-Dock 설정 패널에 `종료 알림 켜기/끄기` 토글 스위치를 배치하여 사용자가 알림 활성화/비활성화를 자유롭게 제어할 수 있도록 구현
