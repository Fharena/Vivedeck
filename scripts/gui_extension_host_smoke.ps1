param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [string]$Editor = "auto",
    [string]$BridgeHost = "127.0.0.1",
    [int]$BridgePort = 0,
    [string]$AgentBaseUrl = "",
    [string]$CursorAgentBin = $(if ($env:CURSOR_AGENT_BIN) { $env:CURSOR_AGENT_BIN } else { "cursor-agent" }),
    [string]$UseWslCursorAgent = $(if ($env:CURSOR_AGENT_USE_WSL) { $env:CURSOR_AGENT_USE_WSL } else { "" }),
    [string]$CursorAgentWslDistro = $(if ($env:CURSOR_AGENT_WSL_DISTRO) { $env:CURSOR_AGENT_WSL_DISTRO } else { "" }),
    [int]$EditorStartupTimeoutSec = 90,
    [int]$AgentStartupTimeoutSec = 60,
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
        return "http://127.0.0.1:$((([System.Net.IPEndPoint]$listener.LocalEndpoint).Port))"
    } finally {
        $listener.Stop()
    }
}

function Get-FreeTcpPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        return ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
    } finally {
        $listener.Stop()
    }
}

function Invoke-BridgeJsonRpc {
    param(
        [Parameter(Mandatory = $true)][string]$Hostname,
        [Parameter(Mandatory = $true)][int]$Port,
        [Parameter(Mandatory = $true)][string]$Method,
        [int]$TimeoutSec = 10
    )

    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $connectTask = $client.ConnectAsync($Hostname, $Port)
        if (-not $connectTask.Wait([TimeSpan]::FromSeconds($TimeoutSec))) {
            throw "bridge 연결 timeout: $Hostname`:$Port"
        }

        $stream = $client.GetStream()
        try {
            $stream.ReadTimeout = $TimeoutSec * 1000
            $stream.WriteTimeout = $TimeoutSec * 1000
            $writer = [System.IO.StreamWriter]::new($stream, [System.Text.UTF8Encoding]::new($false), 1024, $true)
            $reader = [System.IO.StreamReader]::new($stream, [System.Text.Encoding]::UTF8, $true, 1024, $true)
            try {
                $writer.NewLine = "`n"
                $writer.AutoFlush = $true
                $writer.WriteLine((@{ id = "bridge"; method = $Method } | ConvertTo-Json -Compress))
                $line = $reader.ReadLine()
                if ([string]::IsNullOrWhiteSpace($line)) {
                    throw "bridge 응답이 비어 있습니다."
                }
                $response = $line | ConvertFrom-Json
                if ($response.error -and $response.error.message) {
                    throw "bridge $Method 실패: $($response.error.message)"
                }
                return $response.result
            } finally {
                $reader.Dispose()
                $writer.Dispose()
            }
        } finally {
            $stream.Dispose()
        }
    } finally {
        $client.Dispose()
    }
}

function Wait-BridgeReady {
    param(
        [Parameter(Mandatory = $true)][string]$Hostname,
        [Parameter(Mandatory = $true)][int]$Port,
        [Parameter(Mandatory = $true)][System.Diagnostics.Process]$Process,
        [int]$TimeoutSec = 90
    )

    $lastError = ""
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSec)
    do {
        if ($Process.HasExited -and [string]::IsNullOrWhiteSpace($lastError)) {
            $lastError = "editor launcher exited: $($Process.ExitCode)"
        }
        try {
            $name = Invoke-BridgeJsonRpc -Hostname $Hostname -Port $Port -Method "name" -TimeoutSec 3
            if ($name -ne "cursor-extension-bridge") {
                throw "unexpected bridge name: $name"
            }
            $null = Invoke-BridgeJsonRpc -Hostname $Hostname -Port $Port -Method "capabilities" -TimeoutSec 3
            return $name
        } catch {
            $lastError = $_.Exception.Message
            Start-Sleep -Milliseconds 1000
        }
    } while ([DateTime]::UtcNow -lt $deadline)

    throw "editor bridge readiness timeout: ${Hostname}:$Port / $lastError"
}

function Stop-EditorProcessesForUserDataDir {
    param(
        [Parameter(Mandatory = $true)][string]$ExecutablePath,
        [Parameter(Mandatory = $true)][string]$UserDataDir
    )

    $processName = [System.IO.Path]::GetFileName($ExecutablePath)
    $processes = Get-CimInstance Win32_Process -Filter ("Name = '{0}'" -f $processName) -ErrorAction SilentlyContinue
    foreach ($process in @($processes)) {
        if ($process.CommandLine -and $process.CommandLine -like ("*" + $UserDataDir + "*")) {
            Stop-Process -Id $process.ProcessId -Force -ErrorAction SilentlyContinue
        }
    }
}

function Resolve-EditorExecutable {
    param([string]$RequestedEditor)

    $candidates = if ([string]::IsNullOrWhiteSpace($RequestedEditor) -or $RequestedEditor -eq "auto") { @("cursor", "code") } else { @($RequestedEditor) }
    foreach ($candidate in $candidates) {
        $command = Get-Command $candidate -ErrorAction SilentlyContinue
        if (-not $command) {
            continue
        }
        $source = $command.Source
        $leaf = [System.IO.Path]::GetFileName($source).ToLowerInvariant()
        if ($leaf -eq "cursor.cmd") {
            return [PSCustomObject]@{ Label = "Cursor"; Executable = (Resolve-Path (Join-Path (Split-Path $source -Parent) "..\..\..\Cursor.exe")).Path }
        }
        if ($leaf -eq "code.cmd") {
            return [PSCustomObject]@{ Label = "VS Code"; Executable = (Resolve-Path (Join-Path (Split-Path $source -Parent) "..\Code.exe")).Path }
        }
        if ($leaf -eq "cursor.exe") {
            return [PSCustomObject]@{ Label = "Cursor"; Executable = $source }
        }
        if ($leaf -eq "code.exe") {
            return [PSCustomObject]@{ Label = "VS Code"; Executable = $source }
        }
    }
    throw "editor 실행 파일을 찾지 못했습니다. -Editor cursor 또는 -Editor code 로 지정하세요."
}

$repoRootResolved = (Resolve-Path $RepoRoot).Path
$extensionPath = Join-Path $repoRootResolved "extensions\vibedeck-bridge"
$extensionSmokeScript = Join-Path $repoRootResolved "scripts\extension_host_smoke.ps1"
$gitCommand = Resolve-RequiredCommand -Name "git" -Hint "Git이 필요합니다."
$npmCommand = Resolve-RequiredCommand -Name "npm" -Hint "Node.js/npm이 필요합니다."
$editorInvocation = Resolve-EditorExecutable -RequestedEditor $Editor

if (-not $PSBoundParameters.ContainsKey("AgentBaseUrl")) {
    $AgentBaseUrl = Get-FreeLoopbackUrl
}
if ($BridgePort -le 0) {
    $BridgePort = Get-FreeTcpPort
}
$useWslCursorAgentValue = if ([string]::IsNullOrWhiteSpace($UseWslCursorAgent)) { $false } else { @("1", "true", "yes", "on") -contains $UseWslCursorAgent.Trim().ToLowerInvariant() }
if (-not $useWslCursorAgentValue) {
    $null = Resolve-RequiredCommand -Name $CursorAgentBin -Hint "네이티브 cursor-agent가 없으면 -UseWslCursorAgent true 와 -CursorAgentBin /home/.../cursor-agent 를 지정하세요."
} else {
    $null = Resolve-RequiredCommand -Name "wsl.exe" -Hint "WSL이 필요합니다."
}

$bridgeAddress = "{0}:{1}" -f $BridgeHost, $BridgePort
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("vibedeck-gui-extension-smoke-" + [System.Guid]::NewGuid().ToString("N"))
$workspaceRoot = Join-Path $tempRoot "workspace"
$settingsDir = Join-Path $workspaceRoot ".vscode"
$settingsPath = Join-Path $settingsDir "settings.json"
$userDataDir = Join-Path $tempRoot "user-data"
$extensionsDir = Join-Path $tempRoot "extensions"
$extensionLaunchPath = Join-Path $tempRoot "extension-dev"
$cursorAgentTempRoot = Join-Path $tempRoot "cursor-agent-temp"
$profilesPath = Join-Path $tempRoot "run-profiles.json"

[System.IO.Directory]::CreateDirectory($workspaceRoot) | Out-Null
[System.IO.Directory]::CreateDirectory($settingsDir) | Out-Null
[System.IO.Directory]::CreateDirectory($userDataDir) | Out-Null
[System.IO.Directory]::CreateDirectory($extensionsDir) | Out-Null
[System.IO.Directory]::CreateDirectory($cursorAgentTempRoot) | Out-Null

Push-Location $workspaceRoot
try {
    & $gitCommand.Source init | Out-Null
    & $gitCommand.Source config user.name "VibeDeck Smoke" | Out-Null
    & $gitCommand.Source config user.email "vibedeck-smoke@example.local" | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $workspaceRoot "notes.txt"), "base", [System.Text.UTF8Encoding]::new($false))
    & $gitCommand.Source add -A | Out-Null
    & $gitCommand.Source commit -m "base" | Out-Null
} finally {
    Pop-Location
}

$profilesJson = @{ smoke = @{ label = "Smoke"; command = "git status --short"; scope = "SMALL" } } | ConvertTo-Json -Depth 5
[System.IO.File]::WriteAllText($profilesPath, $profilesJson, [System.Text.UTF8Encoding]::new($false))

$settings = [ordered]@{
    "vibedeckBridge.autoStart" = $true
    "vibedeckBridge.mode" = "command"
    "vibedeckBridge.commandProvider" = "builtin_cursor_agent"
    "vibedeckBridge.tcpHost" = $BridgeHost
    "vibedeckBridge.tcpPort" = $BridgePort
    "vibedeckBridge.cursorAgent.workspaceRoot" = $workspaceRoot
    "vibedeckBridge.cursorAgent.tempRoot" = $cursorAgentTempRoot
    "vibedeckBridge.cursorAgent.gitBin" = $gitCommand.Source
    "vibedeckBridge.cursorAgent.bin" = $CursorAgentBin
    "vibedeckBridge.cursorAgent.useWsl" = $useWslCursorAgentValue
    "vibedeckBridge.cursorAgent.wslDistro" = $CursorAgentWslDistro
    "vibedeckBridge.cursorAgent.trustWorkspace" = $true
    "vibedeckBridge.cursorAgent.model" = "auto"
    "vibedeckBridge.cursorAgent.promptTimeoutMs" = 300000
    "vibedeckBridge.cursorAgent.runTimeoutMs" = 300000
}
[System.IO.File]::WriteAllText($settingsPath, ($settings | ConvertTo-Json -Depth 5), [System.Text.UTF8Encoding]::new($false))

Push-Location $repoRootResolved
try {
    & $npmCommand.Source "--prefix" $extensionPath "run" "build"
    if ($LASTEXITCODE -ne 0) {
        throw "extension build failed: exit $LASTEXITCODE"
    }
} finally {
    Pop-Location
}

New-Item -ItemType Junction -Path $extensionLaunchPath -Target $extensionPath -Force | Out-Null

$editorArguments = @("--new-window", "--user-data-dir", $userDataDir, "--extensions-dir", $extensionsDir, "--extensionDevelopmentPath", $extensionLaunchPath, "--disable-gpu", "--suppress-popups-on-startup", $workspaceRoot)
$editorProcess = $null
$smokeSucceeded = $false
try {
    $editorProcess = Start-Process -FilePath $editorInvocation.Executable -ArgumentList $editorArguments -PassThru
    $bridgeName = Wait-BridgeReady -Hostname $BridgeHost -Port $BridgePort -Process $editorProcess -TimeoutSec $EditorStartupTimeoutSec

    $smokeResult = & $extensionSmokeScript `
        -RepoRoot $repoRootResolved `
        -BridgeAddress $bridgeAddress `
        -AgentBaseUrl $AgentBaseUrl `
        -Prompt 'Modify only notes.txt. Make the final contents exactly two lines: "base" and "smoke-agent". Do not modify any other files or add explanations.' `
        -Template "smoke" `
        -ExpectedPatchPath "notes.txt" `
        -ExpectedDiffPattern "smoke-agent" `
        -RunProfileId "smoke" `
        -ExpectedRunStatus "passed" `
        -RunProfileFile $profilesPath `
        -IncludeActiveFile:$false `
        -IncludeSelection:$false `
        -IncludeLatestError:$false `
        -IncludeWorkspaceSummary:$true `
        -StartupTimeoutSec $AgentStartupTimeoutSec `
        -KeepTempRoot:$KeepTempRoot.IsPresent

    $notesContent = Get-Content -Path (Join-Path $workspaceRoot "notes.txt") -Raw
    $normalizedNotes = (($notesContent -replace "`r`n?", "`n")).TrimEnd("`n")
    if ($normalizedNotes -ne "base`nsmoke-agent") {
        throw "unexpected notes.txt content: $notesContent"
    }

    $smokeSucceeded = $true
    [PSCustomObject]@{
        editor = $editorInvocation.Label
        bridgeAddress = $bridgeAddress
        bridgeName = $bridgeName
        adapterName = $smokeResult.adapterName
        adapterMode = $smokeResult.adapterMode
        patchSummary = $smokeResult.patchSummary
        patchFiles = $smokeResult.patchFiles
        applyStatus = $smokeResult.applyStatus
        runStatus = $smokeResult.runStatus
        runSummary = $smokeResult.runSummary
        notesContent = $normalizedNotes
        workspaceRoot = $workspaceRoot
        tempRoot = $tempRoot
    }
} finally {
    if ($editorProcess -and -not $editorProcess.HasExited) {
        Stop-Process -Id $editorProcess.Id -Force
        $editorProcess.WaitForExit()
    }
    if ($editorProcess) {
        try { $editorProcess.Dispose() } catch {}
        Start-Sleep -Milliseconds 1500
    }
    Stop-EditorProcessesForUserDataDir -ExecutablePath $editorInvocation.Executable -UserDataDir $userDataDir
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



