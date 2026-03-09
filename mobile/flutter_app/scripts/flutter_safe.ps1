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
$repoItem = Get-Item $repoRoot
if ($repoItem.LinkType -eq 'Junction' -and $repoItem.Target -and $repoItem.Target.Count -gt 0) {
  $repoRoot = $repoItem.Target[0]
}
$substDrive = 'V:'
$appDataRoot = Join-Path $env:TEMP 'vibedeck_flutter_appdata'
$substMounted = $false

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
  New-Item -ItemType Directory -Force -Path $appDataRoot | Out-Null
  $env:APPDATA = $appDataRoot

  cmd /c "subst $substDrive /d" | Out-Null
  cmd /c "subst $substDrive `"$repoRoot`"" | Out-Null
  $substMounted = $true

  $flutter = "$substDrive\tools\flutter\bin\flutter.bat"
  $safeAppRoot = "$substDrive\mobile\flutter_app"
  $localProperties = "$safeAppRoot\android\local.properties"

  if (Test-Path $localProperties) {
    Remove-Item $localProperties -Force
  }

  $argString = [string]::Join(' ', $FlutterArgs)
  cmd /c "pushd $safeAppRoot && call $flutter $argString"
  exit $LASTEXITCODE
} finally {
  if ($substMounted) {
    cmd /c "subst $substDrive /d" | Out-Null
  }
}