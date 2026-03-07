import type { CursorBridgeCommands, CursorWorkspaceMetadata } from "./cursorHost.js";
import {
  CursorExtensionBridge,
  defaultCursorBridgeCommands,
} from "./cursorExtensionBridge.js";
import { serveStdioBridge } from "./stdioBridgeServer.js";
import type {
  ApplyPatchInput,
  OpenLocationInput,
  RunProfileInput,
  RunResult,
  SubmitTaskInput,
  WorkspaceAdapter,
} from "./types.js";
import { createVSCodeCursorHost, type VSCodeLike } from "./vscodeCursorHost.js";

export interface DisposableLike {
  dispose(): void;
}

export interface VSCodeExtensionCommandsLike {
  executeCommand<T = unknown>(command: string, ...args: unknown[]): Promise<T>;
  registerCommand(
    command: string,
    callback: (...args: unknown[]) => unknown,
  ): DisposableLike;
}

export interface VSCodeExtensionLike extends Omit<VSCodeLike, "commands"> {
  commands: VSCodeExtensionCommandsLike;
}

export interface CursorExtensionRuntimeOptions {
  vscode: VSCodeExtensionLike;
  adapter: WorkspaceAdapter;
  commands?: Partial<CursorBridgeCommands>;
}

export interface CursorExtensionRuntime {
  bridge: CursorExtensionBridge;
  commands: CursorBridgeCommands;
  dispose(): void;
}

interface CursorExtensionRuntimeState {
  lastRunProfile?: string;
  lastRunStatus?: string;
  latestTerminalError?: string;
}

export function createCursorExtensionRuntime(
  options: CursorExtensionRuntimeOptions,
): CursorExtensionRuntime {
  const commands = resolveCursorBridgeCommands(options.commands);
  const host = createVSCodeCursorHost(options.vscode);
  const state: CursorExtensionRuntimeState = {};
  const disposables: DisposableLike[] = [];

  disposables.push(
    options.vscode.commands.registerCommand(commands.submitTask, async (input) =>
      options.adapter.submitTask(asObjectParam<SubmitTaskInput>(input, commands.submitTask)),
    ),
  );
  disposables.push(
    options.vscode.commands.registerCommand(commands.getPatch, async (taskId) =>
      options.adapter.getPatch(readRequiredString(taskId, `${commands.getPatch}.taskId`)),
    ),
  );
  disposables.push(
    options.vscode.commands.registerCommand(commands.applyPatch, async (input) =>
      options.adapter.applyPatch(asObjectParam<ApplyPatchInput>(input, commands.applyPatch)),
    ),
  );
  disposables.push(
    options.vscode.commands.registerCommand(commands.runProfile, async (input) => {
      const params = asObjectParam<RunProfileInput>(input, commands.runProfile);
      state.lastRunProfile = params.profileId;
      state.lastRunStatus = "running";
      return options.adapter.runProfile(params);
    }),
  );
  disposables.push(
    options.vscode.commands.registerCommand(commands.getRunResult, async (runId) => {
      const result = await options.adapter.getRunResult(
        readRequiredString(runId, `${commands.getRunResult}.runId`),
      );
      updateRuntimeStateFromRunResult(state, result);
      return result;
    }),
  );

  if (commands.openLocation) {
    disposables.push(
      options.vscode.commands.registerCommand(commands.openLocation, async (input) =>
        options.adapter.openLocation(
          asObjectParam<OpenLocationInput>(input, commands.openLocation!),
        ),
      ),
    );
  }

  if (commands.getWorkspaceMetadata) {
    disposables.push(
      options.vscode.commands.registerCommand(
        commands.getWorkspaceMetadata,
        async (_input?: unknown): Promise<CursorWorkspaceMetadata> => ({
          changedFiles: await host.getChangedFiles(),
          lastRunProfile: state.lastRunProfile,
          lastRunStatus: state.lastRunStatus,
          latestTerminalError: state.latestTerminalError,
        }),
      ),
    );
  }

  if (commands.getLatestTerminalError) {
    disposables.push(
      options.vscode.commands.registerCommand(
        commands.getLatestTerminalError,
        async (_input?: unknown): Promise<string | undefined> => state.latestTerminalError,
      ),
    );
  }

  const bridge = new CursorExtensionBridge({
    host,
    commands,
    capabilities: options.adapter.capabilities(),
  });

  return {
    bridge,
    commands,
    dispose(): void {
      for (const disposable of [...disposables].reverse()) {
        disposable.dispose();
      }
    },
  };
}

export function serveCursorExtensionBridge(
  options: CursorExtensionRuntimeOptions,
): CursorExtensionRuntime {
  const runtime = createCursorExtensionRuntime(options);
  serveStdioBridge(runtime.bridge);
  return runtime;
}

function resolveCursorBridgeCommands(
  commands?: Partial<CursorBridgeCommands>,
): CursorBridgeCommands {
  return {
    ...defaultCursorBridgeCommands,
    ...commands,
  };
}

function updateRuntimeStateFromRunResult(
  state: CursorExtensionRuntimeState,
  result: RunResult | null,
): void {
  if (!result) {
    return;
  }

  state.lastRunProfile = result.profileId;
  state.lastRunStatus = result.status;
  state.latestTerminalError = extractLatestTerminalError(result);
}

function extractLatestTerminalError(result: RunResult): string | undefined {
  const topError = result.topErrors[0]?.message;
  if (topError && topError.length > 0) {
    return topError;
  }
  if (result.excerpt && result.excerpt.length > 0) {
    return result.excerpt;
  }
  if (
    (result.status === "failed" || result.status === "partial") &&
    result.summary.length > 0
  ) {
    return result.summary;
  }
  return undefined;
}

function asObjectParam<T>(value: unknown, label: string): T {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`invalid ${label} params`);
  }
  return value as T;
}

function readRequiredString(value: unknown, label: string): string {
  if (typeof value !== "string" || value.length === 0) {
    throw new Error(`invalid ${label}`);
  }
  return value;
}
