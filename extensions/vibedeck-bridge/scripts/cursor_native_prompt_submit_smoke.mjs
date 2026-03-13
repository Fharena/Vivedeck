import assert from "node:assert/strict";
import { createBridgeExtensionController } from "../dist/bridgeExtensionController.js";
import { submitCursorNativePrompt } from "../dist/cursorNativePromptSubmit.js";

const commandRegistry = new Map();
const configurationListeners = new Set();
const directPrompts = [];
const directCommands = [];
const messages = { info: [], warn: [], error: [] };
let clipboardText = "keep-original";

commandRegistry.set("cursor.startComposerPrompt", async (prompt) => {
  directCommands.push("cursor.startComposerPrompt");
  directPrompts.push(prompt);
  return undefined;
});

const fakeVscode = {
  commands: {
    async executeCommand(command, ...args) {
      const handler = commandRegistry.get(command);
      if (!handler) {
        throw new Error("missing command: " + command);
      }
      return await handler(...args);
    },
    registerCommand(command, callback) {
      commandRegistry.set(command, callback);
      return {
        dispose() {
          commandRegistry.delete(command);
        },
      };
    },
    async getCommands() {
      return [...commandRegistry.keys()].sort();
    },
  },
  window: {
    async showTextDocument() {
      return undefined;
    },
    showInformationMessage(message) {
      messages.info.push(message);
      return message;
    },
    showWarningMessage(message) {
      messages.warn.push(message);
      return message;
    },
    showErrorMessage(message) {
      messages.error.push(message);
      return message;
    },
    async showInputBox() {
      return "fix auth middleware";
    },
    createStatusBarItem() {
      return {
        text: "",
        tooltip: undefined,
        command: undefined,
        show() {},
        dispose() {},
      };
    },
    createWebviewPanel() {
      throw new Error("webview panel should not be opened in native submit smoke");
    },
  },
  workspace: {
    workspaceFolders: [{ uri: { fsPath: "C:/workspace" } }],
    textDocuments: [],
    async openTextDocument() {
      return {
        fileName: "",
        getText() {
          return "";
        },
      };
    },
    getConfiguration(section = "") {
      return {
        get(key, defaultValue) {
          const qualifiedKey = section ? `${section}.${key}` : key;
          if (qualifiedKey === "vibedeckBridge.autoStart") {
            return false;
          }
          if (qualifiedKey === "vibedeckBridge.agent.autoStart") {
            return false;
          }
          return defaultValue;
        },
      };
    },
    onDidChangeConfiguration(listener) {
      configurationListeners.add(listener);
      return {
        dispose() {
          configurationListeners.delete(listener);
        },
      };
    },
  },
  env: {
    clipboard: {
      async writeText(value) {
        clipboardText = value;
      },
      async readText() {
        return clipboardText;
      },
    },
  },
  statusBarAlignment: { left: 1 },
  viewColumn: { one: 1 },
};

const controller = createBridgeExtensionController(fakeVscode);
const context = { subscriptions: [] };

try {
  await controller.activate(context);

  const result = await fakeVscode.commands.executeCommand("vibedeckBridge.submitCursorPrompt");
  assert.equal(result.status, "submitted");
  assert.equal(result.prompt, "fix auth middleware");
  assert.deepEqual(directPrompts, ["fix auth middleware"]);
  assert.deepEqual(directCommands, ["cursor.startComposerPrompt"]);
  assert.equal(clipboardText, "keep-original");
  assert.match(messages.info.at(-1) ?? "", /Cursor 기본 채팅에 프롬프트를 전송/);

  const fallbackCalls = [];
  let fallbackClipboard = "restore-me";
  let pastedDraft = "";
  const fallbackVscode = {
    commands: {
      async getCommands() {
        return [
          "composer.newAgentChat",
          "cursor.chat.focus",
          "editor.action.clipboardPasteAction",
        ];
      },
      async executeCommand(command, ...args) {
        fallbackCalls.push({ command, args });
        if (command === "editor.action.clipboardPasteAction") {
          pastedDraft = fallbackClipboard;
        }
        return undefined;
      },
    },
    env: {
      clipboard: {
        async writeText(value) {
          fallbackClipboard = value;
        },
        async readText() {
          return fallbackClipboard;
        },
      },
    },
  };

  const fallbackResult = await submitCursorNativePrompt(
    fallbackVscode,
    "mirror this prompt",
    {
      waitAfterOpenMs: 0,
      waitBeforeRestoreMs: 0,
    },
  );
  assert.equal(fallbackResult.status, "draft_inserted");
  assert.equal(pastedDraft, "mirror this prompt");
  assert.equal(fallbackClipboard, "restore-me");
  assert.deepEqual(
    fallbackCalls.map((item) => item.command),
    ["composer.newAgentChat", "cursor.chat.focus", "editor.action.clipboardPasteAction"],
  );

  console.log(
    JSON.stringify(
      {
        directStatus: result.status,
        directPrompt: result.prompt,
        fallbackStatus: fallbackResult.status,
        fallbackCommands: fallbackCalls.map((item) => item.command),
      },
      null,
      2,
    ),
  );
} finally {
  await controller.deactivate();
  for (const disposable of [...context.subscriptions].reverse()) {
    disposable.dispose();
  }
}
