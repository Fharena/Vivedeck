param(
    [string]$RepoRoot = (Join-Path $PSScriptRoot '..'),
    [string]$BaseRef,
    [string]$SessionName = (Get-Date -Format 'yyyyMMdd-HHmmss'),
    [string[]]$Workers = @(
        'agent=cmd/agent,internal/agent,internal/runtime',
        'mobile=mobile/flutter_app',
        'extension=extensions/vibedeck-bridge,adapters/cursor-bridge'
    ),
    [switch]$ForceRecreate
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false

function Invoke-Git {
    param(
        [string[]]$GitArgs,
        [string]$WorkingDirectory = $ResolvedRepoRoot
    )

    $output = & git -C $WorkingDirectory @GitArgs 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "git $($GitArgs -join ' ') failed: $($output -join [Environment]::NewLine)"
    }
    return $output
}

function ConvertTo-SafeName {
    param([string]$Value)
    $safe = ($Value.Trim().ToLower() -replace '[^a-z0-9._-]+', '-')
    $safe = $safe.Trim('-','.')
    if ([string]::IsNullOrWhiteSpace($safe)) {
        return 'worker'
    }
    return $safe
}

function Parse-WorkerSpec {
    param([string]$Spec)
    $parts = $Spec.Split('=', 2)
    if ($parts.Count -ne 2) {
        throw "worker spec 형식이 잘못됐습니다: $Spec"
    }
    $name = ConvertTo-SafeName $parts[0]
    $scopes = $parts[1].Split(',') | ForEach-Object { $_.Trim() } | Where-Object { $_ }
    if ($scopes.Count -eq 0) {
        throw "worker spec에 최소 한 개 scope가 필요합니다: $Spec"
    }
    return [PSCustomObject]@{
        Name = $name
        Scopes = $scopes
    }
}

$RequestedRepoRoot = (Resolve-Path $RepoRoot).Path
$GitRoot = (Invoke-Git -GitArgs @('rev-parse', '--show-toplevel') -WorkingDirectory $RequestedRepoRoot | Select-Object -Last 1).Trim()
$ResolvedRepoRoot = $GitRoot -replace '/', '\'
$SessionRoot = Join-Path $ResolvedRepoRoot ".tmp\codex-sessions\$SessionName"
$WorktreeRoot = Join-Path $SessionRoot 'worktrees'
$InstructionRoot = Join-Path $SessionRoot 'instructions'
New-Item -ItemType Directory -Force -Path $WorktreeRoot | Out-Null
New-Item -ItemType Directory -Force -Path $InstructionRoot | Out-Null

$CurrentBranch = (Invoke-Git -GitArgs @('branch', '--show-current') | Select-Object -Last 1).Trim()
$EffectiveBaseRef = if ($BaseRef) { $BaseRef } else { $CurrentBranch }
$DirtyStatus = (& git -C $ResolvedRepoRoot status --short)
if ($DirtyStatus) {
    Write-Warning '현재 working tree 변경은 새 worktree에 자동 반영되지 않습니다. 필요한 변경은 먼저 commit하거나 별도로 전달하세요.'
}
$WorkersResolved = @()

for ($i = 0; $i -lt $Workers.Count; $i++) {
    $worker = Parse-WorkerSpec $Workers[$i]
    $ordinal = '{0:D2}' -f ($i + 1)
    $branch = "codex/dev-$SessionName-$($worker.Name)"
    $worktreePath = Join-Path $WorktreeRoot $worker.Name
    $instructionPath = Join-Path $InstructionRoot "$ordinal-$($worker.Name).md"

    if (Test-Path $worktreePath) {
        if (-not $ForceRecreate) {
            throw "이미 worktree가 존재합니다: $worktreePath. 덮어쓰려면 -ForceRecreate를 사용하세요."
        }
        & git -C $ResolvedRepoRoot worktree remove --force $worktreePath | Out-Null
    }

    $existingBranch = (& git -C $ResolvedRepoRoot branch --list $branch | ForEach-Object { $_.Trim() })
    if ($existingBranch) {
        if (-not $ForceRecreate) {
            throw "이미 branch가 존재합니다: $branch. 덮어쓰려면 -ForceRecreate를 사용하세요."
        }
        & git -C $ResolvedRepoRoot branch -D $branch | Out-Null
    }

    Invoke-Git -GitArgs @('worktree', 'add', '--quiet', '-B', $branch, $worktreePath, $EffectiveBaseRef) | Out-Null

    $prompt = @(
        "# Codex Worker: $($worker.Name)",
        "",
        "## Worktree",
        "- Path: $worktreePath",
        "- Branch: $branch",
        "- BaseRef: $EffectiveBaseRef",
        "",
        "## Assigned Scope",
        ($worker.Scopes | ForEach-Object { "- $_" }),
        "",
        "## Rules",
        "- Assigned scope 밖 파일은 건드리지 마세요.",
        "- scope 경계가 애매하면 coordinator thread에 먼저 확인만 남기고 멈추세요.",
        "- 큰 구조 변경보다 review 가능한 작은 patch를 우선하세요.",
        "- 테스트/검증 결과는 명령과 핵심 결과만 짧게 남기세요.",
        "",
        "## Starter Prompt",
        "이 worktree는 VibeDeck 개발용 worker 세션입니다.",
        "현재 담당 scope만 수정하세요:",
        ($worker.Scopes | ForEach-Object { "- $_" }),
        "필요 없는 파일은 건드리지 말고, 변경 이유와 검증 결과를 짧게 정리하세요."
    ) -join [Environment]::NewLine
    [System.IO.File]::WriteAllText($instructionPath, $prompt, [System.Text.UTF8Encoding]::new($false))

    $WorkersResolved += [PSCustomObject]@{
        name = $worker.Name
        branch = $branch
        worktreePath = $worktreePath
        instructionPath = $instructionPath
        scopes = $worker.Scopes
    }
}

$manifest = [ordered]@{
    sessionName = $SessionName
    createdAt = (Get-Date).ToString('s')
    repoRoot = $ResolvedRepoRoot
    currentBranch = $CurrentBranch
    baseRef = $EffectiveBaseRef
    dirtyAtCreation = [bool]($DirtyStatus)
    workers = $WorkersResolved
}
$manifestPath = Join-Path $SessionRoot 'session.json'
[System.IO.File]::WriteAllText(
    $manifestPath,
    ($manifest | ConvertTo-Json -Depth 8),
    [System.Text.UTF8Encoding]::new($false)
)

$sessionGuide = @(
    "# Codex Parallel Dev Session",
    "",
    "- Session: $SessionName",
    "- Repo: $ResolvedRepoRoot",
    "- BaseRef: $EffectiveBaseRef",
    "- Main coordinator branch at creation: $CurrentBranch",
    "- Dirty repo at creation: $([bool]($DirtyStatus))",
    "",
    "## Worker Summary",
    ($WorkersResolved | ForEach-Object { "- $($_.name): $($_.worktreePath) [$($_.branch)]" }),
    "",
    "## Coordinator Rules",
    "- main thread는 통합/검토/최종 적용만 담당합니다.",
    "- worker는 자기 scope 밖으로 나가지 않습니다.",
    "- worker 결과는 commit/hash나 diff 요약으로만 받습니다.",
    "- 충돌 가능성이 있으면 worker를 멈추고 scope를 다시 자릅니다.",
    "",
    "## Next Steps",
    "1. Codex에서 worker 수만큼 새 세션/창을 엽니다.",
    "2. 각 세션에서 해당 worktree 폴더를 workspace로 엽니다.",
    "3. instructions 폴더의 worker markdown 내용을 그대로 시작 프롬프트로 붙입니다.",
    "4. 이 coordinator thread는 통합과 테스트만 담당합니다."
) -join [Environment]::NewLine
[System.IO.File]::WriteAllText((Join-Path $SessionRoot 'SESSION.md'), $sessionGuide, [System.Text.UTF8Encoding]::new($false))

Write-Host "[VibeDeck] Codex worker session prepared"
Write-Host "SessionRoot: $SessionRoot"
Write-Host "Manifest: $manifestPath"
foreach ($worker in $WorkersResolved) {
    Write-Host "- $($worker.name): $($worker.worktreePath) [$($worker.branch)]"
    Write-Host "  instruction: $($worker.instructionPath)"
}