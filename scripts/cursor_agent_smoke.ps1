param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [string]$AgentBaseUrl = "http://127.0.0.1:18080",
    [string]$CursorAgentBin = "",
    [int]$StartupTimeoutSec = 60,
    [switch]$KeepTempRoot
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

function Invoke-AgentJson {
    param(
        [Parameter(Mandatory = $true)][ValidateSet('GET','POST')][string]$Method,
        [Parameter(Mandatory = $true)][string]$Uri,
        [object]$Body
    )

    if ($Method -eq 'GET') {
        return Invoke-RestMethod -Method Get -Uri $Uri
    }

    $payload = $Body | ConvertTo-Json -Depth 8
    return Invoke-RestMethod -Method Post -Uri $Uri -ContentType 'application/json' -Body $payload
}

function New-Envelope {
    param(
        [Parameter(Mandatory = $true)][string]$Sid,
        [Parameter(Mandatory = $true)][string]$Rid,
        [Parameter(Mandatory = $true)][int]$Seq,
        [Parameter(Mandatory = $true)][string]$Type,
        [Parameter(Mandatory = $true)][hashtable]$Payload
    )

    return @{
        sid = $Sid
        rid = $Rid
        seq = $Seq
        ts = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
        type = $Type
        payload = $Payload
    }
}

function Wait-AgentReady {
    param(
        [Parameter(Mandatory = $true)][string]$HealthUrl,
        [Parameter(Mandatory = $true)][System.Diagnostics.Process]$Process,
        [Parameter(Mandatory = $true)][int]$TimeoutSec,
        [string]$StdErrLog = ""
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        try {
            return Invoke-RestMethod -Method Get -Uri $HealthUrl
        } catch {
            if ($Process.HasExited) {
                $stderr = ""
                if ($StdErrLog -and (Test-Path $StdErrLog)) {
                    $stderr = Get-Content -Path $StdErrLog -Raw
                }
                throw "agent process exited before health check succeeded. stderr: $stderr"
            }
            Start-Sleep -Milliseconds 250
        }
    }

    throw "agent health check timed out after ${TimeoutSec}s"
}

$repoRootResolved = (Resolve-Path $RepoRoot).Path
$goCommand = Resolve-RequiredCommand -Name 'go' -Hint 'Go 1.23+가 필요합니다.'
$gitCommand = Resolve-RequiredCommand -Name 'git' -Hint 'Git이 필요합니다.'

if ([string]::IsNullOrWhiteSpace($CursorAgentBin)) {
    if (-not [string]::IsNullOrWhiteSpace($env:CURSOR_AGENT_BIN)) {
        $CursorAgentBin = $env:CURSOR_AGENT_BIN
    } else {
        $CursorAgentBin = 'cursor-agent'
    }
}
$cursorAgentCommand = Resolve-RequiredCommand -Name $CursorAgentBin -Hint 'Cursor CLI 설치 후 PATH 또는 CURSOR_AGENT_BIN을 설정하세요.'
$cursorAgentVersion = (& $cursorAgentCommand.Source --version 2>&1 | Out-String).Trim()

$agentUri = [System.Uri]$AgentBaseUrl
$listenAddress = if ($agentUri.IsDefaultPort) { $agentUri.Host } else { "{0}:{1}" -f $agentUri.Host, $agentUri.Port }
if ([string]::IsNullOrWhiteSpace($listenAddress)) {
    $listenAddress = '127.0.0.1:18080'
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("vibedeck-cursor-smoke-" + [System.Guid]::NewGuid().ToString('N'))
$workspaceRoot = Join-Path $tempRoot 'workspace'
$logsDir = Join-Path $tempRoot 'logs'
$profilesPath = Join-Path $tempRoot 'run-profiles.json'
$agentScriptPath = Join-Path $tempRoot 'run-agent.cmd'
$stdoutLog = Join-Path $logsDir 'agent.stdout.log'
$stderrLog = Join-Path $logsDir 'agent.stderr.log'

New-Item -ItemType Directory -Force -Path $workspaceRoot | Out-Null
New-Item -ItemType Directory -Force -Path $logsDir | Out-Null

Push-Location $workspaceRoot
try {
    & $gitCommand.Source init | Out-Null
    & $gitCommand.Source config user.name 'VibeDeck Smoke' | Out-Null
    & $gitCommand.Source config user.email 'vibedeck-smoke@example.local' | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $workspaceRoot 'notes.txt'), "base`n", [System.Text.UTF8Encoding]::new($false))
    & $gitCommand.Source add -A | Out-Null
    & $gitCommand.Source commit -m 'base' | Out-Null
} finally {
    Pop-Location
}

$profilesJson = @{
    smoke = @{
        label = 'Smoke'
        command = 'git status --short'
        scope = 'SMALL'
    }
} | ConvertTo-Json -Depth 5
[System.IO.File]::WriteAllText($profilesPath, $profilesJson, [System.Text.UTF8Encoding]::new($false))

$scriptContent = @(
    '@echo off',
    ('set AGENT_ADDR={0}' -f $listenAddress),
    'set WORKSPACE_ADAPTER_MODE=cursor_agent_cli',
    ('set CURSOR_AGENT_BIN={0}' -f $cursorAgentCommand.Source),
    ('set CURSOR_AGENT_WORKSPACE_ROOT={0}' -f $workspaceRoot),
    ('set RUN_PROFILE_FILE={0}' -f $profilesPath),
    ('set SIGNALING_BASE_URL=http://127.0.0.1:8081'),
    ('"{0}" run ./cmd/agent 1>"{1}" 2>"{2}"' -f $goCommand.Source, $stdoutLog, $stderrLog)
) -join "`r`n"
[System.IO.File]::WriteAllText($agentScriptPath, $scriptContent + "`r`n", [System.Text.UTF8Encoding]::new($false))

$agentProcess = $null
try {
    $agentProcess = Start-Process -FilePath $agentScriptPath -WorkingDirectory $repoRootResolved -PassThru -WindowStyle Hidden
    $health = Wait-AgentReady -HealthUrl ($AgentBaseUrl.TrimEnd('/') + '/healthz') -Process $agentProcess -TimeoutSec $StartupTimeoutSec -StdErrLog $stderrLog
    $adapter = Invoke-AgentJson -Method GET -Uri ($AgentBaseUrl.TrimEnd('/') + '/v1/agent/runtime/adapter')

    if ($adapter.name -ne 'cursor-agent-cli') {
        throw "unexpected adapter name: $($adapter.name)"
    }
    if (-not $adapter.ready) {
        throw 'adapter is not ready'
    }

    $sid = 'sid-smoke'
    $promptResponse = Invoke-AgentJson -Method POST -Uri ($AgentBaseUrl.TrimEnd('/') + '/v1/agent/envelope') -Body (New-Envelope -Sid $sid -Rid 'rid-prompt-1' -Seq 1 -Type 'PROMPT_SUBMIT' -Payload @{
        prompt = 'Append the line "smoke-agent" to notes.txt only. Do not modify any other files.'
        template = 'smoke'
        contextOptions = @{
            includeActiveFile = $false
            includeSelection = $false
            includeLatestError = $false
            includeWorkspaceSummary = $true
        }
    })

    $promptAck = $promptResponse.responses | Where-Object { $_.type -eq 'PROMPT_ACK' } | Select-Object -First 1
    $patchReady = $promptResponse.responses | Where-Object { $_.type -eq 'PATCH_READY' } | Select-Object -First 1
    if (-not $promptAck) {
        throw 'PROMPT_ACK response not found'
    }
    if (-not $patchReady) {
        throw 'PATCH_READY response not found'
    }

    $jobId = $promptAck.payload.jobId
    if ([string]::IsNullOrWhiteSpace($jobId)) {
        throw 'jobId missing from PROMPT_ACK'
    }

    $applyResponse = Invoke-AgentJson -Method POST -Uri ($AgentBaseUrl.TrimEnd('/') + '/v1/agent/envelope') -Body (New-Envelope -Sid $sid -Rid 'rid-apply-1' -Seq 2 -Type 'PATCH_APPLY' -Payload @{
        jobId = $jobId
        mode = 'all'
    })
    $patchResult = $applyResponse.responses | Where-Object { $_.type -eq 'PATCH_RESULT' } | Select-Object -First 1
    if (-not $patchResult) {
        throw 'PATCH_RESULT response not found'
    }
    if ($patchResult.payload.status -notin @('success','partial')) {
        throw "patch apply failed: $($patchResult.payload.status) $($patchResult.payload.message)"
    }

    $runResponse = Invoke-AgentJson -Method POST -Uri ($AgentBaseUrl.TrimEnd('/') + '/v1/agent/envelope') -Body (New-Envelope -Sid $sid -Rid 'rid-run-1' -Seq 3 -Type 'RUN_PROFILE' -Payload @{
        jobId = $jobId
        profileId = 'smoke'
    })
    $runResult = $runResponse.responses | Where-Object { $_.type -eq 'RUN_RESULT' } | Select-Object -First 1
    if (-not $runResult) {
        throw 'RUN_RESULT response not found'
    }
    if ($runResult.payload.status -ne 'passed') {
        throw "run profile failed: $($runResult.payload.summary)"
    }

    $notesContent = Get-Content -Path (Join-Path $workspaceRoot 'notes.txt') -Raw
    if ($notesContent -notmatch 'smoke-agent') {
        throw "smoke change not found in notes.txt: $notesContent"
    }

    [PSCustomObject]@{
        cursorAgentVersion = $cursorAgentVersion
        adapterName = $adapter.name
        adapterMode = $adapter.mode
        workspaceRoot = $adapter.workspaceRoot
        patchSummary = $patchReady.payload.summary
        patchFiles = @($patchReady.payload.files | ForEach-Object { $_.path })
        applyStatus = $patchResult.payload.status
        runStatus = $runResult.payload.status
        runSummary = $runResult.payload.summary
        notesContent = $notesContent.Trim()
        tempRoot = $tempRoot
    }
} finally {
    if ($agentProcess -and -not $agentProcess.HasExited) {
        Stop-Process -Id $agentProcess.Id -Force
        $agentProcess.WaitForExit()
    }
    if (-not $KeepTempRoot -and (Test-Path $tempRoot)) {
        Remove-Item -Path $tempRoot -Recurse -Force
    }
}
