import assert from "node:assert/strict";
import { createBridgeExtensionController } from "../dist/bridgeExtensionController.js";
import { probeCursorPromptSubmitPath } from "../dist/cursorPromptSubmitProbe.js";

const messages = {
  info: [],
  warn: [],
  error: [],
};
const commandRegistry = new Map([
  ["cursor.startComposerPrompt", async () => undefined],
  ["workbench.action.chat.open", async () => undefined],
  ["cursor.chat.focus", async () => undefined],
  ["editor.action.clipboardPasteAction", async () => undefined],
]);
const configurationListeners = new Set();
let clipboardText = "";

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
      throw new Error("webview panel should not be opened in prompt submit smoke");
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
    },
  },
  statusBarAlignment: {
    left: 1,
  },
  viewColumn: {
    one: 1,
  },
};

const controller = createBridgeExtensionController(fakeVscode);
const context = {
  subscriptions: [],
};

try {
  await controller.activate(context);

  const report = await fakeVscode.commands.executeCommand("vibedeckBridge.probeCursorPromptSubmit");
  assert.equal(typeof report, "string");
  assert.match(report, /kind: direct_command/);
  assert.match(report, /cursor\.startComposerPrompt/);
  assert.match(report, /can automate submit: yes/);
  assert.equal(clipboardText, report);
  assert.match(messages.info.at(-1) ?? "", /가장 유력한 direct submit 경로/);

  const fallbackReport = await probeCursorPromptSubmitPath({
    commands: {
      async getCommands() {
        return ["cursor.composer.new", "editor.action.clipboardPasteAction"];
      },
    },
  });
  assert.equal(fallbackReport.recommendedStrategy.kind, "open_then_paste");
  assert.equal(fallbackReport.recommendedStrategy.canAutomateSubmit, false);
  assert.deepEqual(fallbackReport.recommendedStrategy.commandIds, [
    "cursor.composer.new",
    "editor.action.clipboardPasteAction",
  ]);

  console.log(
    JSON.stringify(
      {
        directSummary: messages.info.at(-1) ?? "",
        directClipboardSize: clipboardText.length,
        fallbackKind: fallbackReport.recommendedStrategy.kind,
        fallbackCommands: fallbackReport.recommendedStrategy.commandIds,
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
