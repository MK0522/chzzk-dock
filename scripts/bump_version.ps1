param(
    [Parameter(Mandatory=$true)]
    [string]$NewVersion
)

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

# Strip leading 'v' or 'V' if present
$ver = $NewVersion.TrimStart('v', 'V').Trim()
if ($ver -notmatch '^(\d+)\.(\d+)\.(\d+)') {
    Write-Error "Version must start with Major.Minor.Patch (e.g. 0.5.6 or 0.6.4-beta.1)"
    exit 1
}
$major = [int]$Matches[1]
$minor = [int]$Matches[2]
$patch = [int]$Matches[3]
$build = 0
if ($ver -match '[-.](?:beta|rc|alpha|build)\.?(\d+)') {
    $build = [int]$Matches[1]
}

Write-Host "======================================================"
Write-Host "  Bumping CHZZK OBS Dock version to v$ver"
Write-Host "======================================================"

# 1. main.go
$mainPath = "main.go"
if (Test-Path $mainPath) {
    $c = Get-Content $mainPath -Raw -Encoding UTF8
    $c = $c -replace 'APP_VERSION\s*=\s*"v[^"]+"', "APP_VERSION       = `"v$ver`""
    $c = $c -replace 'CHZZK OBS Dock Server v[^\s]+', "CHZZK OBS Dock Server v$ver"
    Set-Content $mainPath -Value $c -Encoding UTF8
    Write-Host "[OK] Updated $mainPath"
}

# 2. versioninfo.json
$viPath = "versioninfo.json"
if (Test-Path $viPath) {
    $vi = Get-Content $viPath -Raw -Encoding UTF8 | ConvertFrom-Json
    $vi.FixedFileInfo.FileVersion.Major = $major
    $vi.FixedFileInfo.FileVersion.Minor = $minor
    $vi.FixedFileInfo.FileVersion.Patch = $patch
    $vi.FixedFileInfo.FileVersion.Build = $build
    $vi.FixedFileInfo.ProductVersion.Major = $major
    $vi.FixedFileInfo.ProductVersion.Minor = $minor
    $vi.FixedFileInfo.ProductVersion.Patch = $patch
    $vi.FixedFileInfo.ProductVersion.Build = $build
    $vi.StringFileInfo.FileVersion = "$major.$minor.$patch.$build"
    $vi.StringFileInfo.ProductVersion = "$ver"
    $newJson = $vi | ConvertTo-Json -Depth 10
    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText((Resolve-Path $viPath).Path, $newJson, $utf8NoBom)
    Write-Host "[OK] Updated $viPath"
}

# 3. installer.iss
$issPath = "installer.iss"
if (Test-Path $issPath) {
    $c = Get-Content $issPath -Raw -Encoding UTF8
    $c = $c -replace '#define MyAppVersion "[^"]+"', "#define MyAppVersion `"$ver`""
    Set-Content $issPath -Value $c -Encoding UTF8
    Write-Host "[OK] Updated $issPath"
}

# 4. README.md
$readmePath = "README.md"
if (Test-Path $readmePath) {
    $c = Get-Content $readmePath -Raw -Encoding UTF8
    $c = $c -replace '> 현재 개발 버전: `v[^`]+`', "> 현재 개발 버전: ``v$ver``"
    Set-Content $readmePath -Value $c -Encoding UTF8
    Write-Host "[OK] Updated $readmePath"
}

# 5. Regenerate PE Resource
Write-Host "[*] Regenerating PE Resource (resource_windows_amd64.syso)..."
go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest -64 -o resource_windows_amd64.syso
if ($LASTEXITCODE -eq 0) {
    Write-Host "[OK] PE Resource generated successfully."
} else {
    Write-Error "Failed to generate PE Resource."
    exit 1
}

Write-Host "======================================================"
Write-Host "  Successfully bumped version to v$ver!"
Write-Host "======================================================"
