# <img src="docs/icon.png" width="40" alt=""> CHZZK OBS Dock

OBS Studio 안에서 사용자 브라우저 독(`http://localhost:8081`)을 추가하면 치지직(CHZZK) 방송 정보와 설정을 편하게 수정할 수 있는 초경량·고성능 로컬 서버 및 독 위젯 애플리케이션입니다.

과거 트위치 시절처럼 스트리머들이 번거롭게 치지직 스튜디오 웹에 접속할 필요 없이 OBS 화면 안에서 방송의 모든 것을 즉시 제어하는 것을 목표로 합니다.

| Chzzk OBS Dock | Twitch Info Dock |
| :---: | :---: |
| <img src="docs/preview.png" width="380" alt="Chzzk OBS Dock"> | <img src="docs/twitch.jpg" width="380" alt="Twitch Info Dock"> |
> 현재 정식 배포 버전: `v0.5.9`

---

## ✨ 주요 기능

- 📝 **방송 정보 제어**: 방송 제목, 카테고리 검색, 방송 태그를 OBS 안에서 바로 변경
- ⚙️ **방송 세부 옵션**: 다시보기(확인 후/자동/안 함), 19금 연령 제한, 클립 생성, 해외 시청, 유료 프로모션 설정
- 💬 **채팅 제어 & 독립 채팅창**: 채팅 참여 대상(모두/팔로워/운영자) 및 저속·이모티콘 모드 제어, 항상 위 고정이 가능한 독립 채팅창 제공
- ⭐ **후원 실시간 관리**: 치즈·영상·미션 후원 원클릭 ON/OFF 및 구독 알림(볼륨/TTS) 세부 설정
- 🎬 **치지직 같이보기 연동**: 콘텐츠 선택 시 공식 규정에 맞춰 카테고리와 옵션 자동 고정
- 🤝 **파티 방송 관리**: 스튜디오 공식 파티 생성, 초대 링크로 원클릭 합류 및 참여자 관리
- 🎛️ **치지직 공식 리모컨 연동**: 로그인 없이 바로 열리는 공식 리모컨 미니 창 지원
- ⚡ **OBS 자동 연동**: OBS를 실행하면 독 서버가 자동으로 켜지고 OBS 종료 시 함께 자동 종료
- 🔐 **안전한 보안**: 로그인 세션을 디스크 파일이 아닌 Windows 보안 자격 증명에 안전하게 암호화 보관

---

## 🚀 빠른 시작 가이드

### 1. 권장 사항 (테스트된 환경)
- **OS**: Windows 11 (64-bit) (Windows 10 호환)
- **OBS Studio**: v28.0 이상 권장
- **WebView2 런타임**: Windows 10/11 기본 탑재

### 2. OBS 연동 및 자동 실행 (가장 추천 ⭐️)
평소에 서버를 수동으로 켤 필요 없이, **OBS를 켤 때 자동으로 서버가 켜지고 OBS를 끌 때 자동으로 꺼지도록** 설정할 수 있습니다:
1. `chzzk-dock.exe`를 실행하고 브라우저나 OBS 독에서 `http://localhost:8081`을 엽니다.
2. 상단 헤더 우측의 **[⚙️ 설정]** 클릭 ➡️ 관리 섹션의 **`Chzzk-Dock 자동시작`** 카드에서 **`[📦 스크립트 설치]`** 버튼을 클릭합니다.
3. OBS Studio 상단 메뉴 **[도구] ➡️ [스크립트] ➡️ [+] 버튼**을 누르고, 열리는 스크립트 폴더에서 `chzzk_dock_launcher.lua`를 선택하여 등록합니다.
4. 등록 완료! 이제 평소처럼 OBS만 켜고 끄시면 서버가 완전히 자동으로 함께 동작합니다.

### 3. OBS Studio 독(Dock) 등록
1. OBS Studio 실행 ➡️ 상단 메뉴 **[독(Docks)]** ➡️ **[사용자 지정 브라우저 독...]** 클릭
2. 독 이름: `치지직 방송 제어` (원하는 이름 입력)
3. URL: `http://localhost:8081` 입력 후 **[적용]** 클릭
4. OBS 원하는 위치에 드래그하여 도킹 완료!

### 4. 수동 단독 실행
- `chzzk-dock.exe`를 직접 실행하면 웹 브라우저(`http://localhost:8081`)를 통해 단독으로 사용할 수 있습니다.

---

## 📁 프로젝트 구조

```
chzzk-dock/
├── core/                         # 백엔드 핵심 비즈니스 로직 및 Win32 네이티브 바인딩
│   ├── chat_webview.go           # 실시간 채팅창 전용 WebView2 플로팅 윈도우
│   ├── credentials.go            # Windows Credential Manager(advapi32) 자격 증명 금고 관리
│   ├── dock_webview.go           # 메인 OBS 브라우저 독 팝업 모드 윈도우
│   ├── obs_detector.go           # OBS 프로세스 감지, 경로 탐색 및 Lua 스크립트 설치
│   ├── remote_webview.go         # 치지직 공식 리모컨 전용 WebView2 플로팅 윈도우 & DWM 테두리
│   ├── security.go               # Local CSRF / DNS Rebinding 방어 및 Rate Limiter
│   ├── settings.go               # GPU 가속, 외부 브라우저 핸드오프 등 사용자 설정 영속 관리
│   ├── tray.go                   # Win32 네이티브 시스템 트레이 아이콘 구현 (Zero-CGO)
│   ├── watchdog.go               # OBS 프로세스 수명주기 감시 및 안전 자동 종료
│   └── webview.go                # 네이버 로그인 Edge WebView2 팝업 인증 모듈
├── docs/                         # 프로젝트 가이드 문서 및 이미지 에셋
├── scripts/                      # OBS Studio 연동 스크립트
│   └── chzzk_dock_launcher.lua   # OBS 기동 시 chzzk-dock 자동 실행/종료 초경량 Lua 스크립트
├── app.manifest                  # Windows 고해상도 PerMonitorV2 DPI 애플리케이션 매니페스트
├── build.bat                     # 단일 바이너리 자동 빌드 스크립트 (PE 리소스 자동 생성)
├── chzzk-obs-dock.html           # OBS 브라우저 독 통합 UI 위젯 (바이너리 임베딩)
├── go.mod, go.sum                # Go 모듈 및 의존성 라이브러리 정의
├── icon.ico                      # Windows 실행 파일용 아이콘 리소스
├── installer.iss                 # Inno Setup 기반 Windows 설치 파일 패키징 스크립트
├── LICENSE                       # 오픈소스 라이선스
├── main.go                       # HTTP 프록시 서버 라우터, 엔드포인트 및 진입점
├── README.md                     # 프로젝트 안내 및 사용자 가이드
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

---

> 🤖 **안내**: 본 `README.md` 문서는 AI에 의해 작성 및 정리되었습니다.



