import assert from "node:assert/strict";
import http from "node:http";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { DatabaseSync } from "node:sqlite";
import { createBridgeExtensionController } from "../dist/bridgeExtensionController.js";

const tempRoot = await mkdtemp(path.join(os.tmpdir(), "vibedeck-cursor-ingest-"));
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
  const db = new DatabaseSync(dbPath);
  db.exec("CREATE TABLE ItemTable (key TEXT UNIQUE, value BLOB)");
  db.exec("CREATE TABLE cursorDiskKV (key TEXT UNIQUE, value BLOB)");

  let activeComposerId = "";
  const capturedBatches = [];
  const messages = { info: [], warn: [], error: [] };
  const commandRegistry = new Map();
  const configurationListeners = new Set();
  let clipboardText = "";

  const server = http.createServer(async (req, res) => {
    if (req.method === "POST" && req.url === "/v1/agent/sessions/thread-auth/events") {
      const chunks = [];
      for await (const chunk of req) {
        chunks.push(chunk);
      }
      const body = JSON.parse(Buffer.concat(chunks).toString("utf8"));
      capturedBatches.push(body.events ?? []);
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          session: { id: "thread-auth", threadId: "thread-auth", title: "Auth thread" },
          timeline: [],
          liveState: {},
          operationState: {},
        }),
      );
      return;
    }

    res.writeHead(404, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ error: "not found" }));
  });

  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  const port = typeof address === "object" && address ? address.port : 0;
  const agentBaseUrl = `http://127.0.0.1:${port}`;

  function insertCursorConversation(prompt) {
    const composerId = "composer-1";
    activeComposerId = composerId;
    const createdAt = Date.now();
    const userBubbleId = "bubble-user-1";
    const assistantBubbleId = "bubble-assistant-1";
    const insert = db.prepare("INSERT OR REPLACE INTO cursorDiskKV(key, value) VALUES (?, ?)");
    insert.run(
      `composerData:${composerId}`,
      JSON.stringify({
        _v: 3,
        composerId,
        createdAt,
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
        createdAt: new Date(createdAt).toISOString(),
        text: String(prompt),
      }),
    );
    insert.run(
      `bubbleId:${composerId}:${assistantBubbleId}`,
      JSON.stringify({
        _v: 3,
        type: 2,
        createdAt: new Date(createdAt + 1000).toISOString(),
        text: "assistant first reply",
      }),
    );
    insert.run(
      `messageRequestContext:${composerId}:${userBubbleId}`,
      JSON.stringify({ files: ["internal/agent/session_store.go"], terminalCommands: ["go test ./internal/agent"] }),
    );
  }

  function appendAssistantBubble(text) {
    const composerId = activeComposerId;
    const composerRow = db
      .prepare("SELECT value FROM cursorDiskKV WHERE key = ?")
      .get(`composerData:${composerId}`);
    const composer = JSON.parse(String(composerRow.value));
    const bubbleId = `bubble-assistant-${composer.fullConversationHeadersOnly.length}`;
    const createdAt = Date.now();
    composer.fullConversationHeadersOnly.push({ bubbleId, type: 2 });
    db.prepare("INSERT OR REPLACE INTO cursorDiskKV(key, value) VALUES (?, ?)").run(
      `composerData:${composerId}`,
      JSON.stringify(composer),
    );
    db.prepare("INSERT OR REPLACE INTO cursorDiskKV(key, value) VALUES (?, ?)").run(
      `bubbleId:${composerId}:${bubbleId}`,
      JSON.stringify({
        _v: 3,
        type: 2,
        createdAt: new Date(createdAt).toISOString(),
        text,
      }),
    );
  }

  commandRegistry.set("cursor.startComposerPrompt", async (prompt) => {
    insertCursorConversation(prompt);
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
        throw new Error("webview panel should not be opened in chat ingest smoke");
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
            if (qualifiedKey === "vibedeckBridge.agentBaseUrl") {
              return agentBaseUrl;
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
    assert.equal(submitResult.linkState, "linked");

    assert.ok(capturedBatches.length >= 1, "mirror should append at least one batch");
    const firstBatch = capturedBatches.flat();
    assert.ok(firstBatch.some((event) => event.kind === "provider_message" && event.role === "user"));
    assert.ok(firstBatch.some((event) => event.kind === "provider_message" && event.role === "assistant"));
    assert.ok(firstBatch.some((event) => event.kind === "tool_activity"));

    appendAssistantBubble("assistant follow-up reply");
    await waitFor(() =>
      capturedBatches.flat().some((event) => event.body === "assistant follow-up reply"),
    );

    console.log(
      JSON.stringify(
        {
          submitStatus: submitResult.status,
          linkState: submitResult.linkState,
          batches: capturedBatches.length,
          totalEvents: capturedBatches.flat().length,
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
    await new Promise((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
    db.close();
  }
} finally {
  if (originalAppData === undefined) {
    delete process.env.APPDATA;
  } else {
    process.env.APPDATA = originalAppData;
  }
  await rm(tempRoot, { recursive: true, force: true });
}

async function waitFor(check, timeoutMs = 5000, intervalMs = 150) {
  const startedAt = Date.now();
  while (Date.now() - startedAt <= timeoutMs) {
    if (check()) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
  }
  throw new Error("timed out waiting for mirrored events");
}
