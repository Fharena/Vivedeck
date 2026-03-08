param(
    [string]$RepoRoot = (Join-Path $PSScriptRoot '..'),
    [string]$BaseBranch = 'main',
    [switch]$IncludeRemote,
    [string[]]$KeepBranches = @()
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false

$RequestedRepoRoot = (Resolve-Path $RepoRoot).Path
$GitRoot = (& git -C $RequestedRepoRoot rev-parse --show-toplevel).Trim()
if ($LASTEXITCODE -ne 0) {
    throw 'git top-level을 찾지 못했습니다.'
}
$ResolvedRepoRoot = $GitRoot -replace '/', '\'
$CurrentBranch = (& git -C $ResolvedRepoRoot branch --show-current).Trim()
$Keep = @($KeepBranches + @('main', $CurrentBranch)) | Where-Object { $_ } | Select-Object -Unique

$localBranches = & git -C $ResolvedRepoRoot branch --merged $BaseBranch --format '%(refname:short)'
$localTargets = $localBranches |
    ForEach-Object { $_.Trim() } |
    Where-Object { $_ -like 'codex/*' } |
    Where-Object { $Keep -notcontains $_ }

foreach ($branch in $localTargets) {
    & git -C $ResolvedRepoRoot branch -d $branch | Out-Null
}

$remoteTargets = @()
if ($IncludeRemote) {
    & git -C $ResolvedRepoRoot fetch origin --prune | Out-Null
    $remoteBranches = & git -C $ResolvedRepoRoot branch -r --merged origin/$BaseBranch --format '%(refname:short)'
    $remoteTargets = $remoteBranches |
        ForEach-Object { $_.Trim() } |
        Where-Object { $_ -like 'origin/codex/*' } |
        ForEach-Object { $_ -replace '^origin/', '' } |
        Where-Object { $Keep -notcontains $_ }

    foreach ($branch in $remoteTargets) {
        & git -C $ResolvedRepoRoot push origin --delete $branch | Out-Null
    }
}

Write-Host '[VibeDeck] merged codex branches cleaned'
Write-Host "BaseBranch: $BaseBranch"
if ($localTargets) {
    Write-Host "Deleted local branches: $($localTargets -join ', ')"
} else {
    Write-Host 'Deleted local branches: none'
}
if ($IncludeRemote) {
    if ($remoteTargets) {
        Write-Host "Deleted remote branches: $($remoteTargets -join ', ')"
    } else {
        Write-Host 'Deleted remote branches: none'
    }
}