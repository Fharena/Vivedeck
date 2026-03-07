import type { OpenLocationInput } from "./types.js";

export interface CursorActiveEditorSnapshot {
  filePath: string;
  selection?: string;
}

export interface CursorWorkspaceMetadata {
  changedFiles?: string[];
  lastRunProfile?: string;
  lastRunStatus?: string;
  latestTerminalError?: string;
}

export interface CursorCommandHost {
  getActiveEditor(): Promise<CursorActiveEditorSnapshot | null>;
  getChangedFiles(): Promise<string[]>;
  executeCommand<T = unknown>(command: string, ...args: unknown[]): Promise<T>;
  openLocation(input: OpenLocationInput): Promise<void>;
}

export interface CursorBridgeCommands {
  submitTask: string;
  getPatch: string;
  applyPatch: string;
  runProfile: string;
  getRunResult: string;
  openLocation?: string;
  getWorkspaceMetadata?: string;
  getLatestTerminalError?: string;
}
