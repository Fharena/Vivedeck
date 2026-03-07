import type { CursorActiveEditorSnapshot, CursorCommandHost } from "./cursorHost.js";
import type { OpenLocationInput } from "./types.js";

interface PositionLike {
  line: number;
  character: number;
}

interface SelectionLike {
  start: PositionLike;
  end: PositionLike;
}

interface TextDocumentLike {
  fileName: string;
  isDirty?: boolean;
  getText(range?: unknown): string;
}

interface TextEditorLike {
  document: TextDocumentLike;
  selection?: SelectionLike | null;
}

interface TextDocumentShowOptionsLike {
  preview?: boolean;
  preserveFocus?: boolean;
  selection?: SelectionLike;
}

interface VSCodeWindowLike {
  activeTextEditor?: TextEditorLike;
  showTextDocument(
    document: TextDocumentLike,
    options?: TextDocumentShowOptionsLike,
  ): Promise<unknown>;
}

interface VSCodeCommandsLike {
  executeCommand<T = unknown>(command: string, ...args: unknown[]): Promise<T>;
}

interface VSCodeWorkspaceLike {
  textDocuments?: TextDocumentLike[];
  openTextDocument(path: string): Promise<TextDocumentLike>;
}

export interface VSCodeLike {
  window: VSCodeWindowLike;
  commands: VSCodeCommandsLike;
  workspace: VSCodeWorkspaceLike;
}

export function createVSCodeCursorHost(vscodeLike: VSCodeLike): CursorCommandHost {
  return {
    async getActiveEditor(): Promise<CursorActiveEditorSnapshot | null> {
      const editor = vscodeLike.window.activeTextEditor;
      if (!editor) {
        return null;
      }

      const selection = readSelection(editor);
      return selection
        ? {
            filePath: editor.document.fileName,
            selection,
          }
        : {
            filePath: editor.document.fileName,
          };
    },

    async getChangedFiles(): Promise<string[]> {
      const changedFiles =
        vscodeLike.workspace.textDocuments
          ?.filter((document) => document.isDirty)
          .map((document) => document.fileName) ?? [];

      return [...new Set(changedFiles)];
    },

    async executeCommand<T = unknown>(command: string, ...args: unknown[]): Promise<T> {
      return vscodeLike.commands.executeCommand<T>(command, ...args);
    },

    async openLocation(input: OpenLocationInput): Promise<void> {
      const document = await vscodeLike.workspace.openTextDocument(input.path);
      const point = {
        line: toZeroBased(input.line),
        character: toZeroBased(input.column ?? 1),
      };

      await vscodeLike.window.showTextDocument(document, {
        preview: false,
        preserveFocus: false,
        selection: {
          start: point,
          end: point,
        },
      });
    },
  };
}

function readSelection(editor: TextEditorLike): string | undefined {
  if (!editor.selection) {
    return undefined;
  }

  const text = editor.document.getText(editor.selection);
  return text.length > 0 ? text : undefined;
}

function toZeroBased(value: number): number {
  return Math.max(0, Math.trunc(value) - 1);
}
