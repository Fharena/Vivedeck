# Cursor Bridge (TypeScript)

이 패키지는 VibeDeck PC Agent가 호출하는 `WorkspaceAdapter` 계약의 TypeScript 기준 구현입니다.
이제 Mock 외에 Cursor Extension 명령을 실제로 감싸는 브리지 구현도 함께 제공합니다.

## 포함 내용

- `WorkspaceAdapter` 인터페이스
- 공통 데이터 타입
- `MockCursorBridge` 구현
- `CursorCommandHost` 추상화
- `CursorExtensionBridge` 구현
- VS Code/Cursor extension host 어댑터

## 구현 원칙

- 코어 에이전트는 어댑터 구현 세부사항을 몰라야 함
- 패치 데이터는 파일/헝크 구조로 정규화
- 부분 적용 가능한 구조 유지
- Cursor 명령 결과는 브리지에서 `WorkspaceAdapter` 계약으로 정규화

## 실제 연결 방식

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

기본 open-location 동작은 `workspace.openTextDocument + window.showTextDocument`를 사용합니다.
확장 쪽에서 별도 커맨드가 필요하면 `commands.openLocation`을 지정해 override 할 수 있습니다.

## 현재 범위와 다음 단계

- 이번 단계: TypeScript 브리지 패키지에서 Cursor 명령/에디터 상태를 `WorkspaceAdapter` 계약으로 변환
- 다음 단계: Go agent에서 `MockAdapter` 대신 이 브리지를 프로세스 또는 RPC로 연결
