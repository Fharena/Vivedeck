[CmdletBinding(PositionalBinding = $false)]
param(
  [string]$DeviceId,
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$FlutterArgs
)

$ErrorActionPreference = 'Stop'

function Get-SingleAndroidDeviceId {
  $adb = Join-Path $env:LOCALAPPDATA 'Android\Sdk\platform-tools\adb.exe'
  if (-not (Test-Path $adb)) {
    return $null
  }

  $devices = @(
    & $adb devices |
      Select-Object -Skip 1 |
      Where-Object { $_ -match '\sdevice$' } |
      ForEach-Object { ($_ -split '\s+')[0] }
  )

  if ($devices.Count -gt 1) {
    throw '여러 Android 기기가 연결되어 있습니다. -DeviceId 로 대상 기기를 지정하세요.'
  }

  if ($devices.Count -eq 1) {
    return $devices[0]
  }

  return $null
}

$appRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
$repoRoot = Resolve-Path (Join-Path $appRoot '..\..')
$safeRootParent = Join-Path $env:TEMP 'vibedeck_flutter_safe'
$safeRepoRoot = Join-Path $safeRootParent 'repo'
$linked = $false

if (-not $FlutterArgs -or $FlutterArgs.Count -eq 0) {
  if (-not $DeviceId) {
    $DeviceId = Get-SingleAndroidDeviceId
  }

  $FlutterArgs = @('run')
  if ($DeviceId) {
    $FlutterArgs += @('-d', $DeviceId)
  }
}

try {
  New-Item -ItemType Directory -Force -Path $safeRootParent | Out-Null
  if (Test-Path $safeRepoRoot) {
    Remove-Item $safeRepoRoot -Force -Recurse
  }
  New-Item -ItemType Junction -Path $safeRepoRoot -Target $repoRoot | Out-Null
  $linked = $true

  $flutter = Join-Path $safeRepoRoot 'tools\flutter\bin\flutter.bat'
  $safeAppRoot = Join-Path $safeRepoRoot 'mobile\flutter_app'
  $localProperties = Join-Path $safeAppRoot 'android\local.properties'

  if (Test-Path $localProperties) {
    Remove-Item $localProperties -Force
  }

  Push-Location $safeAppRoot
  & $flutter @FlutterArgs
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