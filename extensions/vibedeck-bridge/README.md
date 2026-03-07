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
