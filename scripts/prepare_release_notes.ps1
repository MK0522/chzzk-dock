param(
    [string]$NotesPath = "RELEASE_NOTES.md"
)

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

    if ($env:GITHUB_OUTPUT) {
        Add-Content -Path $env:GITHUB_OUTPUT -Value "has_custom_notes=true"
        Add-Content -Path $env:GITHUB_OUTPUT -Value "release_title=$title"
    }
    Write-Host "Prepared release notes from $NotesPath"
    Write-Host "Title: $title"
} else {
    $defaultTitle = if ($env:GITHUB_REF_NAME) { $env:GITHUB_REF_NAME } else { "Release" }
    if ($env:GITHUB_OUTPUT) {
        Add-Content -Path $env:GITHUB_OUTPUT -Value "has_custom_notes=false"
        Add-Content -Path $env:GITHUB_OUTPUT -Value "release_title=$defaultTitle"
    }
    Write-Host "No custom notes found at $NotesPath"
}
