import assert from "node:assert/strict";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { DatabaseSync } from "node:sqlite";
import { createBridgeExtensionController } from "../dist/bridgeExtensionController.js";
import { probeCursorChatStorage } from "../dist/cursorChatStorageProbe.js";

const tempRoot = await mkdtemp(path.join(os.tmpdir(), "vibedeck-cursor-storage-"));
const originalAppData = process.env.APPDATA;
process.env.APPDATA = path.join(tempRoot, "appdata");

try {
  const cursorUserRoot = path.join(process.env.APPDATA, "Cursor", "User");
  const globalStorageDir = path.join(cursorUserRoot, "globalStorage");
  const workspaceStorageDir = path.join(cursorUserRoot, "workspaceStorage");
  await mkdir(globalStorageDir, { recursive: true });
  await mkdir(path.join(workspaceStorageDir, "ws-a"), { recursive: true });
  await mkdir(path.join(workspaceStorageDir, "ws-b"), { recursive: true });
  await writeFile(path.join(workspaceStorageDir, "ws-a", "state.vscdb"), "", "utf8");
  await writeFile(path.join(workspaceStorageDir, "ws-b", "state.vscdb"), "", "utf8");

  const dbPath = path.join(globalStorageDir, "state.vscdb");
  const db = new DatabaseSync(dbPath);
  try {
    db.exec("CREATE TABLE ItemTable (key TEXT UNIQUE, value BLOB)");
    db.exec("CREATE TABLE cursorDiskKV (key TEXT UNIQUE, value BLOB)");

    const insert = db.prepare("INSERT INTO cursorDiskKV(key, value) VALUES (?, ?)");
    insert.run(
      "composerData:composer-1",
      JSON.stringify({
        _v: 3,
        composerId: "composer-1",
        createdAt: 1760000000000,
        status: "completed",
        isAgentic: true,
        fullConversationHeadersOnly: [
          { bubbleId: "bubble-user", type: 1 },
          { bubbleId: "bubble-assistant", type: 2 },
        ],
        conversationMap: {},
      }),
    );
    insert.run(
      "bubbleId:composer-1:bubble-user",
      JSON.stringify({
        _v: 3,
        type: 1,
        createdAt: "2026-03-12T00:00:00Z",
        text: "hello from mobile",
      }),
    );
    insert.run(
      "bubbleId:composer-1:bubble-assistant",
      JSON.stringify({
        _v: 3,
        type: 2,
        createdAt: "2026-03-12T00:00:01Z",
        text: "assistant reply from cursor",
      }),
    );
    insert.run(
      "messageRequestContext:composer-1:bubble-user",
      JSON.stringify({ terminalFiles: ["npm test"], todos: [] }),
    );
  } finally {
    db.close();
  }

  const messages = { info: [], warn: [], error: [] };
  const commandRegistry = new Map();
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
        throw new Error("webview panel should not be opened in storage probe smoke");
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
    statusBarAlignment: { left: 1 },
    viewColumn: { one: 1 },
  };

  const controller = createBridgeExtensionController(fakeVscode);
  const context = { subscriptions: [] };

  try {
    await controller.activate(context);

    const report = await fakeVscode.commands.executeCommand("vibedeckBridge.probeCursorChatStorage");
    assert.equal(typeof report, "string");
    assert.match(report, /backend: node_sqlite/);
    assert.match(report, /composerData rows: 1/);
    assert.match(report, /bubble rows: 2/);
    assert.match(report, /messageRequestContext rows: 1/);
    assert.match(report, /hello from mobile/);
    assert.match(report, /assistant reply from cursor/);
    assert.equal(clipboardText, report);
    assert.match(messages.info.at(-1) ?? "", /\uCC44\uD305 \uC800\uC7A5\uC18C \uC9C4\uB2E8/);

    const directReport = await probeCursorChatStorage({
      cursorUserRoot,
      maxConversations: 1,
    });
    assert.equal(directReport.backend, "node_sqlite");
    assert.equal(directReport.conversations.length, 1);
    assert.equal(directReport.workspaceDbCount, 2);
    assert.equal(directReport.conversations[0].firstUserText, "hello from mobile");

    console.log(
      JSON.stringify(
        {
          summary: messages.info.at(-1) ?? "",
          clipboardSize: clipboardText.length,
          workspaceDbCount: directReport.workspaceDbCount,
          firstComposer: directReport.conversations[0].composerId,
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
} finally {
  if (originalAppData === undefined) {
    delete process.env.APPDATA;
  } else {
    process.env.APPDATA = originalAppData;
  }
  await rm(tempRoot, { recursive: true, force: true });
}
