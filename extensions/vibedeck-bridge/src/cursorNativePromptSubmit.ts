import {
  probeCursorPromptSubmitPath,
  type CursorPromptSubmitProbeVscodeLike,
} from "./cursorPromptSubmitProbe.js";

export interface CursorNativePromptSubmitVscodeLike extends CursorPromptSubmitProbeVscodeLike {
  readonly commands: CursorPromptSubmitProbeVscodeLike["commands"] & {
    executeCommand<T = unknown>(command: string, ...args: unknown[]): Promise<T>;
  };
  readonly env?: {
    clipboard?: {
      writeText(text: string): Promise<void>;
      readText?(): Promise<string>;
    };
  };
}

export type CursorNativePromptSubmitStatus =
  | "submitted"
  | "draft_inserted"
  | "manual_required"
  | "unavailable";

export interface CursorNativePromptSubmitResult {
  readonly checkedAt: string;
  readonly prompt: string;
  readonly status: CursorNativePromptSubmitStatus;
  readonly strategyKind: string;
  readonly commandIds: readonly string[];
  readonly executedCommands: readonly string[];
  readonly clipboardUsed: boolean;
  readonly clipboardRestored: boolean;
  readonly summary: string;
}

export interface CursorNativePromptSubmitOptions {
  readonly waitAfterOpenMs?: number;
  readonly waitBeforeRestoreMs?: number;
  readonly restoreClipboard?: boolean;
}

const DEFAULT_WAIT_AFTER_OPEN_MS = 120;
const DEFAULT_WAIT_BEFORE_RESTORE_MS = 80;

export async function submitCursorNativePrompt(
  vscodeLike: CursorNativePromptSubmitVscodeLike,
  prompt: string,
  options: CursorNativePromptSubmitOptions = {},
): Promise<CursorNativePromptSubmitResult> {
  const normalizedPrompt = prompt.trim();
  if (!normalizedPrompt) {
    throw new Error("prompt is required");
  }

  const report = await probeCursorPromptSubmitPath(vscodeLike);
  const strategy = report.recommendedStrategy;
  const checkedAt = new Date().toISOString();

  switch (strategy.kind) {
    case "direct_command":
      return await submitViaDirectCommand(vscodeLike, normalizedPrompt, checkedAt, strategy.commandIds);
    case "open_then_paste":
      return await insertDraftViaClipboard(vscodeLike, normalizedPrompt, checkedAt, strategy.commandIds, options);
    case "manual_only":
      return {
        checkedAt,
        prompt: normalizedPrompt,
        status: "manual_required",
        strategyKind: strategy.kind,
        commandIds: strategy.commandIds,
        executedCommands: [],
        clipboardUsed: false,
        clipboardRestored: false,
        summary:
          "자동 submit command는 확인되지 않았습니다. chat 표면만 열 수 있는 상태라 수동 전송이 필요합니다.",
      };
    default:
      return {
        checkedAt,
        prompt: normalizedPrompt,
        status: "unavailable",
        strategyKind: strategy.kind,
        commandIds: strategy.commandIds,
        executedCommands: [],
        clipboardUsed: false,
        clipboardRestored: false,
        summary:
          "현재 공개 command 목록에서는 Cursor 기본 채팅으로 프롬프트를 자동 전송할 경로를 찾지 못했습니다.",
      };
  }
}

async function submitViaDirectCommand(
  vscodeLike: CursorNativePromptSubmitVscodeLike,
  prompt: string,
  checkedAt: string,
  commandIds: readonly string[],
): Promise<CursorNativePromptSubmitResult> {
  const commandId = commandIds[0] ?? "cursor.startComposerPrompt";
  await vscodeLike.commands.executeCommand(commandId, prompt);
  return {
    checkedAt,
    prompt,
    status: "submitted",
    strategyKind: "direct_command",
    commandIds,
    executedCommands: [commandId],
    clipboardUsed: false,
    clipboardRestored: false,
    summary: `\`${commandId}\` 경로로 Cursor 기본 채팅에 프롬프트를 전송했습니다.`,
  };
}

async function insertDraftViaClipboard(
  vscodeLike: CursorNativePromptSubmitVscodeLike,
  prompt: string,
  checkedAt: string,
  commandIds: readonly string[],
  options: CursorNativePromptSubmitOptions,
): Promise<CursorNativePromptSubmitResult> {
  const openCommand = commandIds.find((commandId) =>
    commandId !== "cursor.chat.focus" && commandId !== "editor.action.clipboardPasteAction",
  );
  const focusCommand = commandIds.includes("cursor.chat.focus") ? "cursor.chat.focus" : undefined;
  const pasteCommand = commandIds.includes("editor.action.clipboardPasteAction")
    ? "editor.action.clipboardPasteAction"
    : undefined;
  const clipboard = vscodeLike.env?.clipboard;

  if (!openCommand || !pasteCommand || !clipboard?.writeText) {
    return {
      checkedAt,
      prompt,
      status: "manual_required",
      strategyKind: "open_then_paste",
      commandIds,
      executedCommands: [],
      clipboardUsed: false,
      clipboardRestored: false,
      summary:
        "chat 표면을 열 수는 있지만 clipboard fallback을 실행할 준비가 부족해 수동 전송이 필요합니다.",
    };
  }

  const restoreClipboard = options.restoreClipboard !== false;
  const waitAfterOpenMs = normalizeDelay(options.waitAfterOpenMs, DEFAULT_WAIT_AFTER_OPEN_MS);
  const waitBeforeRestoreMs = normalizeDelay(
    options.waitBeforeRestoreMs,
    DEFAULT_WAIT_BEFORE_RESTORE_MS,
  );
  const previousClipboard = restoreClipboard ? await safeReadClipboard(clipboard) : undefined;
  const executedCommands: string[] = [];
  let clipboardRestored = false;

  try {
    await clipboard.writeText(prompt);
    await vscodeLike.commands.executeCommand(openCommand);
    executedCommands.push(openCommand);
    if (focusCommand) {
      await vscodeLike.commands.executeCommand(focusCommand);
      executedCommands.push(focusCommand);
    }
    if (waitAfterOpenMs > 0) {
      await delay(waitAfterOpenMs);
    }
    await vscodeLike.commands.executeCommand(pasteCommand);
    executedCommands.push(pasteCommand);
  } finally {
    if (restoreClipboard && previousClipboard !== undefined) {
      if (waitBeforeRestoreMs > 0) {
        await delay(waitBeforeRestoreMs);
      }
      await clipboard.writeText(previousClipboard);
      clipboardRestored = true;
    }
  }

  return {
    checkedAt,
    prompt,
    status: "draft_inserted",
    strategyKind: "open_then_paste",
    commandIds,
    executedCommands,
    clipboardUsed: true,
    clipboardRestored,
    summary:
      "Cursor chat/composer를 열고 프롬프트 초안을 붙여넣었습니다. 자동 제출까지는 확정되지 않아 Enter 확인이 필요할 수 있습니다.",
  };
}

async function safeReadClipboard(clipboard: {
  readText?(): Promise<string>;
}): Promise<string | undefined> {
  if (!clipboard.readText) {
    return undefined;
  }
  try {
    return await clipboard.readText();
  } catch {
    return undefined;
  }
}

function normalizeDelay(value: number | undefined, fallback: number): number {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
    return fallback;
  }
  return Math.trunc(value);
}

async function delay(ms: number): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, ms));
}
