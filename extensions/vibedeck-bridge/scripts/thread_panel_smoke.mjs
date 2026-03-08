import assert from "node:assert/strict";
import http from "node:http";
import { createBridgeExtensionController } from "../dist/bridgeExtensionController.js";

const state = {
  adapter: {
    name: "cursor_agent_cli",
    mode: "cursor_agent_cli",
    ready: true,
    workspaceRoot: "C:/demo/workspace",
    binaryPath: "/home/demo/.local/bin/cursor-agent",
    notes: ["shared thread smoke"],
  },
  runProfiles: [
    {
      id: "smoke",
      label: "Smoke",
      command: "git status --short",
      scope: "SMALL",
      optional: false,
    },
  ],
  threads: [],
  details: new Map(),
  envelopes: [],
  openLocations: [],
};

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url || "/", "http://127.0.0.1");
  if (req.method === "GET" && url.pathname === "/v1/agent/runtime/adapter") {
    return json(res, 200, state.adapter);
  }
  if (req.method === "GET" && url.pathname === "/v1/agent/run-profiles") {
    return json(res, 200, { profiles: state.runProfiles });
  }
  if (req.method === "GET" && url.pathname === "/v1/agent/threads") {
    return json(res, 200, { threads: state.threads });
  }
  if (req.method === "GET" && url.pathname.startsWith("/v1/agent/threads/")) {
    const threadId = decodeURIComponent(url.pathname.slice("/v1/agent/threads/".length));
    if (!state.details.has(threadId)) {
      return json(res, 404, { error: "thread not found" });
    }
    return json(res, 200, state.details.get(threadId));
  }
  if (req.method === "POST" && url.pathname === "/v1/agent/envelope") {
    const body = await readJson(req);
    state.envelopes.push(body);
    if (body.type === "PROMPT_SUBMIT") {
      const threadId = "thread_panel_smoke";
      const jobId = "job_panel_smoke";
      const prompt = body.payload?.prompt || "";
      const detail = {
        thread: {
          id: threadId,
          title: prompt.split("\n")[0] || "새 스레드",
          sessionId: body.sid,
          state: "patch_ready",
          currentJobId: jobId,
          lastEventKind: "patch_ready",
          lastEventText: "notes.txt 변경 준비",
          updatedAt: Date.now(),
        },
        events: [
          {
            id: "evt_prompt",
            threadId,
            jobId,
            kind: "prompt_submitted",
            role: "user",
            title: "프롬프트 제출",
            body: prompt,
            data: {},
            at: Date.now(),
          },
          {
            id: "evt_patch",
            threadId,
            jobId,
            kind: "patch_ready",
            role: "assistant",
            title: "패치 준비 완료",
            body: "notes.txt 변경 준비",
            data: {
              summary: "notes.txt 변경 준비",
              files: [
                {
                  path: "notes.txt",
                  status: "modified",
                  hunks: [
                    {
                      hunkId: "hunk_1",
                      header: "@@ -1 +1,2 @@",
                      diff: "+smoke-panel",
                      risk: "LOW",
                    },
                  ],
                },
              ],
            },
            at: Date.now(),
          },
        ],
      };
      state.threads = [detail.thread];
      state.details.set(threadId, detail);
      return json(res, 200, {
        responses: [
          { type: "PROMPT_ACK", payload: { threadId, jobId } },
          { type: "PATCH_READY", payload: { jobId, summary: "notes.txt 변경 준비" } },
        ],
      });
    }
    if (body.type === "PATCH_APPLY") {
      const detail = state.details.get("thread_panel_smoke");
      detail.thread.state = "success";
      detail.thread.lastEventKind = "patch_applied";
      detail.thread.lastEventText = "패치 적용 완료";
      detail.thread.updatedAt = Date.now();
      detail.events.push({
        id: "evt_apply",
        threadId: detail.thread.id,
        jobId: detail.thread.currentJobId,
        kind: "patch_applied",
        role: "system",
        title: "패치 적용 결과",
        body: "패치 적용 완료",
        data: { status: "success", message: "패치 적용 완료" },
        at: Date.now(),
      });
      state.threads = [detail.thread];
      return json(res, 200, { responses: [{ type: "PATCH_RESULT", payload: { status: "success" } }] });
    }
    if (body.type === "RUN_PROFILE") {
      const detail = state.details.get("thread_panel_smoke");
      detail.thread.state = "passed";
      detail.thread.lastEventKind = "run_finished";
      detail.thread.lastEventText = "smoke profile passed";
      detail.thread.updatedAt = Date.now();
      detail.events.push({
        id: "evt_run",
        threadId: detail.thread.id,
        jobId: detail.thread.currentJobId,
        kind: "run_finished",
        role: "system",
        title: "실행 결과",
        body: "smoke profile passed",
        data: {
          profileId: body.payload?.profileId || "smoke",
          status: "passed",
          summary: "smoke profile passed",
          output: "base\nsmoke-panel\n",
          topErrors: [
            {
              message: "synthetic warning",
              path: "notes.txt",
              line: 1,
              column: 1,
            },
          ],
        },
        at: Date.now(),
      });
      state.threads = [detail.thread];
      return json(res, 200, { responses: [{ type: "RUN_RESULT", payload: { status: "passed" } }] });
    }
    if (body.type === "OPEN_LOCATION") {
      state.openLocations.push(body.payload);
      return json(res, 200, { responses: [] });
    }
    return json(res, 200, { responses: [] });
  }
  json(res, 404, { error: "not found" });
});

await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const address = server.address();
const agentBaseUrl = `http://127.0.0.1:${address.port}`;

const messages = { info: [], warn: [], error: [] };
const commandRegistry = new Map();
const panelMessages = [];
let panelMessageHandler = null;

const fakePanel = {
  title: "",
  webview: {
    html: "",
    onDidReceiveMessage(listener) {
      panelMessageHandler = listener;
      return { dispose() {} };
    },
    async postMessage(message) {
      panelMessages.push(message);
      return true;
    },
  },
  reveal() {},
  onDidDispose() {
    return { dispose() {} };
  },
  dispose() {},
};

const fakeVscode = {
  commands: {
    async executeCommand(command, ...args) {
      return await commandRegistry.get(command)(...args);
    },
    registerCommand(command, callback) {
      commandRegistry.set(command, callback);
      return { dispose() { commandRegistry.delete(command); } };
    },
    async getCommands() {
      return [...commandRegistry.keys()];
    },
  },
  window: {
    activeTextEditor: undefined,
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
      return { text: "", tooltip: undefined, command: undefined, show() {}, dispose() {} };
    },
    createWebviewPanel() {
      return fakePanel;
    },
  },
  workspace: {
    textDocuments: [],
    workspaceFolders: [{ uri: { fsPath: "C:/demo/workspace" } }],
    async openTextDocument(path) {
      return { fileName: path, getText() { return ""; } };
    },
    getConfiguration(section = "") {
      return {
        get(key, defaultValue) {
          const qualifiedKey = section ? `${section}.${key}` : key;
          if (qualifiedKey === "vibedeckBridge.autoStart") return false;
          if (qualifiedKey === "vibedeckBridge.mode") return "command";
          if (qualifiedKey === "vibedeckBridge.commandProvider") return "builtin_cursor_agent";
          if (qualifiedKey === "vibedeckBridge.agentBaseUrl") return agentBaseUrl;
          if (qualifiedKey === "vibedeckBridge.panelAutoRefreshMs") return 60000;
          return defaultValue;
        },
      };
    },
    onDidChangeConfiguration() {
      return { dispose() {} };
    },
  },
  env: {
    clipboard: {
      async writeText() {},
    },
  },
  statusBarAlignment: { left: 1 },
  viewColumn: { one: 1 },
};

const controller = createBridgeExtensionController(fakeVscode);
const context = { subscriptions: [] };

try {
  await controller.activate(context);
  await fakeVscode.commands.executeCommand("vibedeckBridge.openThreadPanel");
  await tick();

  assert.match(fakePanel.webview.html, /VibeDeck Threads/);
  assert.ok(panelMessages.length > 0, "panel should receive initial state");

  await panelMessageHandler({
    type: "submit-prompt",
    prompt: "Create notes.txt hello world",
    contextOptions: {
      includeActiveFile: true,
      includeSelection: false,
      includeLatestError: true,
      includeWorkspaceSummary: false,
    },
  });
  await waitFor(() => (panelMessages.at(-1)?.state?.currentJobId || "") === "job_panel_smoke");

  await panelMessageHandler({ type: "apply-patch" });
  await waitFor(() => state.envelopes.some((item) => item.type === "PATCH_APPLY"));
  await panelMessageHandler({ type: "run-profile", profileId: "smoke" });
  await waitFor(() => (panelMessages.at(-1)?.state?.derived?.runStatus || "") === "passed");
  await panelMessageHandler({
    type: "open-location",
    path: "notes.txt",
    line: 1,
    column: 1,
  });
  await tick();

  const latestStateMessage = panelMessages.at(-1);
  assert.equal(latestStateMessage.type, "state");
  assert.equal(latestStateMessage.state.currentThread.id, "thread_panel_smoke");
  assert.equal(latestStateMessage.state.derived.runStatus, "passed");
  assert.equal(latestStateMessage.state.derived.patchFiles[0].path, "notes.txt");
  assert.equal(state.envelopes.map((item) => item.type).join(","), "PROMPT_SUBMIT,PATCH_APPLY,RUN_PROFILE,OPEN_LOCATION");
  assert.equal(state.openLocations.length, 1);

  console.log(JSON.stringify({
    agentBaseUrl,
    threadId: latestStateMessage.state.currentThread.id,
    runStatus: latestStateMessage.state.derived.runStatus,
    patchFiles: latestStateMessage.state.derived.patchFiles.map((item) => item.path),
    envelopeTypes: state.envelopes.map((item) => item.type),
    openLocationCount: state.openLocations.length,
  }, null, 2));
} finally {
  await controller.deactivate();
  for (const disposable of [...context.subscriptions].reverse()) {
    disposable.dispose();
  }
  await new Promise((resolve) => server.close(resolve));
}

function json(res, statusCode, body) {
  res.statusCode = statusCode;
  res.setHeader("Content-Type", "application/json");
  res.end(JSON.stringify(body));
}

async function readJson(req) {
  let body = "";
  for await (const chunk of req) {
    body += chunk;
  }
  return JSON.parse(body || "{}");
}

function tick() {
  return new Promise((resolve) => setTimeout(resolve, 10));
}
async function waitFor(predicate, timeoutMs = 500) {
  const startedAt = Date.now();
  while (Date.now() - startedAt < timeoutMs) {
    if (predicate()) {
      return;
    }
    await tick();
  }
  throw new Error("waitFor timeout");
}