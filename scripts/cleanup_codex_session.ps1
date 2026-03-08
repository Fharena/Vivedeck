param(
    [string]$RepoRoot = (Join-Path $PSScriptRoot '..'),
    [string]$SessionName,
    [string]$SessionRoot,
    [switch]$DeleteBranches,
    [switch]$DeleteSessionFolder
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false

$RequestedRepoRoot = (Resolve-Path $RepoRoot).Path
$GitRoot = (& git -C $RequestedRepoRoot rev-parse --show-toplevel).Trim()
if ($LASTEXITCODE -ne 0) {
    throw 'git top-level을 찾지 못했습니다.'
}
$ResolvedRepoRoot = $GitRoot -replace '/', '\'

if (-not $SessionRoot) {
    if (-not $SessionName) {
        throw 'SessionName 또는 SessionRoot 중 하나는 필요합니다.'
    }
    $SessionRoot = Join-Path $ResolvedRepoRoot ".tmp\codex-sessions\$SessionName"
}

$SessionRoot = $SessionRoot -replace '/', '\'
$ManifestPath = Join-Path $SessionRoot 'session.json'
$Workers = @()
$Branches = @()

if (Test-Path $ManifestPath) {
    $Manifest = Get-Content $ManifestPath -Raw | ConvertFrom-Json
    $Workers = @($Manifest.workers)
    $Branches = @($Workers | ForEach-Object { $_.branch } | Where-Object { $_ })
} elseif ($SessionName) {
    Write-Warning "session.json이 없어 fallback 정리를 수행합니다: $SessionRoot"
    $WorktreeList = & git -C $ResolvedRepoRoot worktree list --porcelain
    $CurrentWorktree = $null
    foreach ($line in $WorktreeList) {
        if ($line -like 'worktree *') {
            $CurrentWorktree = ($line.Substring(9)) -replace '/', '\'
            continue
        }
        if ($line -eq '' -and $CurrentWorktree) {
            if ($CurrentWorktree -like "*\.tmp\codex-sessions\$SessionName\worktrees\*") {
                $Workers += [PSCustomObject]@{ worktreePath = $CurrentWorktree }
            }
            $CurrentWorktree = $null
        }
    }
    if ($CurrentWorktree -and $CurrentWorktree -like "*\.tmp\codex-sessions\$SessionName\worktrees\*") {
        $Workers += [PSCustomObject]@{ worktreePath = $CurrentWorktree }
    }
$Branches = @(& git -C $ResolvedRepoRoot branch --list "codex/dev-$SessionName-*" --format '%(refname:short)' | ForEach-Object { $_.Trim() } | Where-Object { $_ })
} else {
    throw "session.json을 찾을 수 없습니다: $ManifestPath"
}

foreach ($worker in $Workers) {
    if ($worker.worktreePath -and (Test-Path $worker.worktreePath)) {
        & git -C $ResolvedRepoRoot worktree remove --force $worker.worktreePath | Out-Null
    }
}

if ($DeleteBranches) {
    foreach ($branch in ($Branches | Select-Object -Unique)) {
        & git -C $ResolvedRepoRoot branch -D $branch | Out-Null
    }
}

if ($DeleteSessionFolder -and (Test-Path $SessionRoot)) {
    Remove-Item $SessionRoot -Recurse -Force
}

Write-Host '[VibeDeck] Codex worker session cleaned'
Write-Host "SessionRoot: $SessionRoot"
if ($Branches) {
    Write-Host "Branches: $($Branches -join ', ')"
}