import assert from "node:assert/strict";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { DatabaseSync } from "node:sqlite";
import { createBridgeExtensionController } from "../dist/bridgeExtensionController.js";

const tempRoot = await mkdtemp(path.join(os.tmpdir(), "vibedeck-cursor-link-"));
const originalAppData = process.env.APPDATA;
process.env.APPDATA = path.join(tempRoot, "appdata");

try {
  const cursorUserRoot = path.join(process.env.APPDATA, "Cursor", "User");
  const globalStorageDir = path.join(cursorUserRoot, "globalStorage");
  const workspaceStorageDir = path.join(cursorUserRoot, "workspaceStorage");
  await mkdir(globalStorageDir, { recursive: true });
  await mkdir(path.join(workspaceStorageDir, "ws-a"), { recursive: true });
  await writeFile(path.join(workspaceStorageDir, "ws-a", "state.vscdb"), "", "utf8");

  const dbPath = path.join(globalStorageDir, "state.vscdb");
  const messages = { info: [], warn: [], error: [] };
  const commandRegistry = new Map();
  const configurationListeners = new Set();
  let clipboardText = "";
  let composerCount = 0;

  function withDb(run) {
    const db = new DatabaseSync(dbPath);
    try {
      return run(db);
    } finally {
      db.close();
    }
  }

  withDb((db) => {
    db.exec("CREATE TABLE ItemTable (key TEXT UNIQUE, value BLOB)");
    db.exec("CREATE TABLE cursorDiskKV (key TEXT UNIQUE, value BLOB)");
  });

  commandRegistry.set("cursor.startComposerPrompt", async (prompt) => {
    composerCount += 1;
    const composerId = `composer-${composerCount}`;
    const userBubbleId = `bubble-user-${composerCount}`;
    const assistantBubbleId = `bubble-assistant-${composerCount}`;
    const now = Date.now();

    withDb((db) => {
      const insert = db.prepare("INSERT INTO cursorDiskKV(key, value) VALUES (?, ?)");
      insert.run(
        `composerData:${composerId}`,
        JSON.stringify({
          _v: 3,
          composerId,
          createdAt: now,
          status: "streaming",
          isAgentic: true,
          fullConversationHeadersOnly: [
            { bubbleId: userBubbleId, type: 1 },
            { bubbleId: assistantBubbleId, type: 2 },
          ],
          conversationMap: {},
        }),
      );
      insert.run(
        `bubbleId:${composerId}:${userBubbleId}`,
        JSON.stringify({
          _v: 3,
          type: 1,
          createdAt: new Date(now).toISOString(),
          text: String(prompt),
        }),
      );
      insert.run(
        `bubbleId:${composerId}:${assistantBubbleId}`,
        JSON.stringify({
          _v: 3,
          type: 2,
          createdAt: new Date(now + 1000).toISOString(),
          text: "assistant is working on it",
        }),
      );
      insert.run(
        `messageRequestContext:${composerId}:${userBubbleId}`,
        JSON.stringify({ files: ["internal/agent/session_store.go"] }),
      );
    });

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
        throw new Error("webview panel should not be opened in chat link smoke");
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

    const submitResult = await fakeVscode.commands.executeCommand(
      "vibedeckBridge.submitCursorPrompt",
      {
        threadId: "thread-auth",
        prompt: "fix auth middleware",
      },
    );

    assert.equal(submitResult.status, "submitted");
    assert.equal(submitResult.threadId, "thread-auth");
    assert.equal(submitResult.linkState, "linked");
    assert.equal(submitResult.composerId, "composer-1");

    const report = await fakeVscode.commands.executeCommand(
      "vibedeckBridge.inspectCursorChatLinks",
    );
    assert.equal(typeof report, "string");
    assert.match(report, /thread-auth -> composer-1/);
    assert.match(report, /latest_user_text_exact|same_composer_latest_user_text_exact/);
    assert.match(report, /assistant is working on it/);
    assert.equal(clipboardText, report);
    assert.match(messages.info.join("\n"), /composer와 연결/);

    console.log(
      JSON.stringify(
        {
          submitStatus: submitResult.status,
          linkState: submitResult.linkState,
          composerId: submitResult.composerId,
          reportLength: report.length,
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
