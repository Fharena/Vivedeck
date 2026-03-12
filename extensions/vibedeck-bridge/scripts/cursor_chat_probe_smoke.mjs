import assert from "node:assert/strict";
import { createBridgeExtensionController } from "../dist/bridgeExtensionController.js";

class TabInputWebview {
  constructor(viewType) {
    this.viewType = viewType;
  }
}

class TabInputTerminal {}

const messages = {
  info: [],
  warn: [],
  error: [],
};
const commandRegistry = new Map([
  ["workbench.action.chat.open", async () => undefined],
  ["cursor.chat.focus", async () => undefined],
  ["cursor.composer.new", async () => undefined],
]);
const configurationListeners = new Set();
let clipboardText = "";

const activeGroup = {
  isActive: true,
  tabs: [
    {
      label: "Cursor Chat",
      isActive: true,
      input: new TabInputWebview("cursor.chat"),
    },
    {
      label: "Terminal",
      isActive: false,
      input: new TabInputTerminal(),
    },
  ],
};

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
    tabGroups: {
      all: [activeGroup],
      activeTabGroup: activeGroup,
    },
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
      throw new Error("webview panel should not be opened in chat probe smoke");
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
  chat: {
    createChatParticipant() {
      return { dispose() {} };
    },
  },
  lm: {
    async selectChatModels() {
      return [];
    },
    async invokeTool() {
      return { content: [] };
    },
    tools: [{ name: "workspace.search" }],
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

  const report = await fakeVscode.commands.executeCommand("vibedeckBridge.probeCursorChat");
  assert.equal(typeof report, "string");
  assert.match(report, /chat participant API: yes/);
  assert.match(report, /lm\.selectChatModels: yes/);
  assert.match(report, /Cursor Chat \| input=TabInputWebview/);
  assert.match(report, /native Cursor chat transcript direct access: no/);
  assert.match(report, /participant-owned history mirror: yes/);
  assert.match(report, /workbench\.action\.chat\.open/);
  assert.equal(clipboardText, report);
  assert.match(messages.warn.at(-1) ?? "", /기본 Cursor 채팅 transcript 직접 읽기/);

  console.log(
    JSON.stringify(
      {
        warning: messages.warn.at(-1) ?? "",
        clipboardSize: clipboardText.length,
        hasChatCommandHint: clipboardText.includes("cursor.chat.focus"),
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
