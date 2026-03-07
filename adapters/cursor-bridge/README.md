# Cursor Bridge (TypeScript)

이 패키지는 VibeDeck PC Agent가 호출하는 `WorkspaceAdapter` 계약의 TypeScript 기준 구현입니다.
Cursor/VS Code extension host 안에서 contract를 올리는 runtime helper와, Go agent가 붙을 수 있는 stdio/TCP bridge 서버를 함께 제공합니다.

## 포함 내용

- `WorkspaceAdapter` 인터페이스
- 공통 데이터 타입
- `MockCursorBridge` 구현
- `CursorCommandHost` 추상화
- `CursorExtensionBridge` 구현
- VS Code/Cursor extension host 어댑터
- extension activation용 runtime helper(`createCursorExtensionRuntime`, `serveCursorExtensionBridge`)
- stdio bridge 프로토콜/서버
- TCP socket bridge 서버(`serveSocketBridge`)
- 로컬 bootstrap용 fixture bridge
- 로컬 TCP fixture launcher(`npm run start:fixture:tcp`)

## 구현 원칙

- 코어 에이전트는 어댑터 구현 세부사항을 몰라야 함
- 패치 데이터는 파일/헝크 구조로 정규화
- 부분 적용 가능한 구조 유지
- Cursor 명령 결과는 브리지에서 `WorkspaceAdapter` 계약으로 정규화
- Go agent와 브리지 프로세스는 newline-delimited JSON RPC over stdio 또는 localhost TCP로 통신

## extension host에서 쓰는 방식

실제 Cursor/VS Code extension의 `activate()`에서는 runtime helper를 사용합니다.

```ts
import { createCursorExtensionRuntime } from "@vibedeck/cursor-bridge";
import * as vscode from "vscode";

export function activate() {
  const runtime = createCursorExtensionRuntime({
    vscode,
    adapter: actualAdapter,
  });

  return runtime;
}
```

- `adapter`는 실제 IDE 쪽 로직이 구현한 `WorkspaceAdapter`입니다.
- runtime helper는 `vibedeck.submitTask`, `vibedeck.getPatch`, `vibedeck.applyPatch`, `vibedeck.runProfile`, `vibedeck.getRunResult`를 등록합니다.
- 기본값으로 `vibedeck.getWorkspaceMetadata`, `vibedeck.getLatestTerminalError`도 함께 등록해서 `CursorExtensionBridge`가 컨텍스트를 채울 수 있게 합니다.
- `runProfile`/`getRunResult` 호출 결과를 바탕으로 마지막 실행 상태와 최근 에러를 메모리에 유지합니다.
- 같은 프로세스에서 stdio bridge까지 바로 열어야 하면 `serveCursorExtensionBridge()`를 사용할 수 있습니다.
- localhost TCP bridge가 필요하면 `serveSocketBridge()`로 외부 agent가 붙을 포트를 열 수 있습니다.

## agent child-process/TCP 연결

Go agent는 두 경로를 모두 지원합니다.

- 기본값: fixture child-process (`dist/fixtureBridgeMain.js`)
- 외부 TCP bridge: `CURSOR_BRIDGE_TCP_ADDR=127.0.0.1:7797`

TCP fixture smoke test는 다음으로 바로 띄울 수 있습니다.

```bash
npm install
npm run build
CURSOR_BRIDGE_TCP_PORT=7797 npm run start:fixture:tcp
```

## 로컬 검증

```bash
npm install
npm run check
npm run build
npm run start:fixture
npm run start:fixture:tcp
```

기본 open-location 동작은 `workspace.openTextDocument + window.showTextDocument`를 사용합니다.
확장 쪽에서 별도 커맨드가 필요하면 `commands.openLocation`을 지정해 override 할 수 있습니다.
