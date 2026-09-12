# Project Rules & Release Guidelines

## [CRITICAL] 포터블 ZIP(.zip) 패키징 및 언급 절대 금지
- 본 프로젝트는 단일 실행 파일(`chzzk-dock.exe`)과 설치 프로그램(`chzzk-dock-windows-amd64-installer.exe`)만 공식 배포 파일로 취급합니다.
- `chzzk-dock.exe` 자체가 무설치 단일 실행 바이너리(Pure Single Binary)이므로, 별도의 `.zip` 아카이빙이나 '포터블 zip', 'portable.zip'을 생성·패키징하거나 언급하는 행위를 일체 금지합니다.
- GitHub Actions 워크플로우(`release.yml`), 빌드 스크립트, 릴리즈 노트, 백로그, 커밋 메시지, 대화 응답 등 어디에서도 `.zip` 아카이빙을 수행하거나 "포터블.zip", "portable.zip"을 제안하거나 포함하지 마십시오.
- 공식 배포 에셋은 항상 다음 2종으로 엄격히 제한됩니다:
  1. `chzzk-dock-windows-amd64-installer.exe` (인스톨러 설치형)
  2. `chzzk-dock.exe` (단일 실행 파일)

## [CRITICAL] 실행 바이너리 명칭 규격 엄격 준수 (`chzzk-dock.exe`)
- 본 프로젝트의 실행 파일 출력명은 반드시 **`chzzk-dock.exe`**이어야 합니다.
- `go build` 실행 시 `-o chzzk-dock.exe` 옵션을 생략하면 모듈명(`chzzk-obs-dock`)에 따라 `chzzk-obs-dock.exe`가 자동 생성되어 중복 바이너리가 발생하므로, **출력 옵션 없는 단순 `go build` 호출을 절대 금지**합니다.
- 빌드/테스트 시 항상 `go build -trimpath -ldflags="-H windowsgui" -o chzzk-dock.exe .` 규격을 사용하거나 `build.bat`을 통해서만 빌드하십시오.
- `chzzk-obs-dock.exe` 등 규격 외 실행 파일이 생성되거나 레포지토리에 잔존하지 않도록 엄격히 관리하십시오.
