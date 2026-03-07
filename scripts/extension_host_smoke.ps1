param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [string]$BridgeAddress = $(if ($env:CURSOR_BRIDGE_TCP_ADDR) { $env:CURSOR_BRIDGE_TCP_ADDR } else { "127.0.0.1:7797" }),
    [string]$AgentBaseUrl = "",
    [string]$ExpectedRunStatus = "failed",
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

function Get-FreeLoopbackUrl {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        $endpoint = [System.Net.IPEndPoint]$listener.LocalEndpoint
        return "http://127.0.0.1:$($endpoint.Port)"
    } finally {
        $listener.Stop()
    }
}

function Split-BridgeAddress {
    param([Parameter(Mandatory = $true)][string]$Address)

    $trimmed = $Address.Trim()
    $separator = $trimmed.LastIndexOf(':')
    if ($separator -lt 1) {
        throw "bridge 주소 형식이 잘못되었습니다: $Address"
    }

    $bridgeHost = $trimmed.Substring(0, $separator)
    $port = 0
    if (-not [int]::TryParse($trimmed.Substring($separator + 1), [ref]$port) -or $port -lt 1 -or $port -gt 65535) {
        throw "bridge port가 잘못되었습니다: $Address"
    }

    return [PSCustomObject]@{
        Hostname = $bridgeHost
        Port = $port
    }
}

function Invoke-BridgeJsonRpc {
    param(
        [Parameter(Mandatory = $true)][string]$Hostname,
        [Parameter(Mandatory = $true)][int]$Port,
        [Parameter(Mandatory = $true)][string]$Method,
        [object]$Params,
        [int]$TimeoutSec = 10
    )

    $client = $null
    $stream = $null
    $writer = $null
    $reader = $null
    try {
        $client = [System.Net.Sockets.TcpClient]::new()
        $connectTask = $client.ConnectAsync($Hostname, $Port)
        if (-not $connectTask.Wait([TimeSpan]::FromSeconds($TimeoutSec))) {
            throw "bridge 연결 timeout: $Hostname`:$Port"
        }

        $stream = $client.GetStream()
        $stream.ReadTimeout = $TimeoutSec * 1000
        $stream.WriteTimeout = $TimeoutSec * 1000
        $writer = [System.IO.StreamWriter]::new($stream, [System.Text.UTF8Encoding]::new($false), 1024, $true)
        $writer.NewLine = "`n"
        $writer.AutoFlush = $true
        $reader = [System.IO.StreamReader]::new($stream, [System.Text.Encoding]::UTF8, $true, 1024, $true)

        $request = @{
            id = "bridge-" + [System.Guid]::NewGuid().ToString("N")
            method = $Method
        }
        if ($PSBoundParameters.ContainsKey("Params") -and $null -ne $Params) {
            $request.params = $Params
        }

        $writer.WriteLine(($request | ConvertTo-Json -Depth 10 -Compress))
        $line = $reader.ReadLine()
        if ([string]::IsNullOrWhiteSpace($line)) {
            throw "bridge 응답이 비어 있습니다."
        }

        $response = $line | ConvertFrom-Json
        if ($response.error -and -not [string]::IsNullOrWhiteSpace($response.error.message)) {
            throw "bridge $Method 실패: $($response.error.message)"
        }

        return $response.result
    } finally {
        if ($reader) { $reader.Dispose() }
        if ($writer) { $writer.Dispose() }
        if ($stream) { $stream.Dispose() }
        if ($client) { $client.Dispose() }
    }
}

function Wait-AgentReady {
    param(
        [Parameter(Mandatory = $true)][string]$HealthUrl,
        [Parameter(Mandatory = $true)][System.Diagnostics.Process]$Process,
        [int]$TimeoutSec = 60,
        [string]$StdErrLog = ""
    )

    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSec)
    do {
        if ($Process.HasExited) {
            $stderr = if ($StdErrLog -and (Test-Path $StdErrLog)) { Get-Content -Path $StdErrLog -Raw } else { "" }
            throw "agent가 준비되기 전에 종료되었습니다. exit=$($Process.ExitCode) $stderr"
        }

        try {
            return Invoke-RestMethod -Method Get -Uri $HealthUrl -TimeoutSec 2
        } catch {
            Start-Sleep -Milliseconds 1000
        }
    } while ([DateTime]::UtcNow -lt $deadline)

    throw "agent readiness timeout: $HealthUrl"
}

function Invoke-AgentJson {
    param(
        [Parameter(Mandatory = $true)][string]$Method,
        [Parameter(Mandatory = $true)][string]$Uri,
        [object]$Body
    )

    if ($PSBoundParameters.ContainsKey("Body")) {
        return Invoke-RestMethod -Method $Method -Uri $Uri -ContentType "application/json" -Body ($Body | ConvertTo-Json -Depth 10)
    }

    return Invoke-RestMethod -Method $Method -Uri $Uri
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

$repoRootResolved = (Resolve-Path $RepoRoot).Path
$goCommand = Resolve-RequiredCommand -Name "go" -Hint "Go toolchain이 필요합니다."
if ([string]::IsNullOrWhiteSpace($AgentBaseUrl)) {
    $AgentBaseUrl = Get-FreeLoopbackUrl
}

$bridge = Split-BridgeAddress -Address $BridgeAddress
$bridgeName = Invoke-BridgeJsonRpc -Hostname $bridge.Hostname -Port $bridge.Port -Method "name"
if ($bridgeName -ne "cursor-extension-bridge") {
    throw "unexpected bridge name: $bridgeName (extension host의 mock mode 또는 command mode bridge가 필요합니다)"
}
$null = Invoke-BridgeJsonRpc -Hostname $bridge.Hostname -Port $bridge.Port -Method "capabilities"

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("vibedeck-extension-smoke-" + [System.Guid]::NewGuid().ToString("N"))
$logsDir = Join-Path $tempRoot "logs"
[System.IO.Directory]::CreateDirectory($logsDir) | Out-Null
$stdoutLog = Join-Path $logsDir "agent.stdout.log"
$stderrLog = Join-Path $logsDir "agent.stderr.log"
$agentScriptPath = Join-Path $tempRoot "run-agent.cmd"
$goCacheDir = Join-Path $tempRoot "go-cache"
$goTmpDir = Join-Path $tempRoot "go-tmp"
[System.IO.Directory]::CreateDirectory($goCacheDir) | Out-Null
[System.IO.Directory]::CreateDirectory($goTmpDir) | Out-Null
$listenAddress = $AgentBaseUrl -replace "^https?://", ""

$scriptContent = @(
    "@echo off",
    ("set AGENT_ADDR={0}" -f $listenAddress),
    ("set GOCACHE={0}" -f $goCacheDir),
    ("set GOTMPDIR={0}" -f $goTmpDir),
    ("set CURSOR_BRIDGE_TCP_ADDR={0}" -f $BridgeAddress),
    ('"{0}" run ./cmd/agent 1>"{1}" 2>"{2}"' -f $goCommand.Source, $stdoutLog, $stderrLog)
) -join "`r`n"
[System.IO.File]::WriteAllText($agentScriptPath, $scriptContent + "`r`n", [System.Text.UTF8Encoding]::new($false))

$agentProcess = $null
try {
    $agentProcess = Start-Process -FilePath $agentScriptPath -WorkingDirectory $repoRootResolved -PassThru -WindowStyle Hidden
    $null = Wait-AgentReady -HealthUrl ($AgentBaseUrl.TrimEnd("/") + "/healthz") -Process $agentProcess -TimeoutSec $StartupTimeoutSec -StdErrLog $stderrLog

    $adapter = Invoke-AgentJson -Method GET -Uri ($AgentBaseUrl.TrimEnd("/") + "/v1/agent/runtime/adapter")
    if ($adapter.name -ne "cursor-extension-bridge") {
        throw "unexpected adapter name: $($adapter.name)"
    }

    $sid = "sid-extension-smoke"
    $promptResponse = Invoke-AgentJson -Method POST -Uri ($AgentBaseUrl.TrimEnd("/") + "/v1/agent/envelope") -Body (New-Envelope -Sid $sid -Rid "rid-prompt-1" -Seq 1 -Type "PROMPT_SUBMIT" -Payload @{
        prompt = "auth middleware 401 handling bug root cause를 설명하고 patch를 제안해줘"
        template = "smoke"
        contextOptions = @{
            includeActiveFile = $true
            includeSelection = $true
            includeLatestError = $true
            includeWorkspaceSummary = $true
        }
    })

    $promptAck = $promptResponse.responses | Where-Object { $_.type -eq "PROMPT_ACK" } | Select-Object -First 1
    $patchReady = $promptResponse.responses | Where-Object { $_.type -eq "PATCH_READY" } | Select-Object -First 1
    if (-not $promptAck) {
        throw "PROMPT_ACK response not found"
    }
    if (-not $patchReady) {
        throw "PATCH_READY response not found"
    }

    $jobId = $promptAck.payload.jobId
    if ([string]::IsNullOrWhiteSpace($jobId)) {
        throw "jobId missing from PROMPT_ACK"
    }

    $patchFiles = @($patchReady.payload.files)
    if ($patchFiles.Count -ne 1 -or $patchFiles[0].path -ne "src/auth/middleware.ts") {
        throw "unexpected patch files: $(($patchFiles | ConvertTo-Json -Depth 6 -Compress))"
    }
    $patchDiff = @($patchFiles[0].hunks | ForEach-Object { $_.diff }) -join "`n"
    if ($patchDiff -notmatch "return res\.status\(401\)\.send\(\)") {
        throw "smoke patch does not contain expected fix: $patchDiff"
    }

    $applyResponse = Invoke-AgentJson -Method POST -Uri ($AgentBaseUrl.TrimEnd("/") + "/v1/agent/envelope") -Body (New-Envelope -Sid $sid -Rid "rid-apply-1" -Seq 2 -Type "PATCH_APPLY" -Payload @{
        jobId = $jobId
        mode = "all"
    })
    $patchResult = $applyResponse.responses | Where-Object { $_.type -eq "PATCH_RESULT" } | Select-Object -First 1
    if (-not $patchResult) {
        throw "PATCH_RESULT response not found"
    }
    if ($patchResult.payload.status -ne "success") {
        throw "patch apply failed: $($patchResult.payload.status) $($patchResult.payload.message)"
    }

    $runResponse = Invoke-AgentJson -Method POST -Uri ($AgentBaseUrl.TrimEnd("/") + "/v1/agent/envelope") -Body (New-Envelope -Sid $sid -Rid "rid-run-1" -Seq 3 -Type "RUN_PROFILE" -Payload @{
        jobId = $jobId
        profileId = "test_all"
    })
    $runResult = $runResponse.responses | Where-Object { $_.type -eq "RUN_RESULT" } | Select-Object -First 1
    if (-not $runResult) {
        throw "RUN_RESULT response not found"
    }
    if (-not [string]::IsNullOrWhiteSpace($ExpectedRunStatus) -and $runResult.payload.status -ne $ExpectedRunStatus) {
        throw "unexpected run status: $($runResult.payload.status)"
    }

    $smokeSucceeded = $true
    [PSCustomObject]@{
        bridgeAddress = $BridgeAddress
        bridgeName = $bridgeName
        adapterName = $adapter.name
        adapterMode = $adapter.mode
        patchSummary = $patchReady.payload.summary
        patchFiles = @($patchFiles | ForEach-Object { $_.path })
        applyStatus = $patchResult.payload.status
        runStatus = $runResult.payload.status
        runSummary = $runResult.payload.summary
        topError = $runResult.payload.topErrors[0].message
        tempRoot = $tempRoot
    }
} finally {
    if ($agentProcess -and -not $agentProcess.HasExited) {
        Stop-Process -Id $agentProcess.Id -Force
        $agentProcess.WaitForExit()
    }
    if ($agentProcess) {
        try {
            $agentProcess.Dispose()
        } catch {
        }
        $agentProcess = $null
        Start-Sleep -Milliseconds 1000
    }
    if ($smokeSucceeded) {
        $global:LASTEXITCODE = 0
    }
    if (-not $KeepTempRoot -and (Test-Path $tempRoot)) {
        for ($attempt = 0; $attempt -lt 30; $attempt++) {
            try {
                Remove-Item -Path $tempRoot -Recurse -Force -ErrorAction Stop
                break
            } catch {
                if ($attempt -eq 29) {
                    Write-Warning "temp root cleanup skipped: $tempRoot / $($_.Exception.Message)"
                } else {
                    Start-Sleep -Milliseconds 1000
                }
            }
        }
    }
    if ($smokeSucceeded) {
        $global:LASTEXITCODE = 0
    }
}
