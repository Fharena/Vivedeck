# Cursor Bridge (TypeScript)

이 패키지는 VibeDeck PC Agent가 호출하는 `WorkspaceAdapter` 계약의 TypeScript 기준 구현입니다.
Cursor Extension 명령을 감싸는 브리지와, Go agent가 붙을 수 있는 stdio bridge 서버를 함께 제공합니다.

## 포함 내용

- `WorkspaceAdapter` 인터페이스
- 공통 데이터 타입
- `MockCursorBridge` 구현
- `CursorCommandHost` 추상화
- `CursorExtensionBridge` 구현
- VS Code/Cursor extension host 어댑터
- stdio bridge 프로토콜/서버
- 로컬 bootstrap용 fixture bridge

## 구현 원칙

- 코어 에이전트는 어댑터 구현 세부사항을 몰라야 함
- 패치 데이터는 파일/헝크 구조로 정규화
- 부분 적용 가능한 구조 유지
- Cursor 명령 결과는 브리지에서 `WorkspaceAdapter` 계약으로 정규화
- Go agent와 브리지 프로세스는 newline-delimited JSON RPC over stdio로 통신

## 실제 연결 방식

Cursor extension 안에서는 `CursorExtensionBridge`와 `createVSCodeCursorHost`를 사용합니다.

```ts
import { CursorExtensionBridge, createVSCodeCursorHost } from "@vibedeck/cursor-bridge";
import * as vscode from "vscode";

const bridge = new CursorExtensionBridge({
  host: createVSCodeCursorHost(vscode),
  commands: {
    submitTask: "vibedeck.submitTask",
    getPatch: "vibedeck.getPatch",
    applyPatch: "vibedeck.applyPatch",
    runProfile: "vibedeck.runProfile",
    getRunResult: "vibedeck.getRunResult",
    getWorkspaceMetadata: "vibedeck.getWorkspaceMetadata",
    getLatestTerminalError: "vibedeck.getLatestTerminalError",
  },
});
```

Go agent와 stdio로 연결할 때는 `serveStdioBridge(adapter)`를 사용합니다.
저장소 기본값은 `npm run start:fixture`로 실행되는 fixture bridge이며, 실제 Cursor extension 런처는 다음 단계에서 이 자리에 들어갑니다.

## 로컬 검증

```bash
npm install
npm run check
npm run build
npm run start:fixture
```

기본 open-location 동작은 `workspace.openTextDocument + window.showTextDocument`를 사용합니다.
확장 쪽에서 별도 커맨드가 필요하면 `commands.openLocation`을 지정해 override 할 수 있습니다.

## 현재 범위와 다음 단계

- 이번 단계:
  - `CursorExtensionBridge`와 stdio bridge 서버 추가
  - Go agent 기본 런타임을 child-process bridge 호출로 전환
  - 로컬 bootstrap용 fixture bridge 추가
- 다음 단계:
  - Cursor extension 프로세스 안에서 `createVSCodeCursorHost` 기반 런처 추가
  - 모바일 direct 제어 경로와 실제 IDE 연동 통합 검증