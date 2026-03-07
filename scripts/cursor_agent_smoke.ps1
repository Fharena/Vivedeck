param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [string]$AgentBaseUrl = "http://127.0.0.1:18080",
    [string]$CursorAgentBin = "",
    [switch]$UseWslCursorAgent,
    [string]$CursorAgentWslDistro = "",
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

function Invoke-CommandCapture {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [string[]]$ArgumentList = @()
    )

    $stdoutPath = Join-Path ([System.IO.Path]::GetTempPath()) ("vibedeck-capture-" + [System.Guid]::NewGuid().ToString('N') + '.stdout.log')
    $stderrPath = Join-Path ([System.IO.Path]::GetTempPath()) ("vibedeck-capture-" + [System.Guid]::NewGuid().ToString('N') + '.stderr.log')
    try {
        $process = Start-Process -FilePath $FilePath -ArgumentList $ArgumentList -Wait -NoNewWindow -PassThru -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath
        $stdout = if (Test-Path $stdoutPath) { Get-Content -Path $stdoutPath -Raw } else { '' }
        $stderr = if (Test-Path $stderrPath) { Get-Content -Path $stderrPath -Raw } else { '' }
        $combined = @($stdout, $stderr) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
        return [PSCustomObject]@{
            ExitCode = $process.ExitCode
            Output = ($combined -join "`n").Trim()
        }
    } finally {
        if (Test-Path $stdoutPath) { Remove-Item $stdoutPath -Force }
        if (Test-Path $stderrPath) { Remove-Item $stderrPath -Force }
    }
}

function Normalize-WslOutput {
    param([string]$Value)

    return ($Value -replace "`0", '').Trim()
}

function Get-WslArgumentList {
    param(
        [string]$Distro,
        [string[]]$CommandArguments = @()
    )

    $args = @()
    if (-not [string]::IsNullOrWhiteSpace($Distro)) {
        $args += '-d'
        $args += $Distro
    }
    $args += '--'
    $args += $CommandArguments
    return ,$args
}

function Invoke-WslCommandCapture {
    param(
        [Parameter(Mandatory = $true)][string]$WslPath,
        [string]$Distro = '',
        [string[]]$CommandArguments = @()
    )

    return Invoke-CommandCapture -FilePath $WslPath -ArgumentList (Get-WslArgumentList -Distro $Distro -CommandArguments $CommandArguments)
}

function Get-WslHomePath {
    param(
        [Parameter(Mandatory = $true)][string]$WslPath,
        [string]$Distro = ''
    )

    $result = Invoke-WslCommandCapture -WslPath $WslPath -Distro $Distro -CommandArguments @('printenv', 'HOME')
    $result.Output = Normalize-WslOutput $result.Output
    if ($result.ExitCode -ne 0 -or [string]::IsNullOrWhiteSpace($result.Output)) {
        return $null
    }
    return $result.Output
}

function Resolve-WslCursorAgentInDistro {
    param(
        [Parameter(Mandatory = $true)][string]$WslPath,
        [string]$Distro = ''
    )

    $homePath = Get-WslHomePath -WslPath $WslPath -Distro $Distro
    if ([string]::IsNullOrWhiteSpace($homePath)) {
        return $null
    }

    $candidates = @(
        "$homePath/.local/bin/cursor-agent",
        "$homePath/.local/bin/agent"
    )
    foreach ($candidate in $candidates) {
        $version = Invoke-WslCommandCapture -WslPath $WslPath -Distro $Distro -CommandArguments @($candidate, '--version')
        $version.Output = Normalize-WslOutput $version.Output
        if ($version.ExitCode -eq 0 -and -not [string]::IsNullOrWhiteSpace($version.Output)) {
            return [PSCustomObject]@{
                Distro = $Distro
                Binary = $candidate
                Version = $version.Output
            }
        }
    }
    return $null
}

function Get-WslDistroCandidates {
    param([Parameter(Mandatory = $true)][string]$WslPath)

    $list = Invoke-CommandCapture -FilePath $WslPath -ArgumentList @('-l', '-q')
    $list.Output = Normalize-WslOutput $list.Output
    if ($list.ExitCode -ne 0) {
        throw "WSL distro 목록 조회 실패: $($list.Output)"
    }

    $distros = @()
    foreach ($line in ($list.Output -split "`n")) {
        $name = $line.Trim()
        if (-not [string]::IsNullOrWhiteSpace($name) -and -not ($distros -contains $name)) {
            $distros += $name
        }
    }
    return ,$distros
}

function Resolve-WslCursorAgentDistro {
    param(
        [Parameter(Mandatory = $true)][string]$WslPath,
        [string]$RequestedDistro = ''
    )

    if (-not [string]::IsNullOrWhiteSpace($RequestedDistro)) {
        $resolved = Resolve-WslCursorAgentInDistro -WslPath $WslPath -Distro $RequestedDistro
        if ($null -eq $resolved) {
            throw "WSL distro '$RequestedDistro' 에서 cursor-agent를 찾지 못했습니다."
        }
        return $resolved
    }

    $defaultResolved = Resolve-WslCursorAgentInDistro -WslPath $WslPath
    if ($null -ne $defaultResolved) {
        return $defaultResolved
    }

    foreach ($distro in (Get-WslDistroCandidates -WslPath $WslPath)) {
        $resolved = Resolve-WslCursorAgentInDistro -WslPath $WslPath -Distro $distro
        if ($null -ne $resolved) {
            return $resolved
        }
    }

    throw 'WSL 어느 distro에서도 cursor-agent를 찾지 못했습니다. CURSOR_AGENT_WSL_DISTRO를 지정하세요.'
}
function Resolve-CursorAgentInvocation {
    param(
        [string]$RequestedBin,
        [bool]$PreferWsl,
        [string]$WslDistro
    )

    $wslRequested = $PreferWsl
    if (-not $wslRequested -and -not [string]::IsNullOrWhiteSpace($RequestedBin)) {
        $leaf = [System.IO.Path]::GetFileName($RequestedBin)
        $wslRequested = $leaf -ieq 'wsl.exe' -or $leaf -ieq 'wsl'
    }

    if (-not $wslRequested -and -not [string]::IsNullOrWhiteSpace($RequestedBin)) {
        $command = Resolve-RequiredCommand -Name $RequestedBin -Hint 'Cursor CLI 설치 후 PATH 또는 CURSOR_AGENT_BIN을 설정하세요.'
        $version = Invoke-CommandCapture -FilePath $command.Source -ArgumentList @('--version')
        if ($version.ExitCode -ne 0) {
            throw "cursor-agent 버전 확인 실패: $($version.Output)"
        }
        return [PSCustomObject]@{
            Binary = $command.Source
            Version = $version.Output
            UseWsl = $false
            WslDistro = ''
            NestedBinary = $command.Source
        }
    }

    if (-not $wslRequested) {
        $native = Get-Command 'cursor-agent' -ErrorAction SilentlyContinue
        if ($native) {
            $version = Invoke-CommandCapture -FilePath $native.Source -ArgumentList @('--version')
            if ($version.ExitCode -ne 0) {
                throw "cursor-agent 버전 확인 실패: $($version.Output)"
            }
            return [PSCustomObject]@{
                Binary = $native.Source
                Version = $version.Output
                UseWsl = $false
                WslDistro = ''
                NestedBinary = $native.Source
            }
        }
    }

    $wsl = Resolve-RequiredCommand -Name 'wsl.exe' -Hint 'Windows에서는 WSL 또는 네이티브 cursor-agent 중 하나가 필요합니다.'
    $resolved = Resolve-WslCursorAgentDistro -WslPath $wsl.Source -RequestedDistro $WslDistro

    return [PSCustomObject]@{
        Binary = $wsl.Source
        Version = $resolved.Version
        UseWsl = $true
        WslDistro = $resolved.Distro
        NestedBinary = $resolved.Binary
    }
}

function Get-CursorAgentLoginHint {
    param([Parameter(Mandatory = $true)][object]$Invocation)

    if ($Invocation.UseWsl) {
        $prefix = 'wsl.exe --'
        if (-not [string]::IsNullOrWhiteSpace($Invocation.WslDistro)) {
            $prefix = 'wsl.exe -d ' + $Invocation.WslDistro + ' --'
        }
        return $prefix + ' ' + $Invocation.NestedBinary + ' login'
    }
    return $Invocation.NestedBinary + ' login'
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

if ([string]::IsNullOrWhiteSpace($CursorAgentBin) -and -not [string]::IsNullOrWhiteSpace($env:CURSOR_AGENT_BIN)) {
    $CursorAgentBin = $env:CURSOR_AGENT_BIN
}
if (-not $UseWslCursorAgent.IsPresent -and -not [string]::IsNullOrWhiteSpace($env:CURSOR_AGENT_USE_WSL)) {
    $UseWslCursorAgent = @('1','true','yes','on') -contains $env:CURSOR_AGENT_USE_WSL.Trim().ToLowerInvariant()
}
if ([string]::IsNullOrWhiteSpace($CursorAgentWslDistro) -and -not [string]::IsNullOrWhiteSpace($env:CURSOR_AGENT_WSL_DISTRO)) {
    $CursorAgentWslDistro = $env:CURSOR_AGENT_WSL_DISTRO
}

$cursorAgentInvocation = Resolve-CursorAgentInvocation -RequestedBin $CursorAgentBin -PreferWsl ([bool]$UseWslCursorAgent) -WslDistro $CursorAgentWslDistro
$cursorAgentVersion = $cursorAgentInvocation.Version

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
    ('set CURSOR_AGENT_BIN={0}' -f $cursorAgentInvocation.Binary),
    ('set CURSOR_AGENT_USE_WSL={0}' -f ($(if ($cursorAgentInvocation.UseWsl) { 'true' } else { 'false' }))),
    ('set CURSOR_AGENT_WSL_DISTRO={0}' -f $cursorAgentInvocation.WslDistro),
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
    try {
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
    } catch {
        $message = ''
        if ($_.ErrorDetails -and -not [string]::IsNullOrWhiteSpace($_.ErrorDetails.Message)) {
            $message = $_.ErrorDetails.Message
        }
        if ([string]::IsNullOrWhiteSpace($message)) {
            $message = $_.Exception.Message
        }
        if ($message -like '*Authentication required*') {
            $loginHint = Get-CursorAgentLoginHint -Invocation $cursorAgentInvocation
            throw "cursor-agent 인증이 필요합니다. 먼저 '$loginHint' 를 실행하거나 CURSOR_API_KEY 환경변수를 설정하세요."
        }
        throw
    }

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
        for ($attempt = 0; $attempt -lt 3; $attempt++) {
            try {
                Remove-Item -Path $tempRoot -Recurse -Force -ErrorAction Stop
                break
            } catch {
                if ($attempt -eq 2) {
                    Write-Warning "temp root cleanup skipped: $tempRoot / $($_.Exception.Message)"
                } else {
                    Start-Sleep -Milliseconds 250
                }
            }
        }
    }
}
