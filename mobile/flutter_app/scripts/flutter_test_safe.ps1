param()

$ErrorActionPreference = 'Stop'

$appRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
$repoRoot = Resolve-Path (Join-Path $appRoot '..\..')
$repoItem = Get-Item $repoRoot
if ($repoItem.LinkType -eq 'Junction' -and $repoItem.Target -and $repoItem.Target.Count -gt 0) {
  $repoRoot = $repoItem.Target[0]
}
$substDrive = 'V:'
$appDataRoot = Join-Path $env:TEMP 'vibedeck_flutter_appdata'
$substMounted = $false

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

  cmd /c "pushd $safeAppRoot && call $flutter test"
  exit $LASTEXITCODE
} finally {
  if ($substMounted) {
    cmd /c "subst $substDrive /d" | Out-Null
  }
}