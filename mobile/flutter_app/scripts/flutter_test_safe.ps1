param(
  [string]$DriveLetter = 'V'
)

$ErrorActionPreference = 'Stop'

$appRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
$repoRoot = Resolve-Path (Join-Path $appRoot '..\..')
$flutter = Join-Path $repoRoot 'tools\flutter\bin\flutter.bat'
$drive = "$DriveLetter`:"
$mapped = $false

try {
  subst $drive "$repoRoot"
  $mapped = $true

  Push-Location "$drive\mobile\flutter_app"
  & $flutter test
  exit $LASTEXITCODE
} finally {
  Pop-Location -ErrorAction SilentlyContinue
  if ($mapped) {
    subst $drive /d | Out-Null
  }
}

