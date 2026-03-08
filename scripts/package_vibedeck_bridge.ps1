param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [string]$OutputDir = "",
    [switch]$InstallDependencies,
    [switch]$SkipCheck,
    [switch]$RunSmoke
)

$ErrorActionPreference = "Stop"

function Resolve-RequiredCommand {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [string]$Hint = ""
    )

    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if (-not $command) {
        if ($Hint) {
            throw "$Name 명령을 찾을 수 없습니다. $Hint"
        }
        throw "$Name 명령을 찾을 수 없습니다."
    }
    return $command
}

function Invoke-NpmScript {
    param(
        [Parameter(Mandatory = $true)][string]$NpmPath,
        [Parameter(Mandatory = $true)][string]$PrefixPath,
        [Parameter(Mandatory = $true)][string[]]$Arguments
    )

    & $NpmPath "--prefix" $PrefixPath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "npm command failed: npm --prefix $PrefixPath $($Arguments -join ' ')"
    }
}

$repoRootResolved = (Resolve-Path $RepoRoot).Path
$adaptersPath = Join-Path $repoRootResolved "adapters\cursor-bridge"
$extensionPath = Join-Path $repoRootResolved "extensions\vibedeck-bridge"
$packageJsonPath = Join-Path $extensionPath "package.json"
$packageJson = Get-Content $packageJsonPath -Raw | ConvertFrom-Json

if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    $OutputDir = Join-Path $repoRootResolved "artifacts\vsix"
}

$npm = Resolve-RequiredCommand -Name "npm" -Hint "Node.js/npm이 필요합니다."
$node = Resolve-RequiredCommand -Name "node" -Hint "Node.js가 필요합니다."

if ($InstallDependencies) {
    Invoke-NpmScript -NpmPath $npm.Source -PrefixPath $adaptersPath -Arguments @("install")
    Invoke-NpmScript -NpmPath $npm.Source -PrefixPath $extensionPath -Arguments @("install")
}

$vscePackageJson = Join-Path $extensionPath "node_modules\@vscode\vsce\package.json"
if (-not (Test-Path $vscePackageJson)) {
    throw "VSIX 패키징 도구가 없습니다. .\scripts\package_vibedeck_bridge.ps1 -InstallDependencies 또는 cd .\extensions\vibedeck-bridge ; npm install 을 먼저 실행하세요."
}

Invoke-NpmScript -NpmPath $npm.Source -PrefixPath $adaptersPath -Arguments @("run", "build")
if (-not $SkipCheck) {
    Invoke-NpmScript -NpmPath $npm.Source -PrefixPath $extensionPath -Arguments @("run", "check")
}
Invoke-NpmScript -NpmPath $npm.Source -PrefixPath $extensionPath -Arguments @("run", "build")

if ($RunSmoke) {
    Invoke-NpmScript -NpmPath $npm.Source -PrefixPath $extensionPath -Arguments @("run", "smoke:provider")
    Invoke-NpmScript -NpmPath $npm.Source -PrefixPath $extensionPath -Arguments @("run", "smoke:extension")
}

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$version = $packageJson.version
$vsixPath = Join-Path $OutputDir ("vibedeck-bridge-" + $version + ".vsix")
if (Test-Path $vsixPath) {
    Remove-Item $vsixPath -Force
}

$packageScript = Join-Path $extensionPath "scripts\package_vsix.mjs"
& $node.Source $packageScript "--out" $vsixPath
if ($LASTEXITCODE -ne 0) {
    throw "VSIX 패키징 실패: $vsixPath"
}

if (-not (Test-Path $vsixPath)) {
    throw "VSIX 생성 실패: $vsixPath"
}

$artifact = Get-Item $vsixPath
[PSCustomObject]@{
    vsixPath = $artifact.FullName
    version = $version
    sizeBytes = $artifact.Length
    installCommandCursor = "cursor --install-extension `"$($artifact.FullName)`" --force"
    installCommandCode = "code --install-extension `"$($artifact.FullName)`" --force"
}
