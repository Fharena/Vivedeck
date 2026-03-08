param()

$ErrorActionPreference = 'Stop'

$appRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
$repoRoot = Resolve-Path (Join-Path $appRoot '..\..')
$safeRootParent = Join-Path $env:TEMP 'vibedeck_flutter_safe'
$safeRepoRoot = Join-Path $safeRootParent 'repo'
$linked = $false

try {
  New-Item -ItemType Directory -Force -Path $safeRootParent | Out-Null
  if (Test-Path $safeRepoRoot) {
    Remove-Item $safeRepoRoot -Force -Recurse
  }
  New-Item -ItemType Junction -Path $safeRepoRoot -Target $repoRoot | Out-Null
  $linked = $true

  $flutter = Join-Path $safeRepoRoot 'tools\flutter\bin\flutter.bat'
  $safeAppRoot = Join-Path $safeRepoRoot 'mobile\flutter_app'

  Push-Location $safeAppRoot
  & $flutter test
  exit $LASTEXITCODE
} finally {
  Pop-Location -ErrorAction SilentlyContinue
  if ($linked -and (Test-Path $safeRepoRoot)) {
    Remove-Item $safeRepoRoot -Force -Recurse
  }
  if ((Test-Path $safeRootParent) -and -not (Get-ChildItem $safeRootParent -Force | Select-Object -First 1)) {
    Remove-Item $safeRootParent -Force
  }
}