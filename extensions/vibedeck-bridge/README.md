# VibeDeck Bridge Extension

이 패키지는 Cursor/VS Code extension host 안에서 localhost TCP bridge를 열어 VibeDeck agent가 실제 editor 프로세스에 붙을 수 있게 합니다.

## 모드

- `command`: 설정된 command ID를 호출해 `submitTask/getPatch/applyPatch/runProfile/getRunResult`를 위임합니다.
- `mock`: 저장소의 `MockCursorBridge`를 사용해 smoke test를 빠르게 확인합니다.

## 빠른 smoke test

1. 이 extension을 Cursor/VS Code에 로드
2. 설정에서 `vibedeckBridge.mode=mock`
3. bridge 시작 확인
4. agent 실행 전에 다음 환경변수 설정

```powershell
$env:CURSOR_BRIDGE_TCP_ADDR = "127.0.0.1:7797"
go run ./cmd/agent
```

## command mode

실제 Cursor/VS Code 명령이 있으면 `vibedeckBridge.commands.*` 설정에 command ID를 넣어 연결합니다.
기본값은 `vibedeck.*` 이름을 사용합니다.

### 시작 전 검증

- `VibeDeck: Validate Commands` 명령으로 현재 extension host에 등록된 command ID를 검사할 수 있습니다.
- `submitTask/getPatch/applyPatch/runProfile/getRunResult`가 없으면 command mode는 시작하지 않습니다.
- `openLocation/getWorkspaceMetadata/getLatestTerminalError`는 optional이라서 누락 시 경고만 표시합니다.

### Agent 연결

- `VibeDeck: Copy Agent Env` 명령은 현재 bridge 주소 기준으로 PowerShell 환경변수 문자열을 클립보드에 복사합니다.
- 복사된 값을 agent 실행 터미널에 붙여 넣으면 TCP bridge로 직접 연결됩니다.

```powershell
$env:CURSOR_BRIDGE_TCP_ADDR = "127.0.0.1:7797"
go run ./cmd/agent
```

### 현재 한계

- 이 패키지는 "등록된 command ID를 브리지에 연결"하는 계층입니다.
- 저장소에 실제 Cursor AI task command 매핑이 포함되어 있지는 않습니다.
- 그래서 바로 실사용하려면, 사용하는 Cursor/VS Code 환경에서 해당 command ID를 제공하거나 별도 어댑터를 추가해야 합니다.
