# 🎮 CHZZK OBS Dock (v0.4.1 - Go Single Binary)

OBS Studio에서 **네이버 치지직(CHZZK) 방송 정보(제목, 카테고리, 태그, 연령제한, 다시보기, 클립)와 채팅/후원 설정**을 실시간으로 손쉽게 제어할 수 있는 초경량·고성능 OBS 전용 브라우저 독(Dock) 위젯 & 로컬 서버 단일 바이너리 애플리케이션입니다.

---

## ✨ 주요 기능

- ⚡ **원클릭 방송 설정 동기화**: 방송 제목, 카테고리 실시간 검색, 태그(최대 10개), 연령 제한, 클립 허용, 다시보기 게시 방식을 OBS 내부에서 한 번에 제어
- ⚡ **실시간 즉시 제어 영역 (강조 테두리)**: 치즈/영상/미션 후원 3종 및 **이모티콘 모드**, **저속 모드(3초, 5초, 10초, 30초, 1분, 2분, 5분)** 클릭 즉시 실시간 반영
- 💬 **채팅 이용 권한**: 모두 / 팔로워(최소 팔로우 시간, 구독자 예외 설정) / 운영자 맞춤 제한
- 🚀 **Go 단일 바이너리 (Single Binary)**: Python/C 컴파일러 의존성 없이 `chzzk-dock.exe` 하나로 모든 기능(서버, HTML/아이콘 임베딩, 로그인 팝업, 트레이) 완결
- 🍪 **간편한 네이버 로그인**: Microsoft Edge WebView2 기반 팝업을 통한 자동 쿠키 캡처 및 수동 입력 지원
- 🔐 **OS 커널 레벨 보안 (Zero-File)**: 세션 쿠키를 디스크 파일(`config.json`)에 남기지 않고 **Windows 자격 증명 관리자(Windows Credential Manager)** 시스템 금고에 직접 보관
- 🛡️ **엔터프라이즈 보안 방어**: Local CSRF 방어(`X-Requested-With` 헤더 강제), DNS Rebinding 방어, 엄격한 CORS 화이트리스트
- ⏱️ **지능형 Rate Limiting**: 비공식 스튜디오 API 과호출 방지를 위한 3초 인메모리 캐시 적용
- 🪶 **Zero-CGO Pure Win32 시스템 트레이**: 외부 무거운 라이브러리 없이 순수 Win32 API로 시스템 트레이, OBS 연동 스크립트 원클릭 추출 지원

---

## 🚀 빠른 시작 가이드

### 1. 요구 사항
- **OS**: Windows 10 / 11 (64-bit)

### 2. OBS 연동 및 자동 실행 (초간편 ⭐️)
평소에 서버를 켜둘 필요 없이, **OBS를 켤 때 자동으로 켜지고 OBS를 끌 때 자동으로 꺼지도록** 설정할 수 있습니다:
1. `chzzk-dock.exe` 실행 ➡️ OBS 브라우저 독(`http://localhost:8081`)을 엽니다.
2. 독 상단 설정(⚙️) ➡️ **[⚡ OBS에 스크립트 자동 추가하기]** 클릭 (관리자 권한 창이 뜨면 **[예]** 클릭)
3. OBS 상단 메뉴 **[도구] ➡️ [스크립트] ➡️ [+] 버튼** 클릭 ➡️ 기본으로 열리는 폴더에서 `chzzk_dock_launcher.lua` 선택!
4. 끝! 이제 평소처럼 OBS만 켜고 끄시면 서버가 완전히 자동으로 함께 동작합니다.

### 3. 수동 서버 실행 (단독 실행)
- `chzzk-dock.exe`를 더블 클릭하여 실행합니다. (OBS가 실행되지 않은 상태에서는 3분 동안 OBS 실행을 대기하며, OBS가 종료되면 3초 후 자동 종료됩니다.)
- 와치독 자동 종료 없이 계속 켜두려면 `--no-watchdog` 옵션으로 실행하세요.

### 4. OBS Studio 독(Dock) 등록
1. OBS Studio 실행 ➡️ 상단 메뉴 **[독(Docks)]** ➡️ **[사용자 지정 브라우저 독...]** 클릭
2. 독 이름: `치지직 방송 제어` (원하는 이름 입력)
3. URL: `http://localhost:8081` 입력 후 **[적용]** 클릭
4. OBS 원하는 위치에 드래그하여 도킹 완료!

### 5. 소스코드 직접 빌드 (개발자용)
- Go 1.22 이상이 설치된 환경에서 `build.bat`을 실행합니다:
  - `chzzk-dock.exe`: `-trimpath` 적용 및 백신 오탐 방지를 위한 디버그 심볼 유지 빌드

---

## 📁 프로젝트 구조

```
dazzling-brahmagupta/
├── core/
│   ├── credentials.go    # Windows Credential Manager(advapi32) 자격 증명 관리
│   ├── security.go       # Local CSRF / DNS Rebinding 방어 및 Rate Limiter
│   ├── tray.go           # Win32 순수 시스템 트레이 아이콘 구현 (Zero-CGO)
│   └── webview.go        # 네이버 로그인 팝업 웹뷰 모듈
├── main.go               # 경량 HTTP 서버 및 치지직 API 프록시 라우터 & 진입점
├── chzzk_dock_launcher.lua # OBS Studio 자동 실행/종료 초경량 Lua 스크립트
├── chzzk-obs-dock.html   # OBS 브라우저 독 위젯 UI (embed.FS 내장)
├── go.mod                # Go 모듈 의존성 정의
├── build.bat             # Go 단일 바이너리 빌드 스크립트 (아이콘 리소스 자동 포함)
├── start.bat             # 원클릭 실행 배치 스크립트
├── icon.ico / icon.png   # 앱 리소스 아이콘
├── app.manifest          # Windows 애플리케이션 매니페스트 (고해상도 DPI)
├── versioninfo.json      # 바이너리 버전 및 아이콘 리소스 정의
├── README.md             # 프로젝트 안내 문서
├── BACKLOG.md            # 애자일 스프린트 트래커
└── python_legacy/        # 이전 Python 레거시 소스코드 백업
```

---

## ⚠️ 면책 조항 (Disclaimer)

본 소프트웨어는 네이버(치지직) 공식 서비스가 아니며, 비공식 스튜디오 API를 연동하여 개인 방송 편의를 돕는 오픈소스 도구입니다. 본 프로그램 사용으로 인해 발생하는 모든 책임은 사용자 본인에게 있습니다.
