# <img src="docs/icon.png" width="40" alt=""> CHZZK OBS Dock

OBS Studio 안에서 사용자 브라우저 독(`http://localhost:8081`)을 추가하면 치지직(CHZZK) 방송 정보와 설정을 편하게 제어할 수 있는 초경량·고성능 로컬 서버 및 독 위젯 애플리케이션입니다.

과거 트위치 시절처럼 스트리머들이 번거롭게 치지직 스튜디오 웹에 접속할 필요 없이 OBS 화면 안에서 방송의 모든 것을 즉시 제어하는 것을 목표로 합니다.

| Chzzk OBS Dock | Twitch Info Dock |
| :---: | :---: |
| <img src="docs/preview.png" width="380" alt="Chzzk OBS Dock"> | <img src="docs/twitch.jpg" width="380" alt="Twitch Info Dock"> |
> `v0.6.1` 기준

---

## ✨ 주요 기능

- 👥 **다중 계정 지원**: 멀티 프로필 전환 가능
- 📝 **방송 정보 제어**
  - 방송 제목
  - 카테고리
  - 태그
  - 같이보기
  - 드롭스
  - 썸네일
  - 세부 옵션(다시보기, 19금 연령 제한, 클립 생성, 해외 시청, 유료 프로모션)
- 🛡️ **채팅 모드 제어**
  - 이모티콘 전용
  - 슬로우 모드(초 단위)
- 🪙 **후원 관리**
- 📱 **치지직 공식 리모컨**
- 📊 **컴팩트한 통계 독**
- 💬 **플로팅 채팅창**

---

## 🚀 빠른 시작 가이드

### 1. 권장 사양 (테스트된 환경)
- **OS**: Windows 11 (64-bit) (Windows 10 호환)
- **OBS Studio**: v28.0 이상 권장
- **WebView2 런타임**: Windows 10/11 기본 탑재

### 2. 치지직 계정 연동 및 다중 계정 관리
1. `chzzk-dock.exe`를 실행하고 브라우저나 OBS 독에서 `http://localhost:8081`을 엽니다.
2. 독 화면 우측 상단의 **[⚙️ 설정] ➡️ [로그인] 탭**으로 이동합니다.
3. **방법 A (팝업 로그인)**: [네이버 로그인] 버튼을 눌러 공식 웹뷰 창에서 로그인하면 세션 쿠키가 Windows 자격 증명 관리자에 안전하게 암호화 보관됩니다.
4. **방법 B (쿠키 직접 입력)**: 브라우저 개발자 도구(F12) 등에서 추출한 `NID_AUT`, `NID_SES` 쿠키 값을 직접 붙여넣어 연동할 수 있습니다.
5. **다중 계정 전환**: 상단 프로필 영역의 채널명을 클릭하면 등록된 계정 목록 팝오버가 열리며, 다른 계정으로 즉시 전환하거나 신규 계정을 추가 등록할 수 있습니다.

### 3. OBS 연동 및 앱 자동 시작 (가장 추천 ⭐️)
평소에 서버를 수동으로 켤 필요 없이, **OBS를 켤 때 자동으로 서버가 켜지고 OBS를 끌 때 자동으로 꺼지도록** 설정할 수 있습니다:
1. 설정창의 **[일반] 탭** ➡️ **`앱 자동 시작`** 카드에서 **`[📦 스크립트 설치]`** 버튼을 클릭합니다.
2. OBS Studio 상단 메뉴 **[도구] ➡️ [스크립트] ➡️ [+] 버튼**을 누르고, 열리는 스크립트 폴더에서 `chzzk_dock_launcher.lua`를 선택하여 등록합니다.
3. **`OBS와 함께 종료`** 옵션에서 **"즉시 종료"** 또는 원하는 대기 시간(초)을 선택합니다.
4. 등록 완료! 이제 평소처럼 OBS만 켜고 끄시면 서버가 완전히 자동으로 함께 동작합니다.

### 4. OBS Studio 독(Dock) 등록
1. OBS Studio 실행 ➡️ 상단 메뉴 **[독(Docks)] ➡️ [사용자 지정 브라우저 독...]** 클릭
2. **방송 제어 독**: 독 이름: `치지직 방송 제어`, URL: `http://localhost:8081` 입력 후 **[적용]** 클릭
3. **방송 통계 독 (선택)**: 독 이름: `OBS 방송 통계`, URL: `http://localhost:8081/obs-stats-dock` 입력 후 **[적용]** 클릭
4. OBS 원하는 위치에 드래그하여 도킹 완료!

### 5. 수동 단독 실행 및 고급 제어
- `chzzk-dock.exe`를 직접 실행하면 웹 브라우저(`http://localhost:8081`)를 통해 단독으로 사용할 수 있습니다.
- 작업 표시줄 알림 영역(트레이)의 치지직 독 아이콘을 우클릭하여 독 창 열기, 로그 확인, 자동 종료 시간 변경, 서버 종료 등을 즉시 제어할 수 있습니다.
- **[고급] 설정 탭**에서 기본 HTTP 서버 포트 변경(원클릭 즉시 재시작 지원), GPU 하드웨어 가속 토글, 외부 링크 클릭 시 기본 웹 브라우저 위임(감지된 기본 브라우저 색상 뱃지 표시)을 설정할 수 있습니다.

---

## 📁 프로젝트 구조
```
구조 리펙토링 진행중
```

```text
chzzk-dock/
├── core/                         # 백엔드 핵심 비즈니스 로직 및 Win32 네이티브 바인딩
│   ├── auth_webview.go           # 네이버 로그인 Edge WebView2 팝업 인증 및 외부 링크 가드
│   ├── credentials.go            # Windows Credential Manager(advapi32) 다중 계정 암호화 금고 관리
│   ├── dock_manager.go           # 방송 프리셋, 카테고리 즐겨찾기, 앱 설정 영속화 및 비동기 업데이터
│   ├── errors.go                 # 도메인별 표준 에러 코드 규격 정의
│   ├── logger.go                 # 순환 링 버퍼 기반 초경량 시스템 이벤트 로깅
│   ├── process.go                # OBS 프로세스 감지, 생명주기 감시(Watchdog) 및 Lua 스크립트 설치
│   ├── sysinfo.go                # WMI 없는 초고속 Win32 하드웨어 사양 수집 및 진단 리포트 생성기
│   ├── tray.go                   # Win32 네이티브 시스템 트레이 아이콘 구현 (Zero-CGO)
│   └── user_webview.go           # 메인 독 팝업, 플로팅 채팅창, 공식 리모컨 통합 윈도우 엔진
├── docs/                         # 프로젝트 안내 문서 및 에셋
│   ├── cookie_guide_1.png, 2.png # 로그인 및 쿠키 수동 연동 가이드 이미지
│   ├── icon.png, preview.png     # 독 UI 프리뷰 및 공식 로고 에셋
│   ├── licenses/                 # 써드파티 오픈소스 라이선스 전문
│   ├── README.md                 # 프로젝트 안내 및 사용자 가이드
│   ├── RELEASE_NOTES.md          # 버전별 릴리즈 노트
│   └── twitch.jpg                # 트위치 독 비교 이미지
├── scripts/                      # OBS Studio 연동 스크립트
│   └── chzzk_dock_launcher.lua   # OBS 기동 시 chzzk-dock 자동 실행/종료 초경량 Lua 스크립트
├── ui/                           # OBS 브라우저 독 웹 프론트엔드 리소스
│   ├── chzzk-obs-dock.html       # 메인 방송 제어 및 설정 일체형 통합 독 UI (바이너리 임베딩)
│   └── obs-stats-dock.html       # OBS 실시간 방송/녹화 텔레메트리 통계 독 UI
├── app.manifest                  # Windows 고해상도 PerMonitorV2 DPI 애플리케이션 매니페스트
├── build.bat                     # 단일 바이너리 자동 빌드 스크립트 (PE 리소스 자동 생성)
├── go.mod, go.sum                # Go 모듈 및 의존성 라이브러리 정의
├── icon.ico                      # Windows 실행 파일용 아이콘 리소스
├── installer.iss                 # Inno Setup 기반 Windows 설치 파일 패키징 스크립트
├── LICENSE                       # 오픈소스 라이선스 (MPL-2.0)
├── main.go                       # 서버 부트스트랩, 정적 에셋 번들링 및 단일 인스턴스 Mutex 제어
├── routes.go                     # HTTP 엔드포인트 핸들러 라우팅 및 3초 쿨다운 인메모리 캐시
└── versioninfo.json              # PE 리소스 메타데이터 정의 (한국어 UTF-16)
```

---

## 🛠️ 개발자 빌드 가이드

Go 1.22 이상 (권장: Go 1.27 이상) 환경에서 `build.bat`을 실행하거나 아래 명령어로 직접 빌드합니다:

```powershell
# 1. PE 리소스 생성 (아이콘, 매니페스트, 메타데이터 번들링)
go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest -64 -o resource_windows_amd64.syso

# 2. 단일 바이너리 빌드 (콘솔 창 숨김, 심볼 제거 및 로컬 경로 제거)
go build -trimpath -ldflags="-H windowsgui -s -w" -o chzzk-dock.exe .
```

---

## ⚠️ 면책 조항 (Disclaimer)

본 소프트웨어는 네이버(치지직)의 공식 배포 서비스가 아니며, 개인 방송 환경의 편의를 위해 제작된 독립 오픈소스 도구입니다. 본 프로그램 사용으로 인해 발생하는 모든 책임은 사용자 본인에게 있습니다.

```text
쿠키 정보(NID_AUT, NID_SES)는 평문 파일에 저장되지 않으며, Windows 자격 증명 관리자에 암호화 보관되어 필요시 치지직 API 통신에만 사용됩니다.
```

---

## 📜 라이선스 (License)

- **CHZZK OBS Dock**: 본 프로젝트는 [Mozilla Public License 2.0 (MPL-2.0)](../LICENSE) 라이선스 하에 배포됩니다.
- **오픈소스 크레딧 (Third-party Licenses)**: 본 프로젝트에서 사용하는 써드파티 오픈소스 목록입니다. 각 라이선스 전문은 링크된 문서를 통해 확인하실 수 있습니다.
  - [wailsapp/go-webview2](https://github.com/wailsapp/go-webview2) — [MIT License](licenses/MIT.md) (Copyright © 2021 Lea Anthony)
  - [golang.org/x/sys](https://github.com/golang/sys) — [BSD-3-Clause License](licenses/BSD-3-Clause.md) (Copyright © 2009 The Go Authors)
  - [jchv/go-winloader](https://github.com/jchv/go-winloader) — [MIT License](licenses/MIT.md) (Copyright © 2021 John Chadwick)
  - [Tailwind CSS](https://tailwindcss.com/) — [MIT License](licenses/MIT.md) (Copyright © Tailwind Labs, Inc.)
  - [Lucide Icons](https://lucide.dev/) — [ISC License](licenses/ISC.md) (Copyright © Lucide Contributors 2022)

---

> 🤖 **안내**: 본 프로젝트와 `README.md` 문서는 AI를 활용해 작성되었습니다.
