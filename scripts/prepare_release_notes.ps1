param(
    [string]$NotesPath = "docs/RELEASE_NOTES.md"
)

# 폴백 가드: docs/ 또는 루트 중 존재하는 경로 자동 감지
if (-not (Test-Path $NotesPath)) {
    if (Test-Path "docs/RELEASE_NOTES.md") {
        $NotesPath = "docs/RELEASE_NOTES.md"
    } elseif (Test-Path "RELEASE_NOTES.md") {
        $NotesPath = "RELEASE_NOTES.md"
    }
}

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

if (Test-Path $NotesPath) {
    $raw = Get-Content $NotesPath -Raw -Encoding UTF8
    $lines = $raw -split "`r?`n"
    $title = $lines[0].Trim()
    if ($title.StartsWith("# ")) {
        $title = $title.Substring(2).Trim()
    }
    $bodyLines = if ($lines.Length -gt 1) { $lines[1..($lines.Length - 1)] } else { @() }
    $body = ($bodyLines -join "`n").Trim()

    Set-Content -Path "release_body.md" -Value $body -Encoding UTF8

    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    if ($env:GITHUB_OUTPUT) {
        [System.IO.File]::AppendAllText($env:GITHUB_OUTPUT, "has_custom_notes=true`n", $utf8NoBom)
        [System.IO.File]::AppendAllText($env:GITHUB_OUTPUT, "release_title=$title`n", $utf8NoBom)
    }
    Write-Host "Prepared release notes from $NotesPath"
    Write-Host "Title: $title"
} else {
    $defaultTitle = if ($env:GITHUB_REF_NAME) { $env:GITHUB_REF_NAME } else { "Release" }
    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    if ($env:GITHUB_OUTPUT) {
        [System.IO.File]::AppendAllText($env:GITHUB_OUTPUT, "has_custom_notes=false`n", $utf8NoBom)
        [System.IO.File]::AppendAllText($env:GITHUB_OUTPUT, "release_title=$defaultTitle`n", $utf8NoBom)
    }
    Write-Host "No custom notes found at $NotesPath"
}
