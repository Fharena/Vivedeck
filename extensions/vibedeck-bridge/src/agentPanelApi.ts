import http from "node:http";
import https from "node:https";

export interface AgentPanelAdapterRuntime {
  name: string;
  mode: string;
  ready: boolean;
  workspaceRoot: string;
  binaryPath: string;
  notes: string[];
}

export interface AgentPanelRunProfile {
  id: string;
  label: string;
  command: string;
  scope: string;
  optional: boolean;
}

export interface AgentPanelThreadSummary {
  id: string;
  title: string;
  sessionId: string;
  state: string;
  currentJobId: string;
  lastEventKind: string;
  lastEventText: string;
  updatedAt: number;
}

export interface AgentPanelThreadEvent {
  id: string;
  threadId: string;
  jobId: string;
  kind: string;
  role: string;
  title: string;
  body: string;
  data: Record<string, unknown>;
  at: number;
}

export interface AgentPanelThreadDetail {
  thread: AgentPanelThreadSummary;
  events: AgentPanelThreadEvent[];
}

export interface AgentPanelBootstrapAdapter {
  name: string;
  mode: string;
  provider: string;
  ready: boolean;
}

export interface AgentPanelBootstrapThread {
  id: string;
  title: string;
  updatedAt: number;
  current: boolean;
}

export interface AgentPanelBootstrap {
  agentBaseUrl: string;
  signalingBaseUrl: string;
  workspaceRoot: string;
  currentThreadId: string;
  adapter: AgentPanelBootstrapAdapter;
  recentThreads: AgentPanelBootstrapThread[];
}

export interface AgentPanelEnvelope {
  sid: string;
  rid: string;
  seq: number;
  ts: number;
  type: string;
  payload: Record<string, unknown>;
}

export interface AgentPanelEnvelopeResponse {
  responses: Record<string, unknown>[];
}

export interface AgentPanelApi {
  runtimeAdapter(baseUrl: string): Promise<AgentPanelAdapterRuntime>;
  bootstrap(baseUrl: string): Promise<AgentPanelBootstrap>;
  runProfiles(baseUrl: string): Promise<AgentPanelRunProfile[]>;
  threads(baseUrl: string): Promise<AgentPanelThreadSummary[]>;
  threadDetail(baseUrl: string, threadId: string): Promise<AgentPanelThreadDetail>;
  sendEnvelope(
    baseUrl: string,
    envelope: AgentPanelEnvelope,
  ): Promise<AgentPanelEnvelopeResponse>;
}

export class AgentPanelApiError extends Error {
  readonly statusCode: number;

  constructor(statusCode: number, message: string) {
    super(message);
    this.name = "AgentPanelApiError";
    this.statusCode = statusCode;
  }
}

export function createAgentPanelApi(): AgentPanelApi {
  return new DefaultAgentPanelApi();
}

class DefaultAgentPanelApi implements AgentPanelApi {
  async runtimeAdapter(baseUrl: string): Promise<AgentPanelAdapterRuntime> {
    const body = await requestJson(baseUrl, "/v1/agent/runtime/adapter");
    return {
      name: text(body.name),
      mode: text(body.mode),
      ready: body.ready === true,
      workspaceRoot: text(body.workspaceRoot),
      binaryPath: text(body.binaryPath),
      notes: stringArray(body.notes),
    };
  }

  async bootstrap(baseUrl: string): Promise<AgentPanelBootstrap> {
    const body = await requestJson(baseUrl, "/v1/agent/bootstrap");
    const adapterSource = objectValue(body.adapter);
    return {
      agentBaseUrl: text(body.agentBaseUrl),
      signalingBaseUrl: text(body.signalingBaseUrl),
      workspaceRoot: text(body.workspaceRoot),
      currentThreadId: text(body.currentThreadId),
      adapter: {
        name: text(adapterSource.name),
        mode: text(adapterSource.mode),
        provider: text(adapterSource.provider),
        ready: adapterSource.ready === true,
      },
      recentThreads: objectArray(body.recentThreads).map((item) => ({
        id: text(item.id),
        title: text(item.title),
        updatedAt: numberValue(item.updatedAt),
        current: item.current === true,
      })),
    };
  }

  async runProfiles(baseUrl: string): Promise<AgentPanelRunProfile[]> {
    const body = await requestJson(baseUrl, "/v1/agent/run-profiles");
    return objectArray(body.profiles).map((item) => ({
      id: text(item.id),
      label: text(item.label),
      command: text(item.command),
      scope: text(item.scope),
      optional: item.optional === true,
    }));
  }

  async threads(baseUrl: string): Promise<AgentPanelThreadSummary[]> {
    const body = await requestJson(baseUrl, "/v1/agent/threads");
    return objectArray(body.threads).map((item) => ({
      id: text(item.id),
      title: text(item.title),
      sessionId: text(item.sessionId),
      state: text(item.state),
      currentJobId: text(item.currentJobId),
      lastEventKind: text(item.lastEventKind),
      lastEventText: text(item.lastEventText),
      updatedAt: numberValue(item.updatedAt),
    }));
  }

  async threadDetail(
    baseUrl: string,
    threadId: string,
  ): Promise<AgentPanelThreadDetail> {
    const body = await requestJson(
      baseUrl,
      `/v1/agent/threads/${encodeURIComponent(threadId)}`,
    );
    const threadSource = objectValue(body.thread);
    return {
      thread: {
        id: text(threadSource.id),
        title: text(threadSource.title),
        sessionId: text(threadSource.sessionId),
        state: text(threadSource.state),
        currentJobId: text(threadSource.currentJobId),
        lastEventKind: text(threadSource.lastEventKind),
        lastEventText: text(threadSource.lastEventText),
        updatedAt: numberValue(threadSource.updatedAt),
      },
      events: objectArray(body.events).map((item) => ({
        id: text(item.id),
        threadId: text(item.threadId),
        jobId: text(item.jobId),
        kind: text(item.kind),
        role: text(item.role),
        title: text(item.title),
        body: text(item.body),
        data: objectValue(item.data),
        at: numberValue(item.at),
      })),
    };
  }

  async sendEnvelope(
    baseUrl: string,
    envelope: AgentPanelEnvelope,
  ): Promise<AgentPanelEnvelopeResponse> {
    const body = await requestJson(baseUrl, "/v1/agent/envelope", "POST", envelope);
    return {
      responses: objectArray(body.responses),
    };
  }
}

async function requestJson(
  baseUrl: string,
  path: string,
  method = "GET",
  body?: unknown,
): Promise<Record<string, unknown>> {
  const normalizedBaseUrl = normalizeBaseUrl(baseUrl);
  const url = new URL(path, normalizedBaseUrl);
  const payload = body == null ? undefined : JSON.stringify(body);
  const transport = url.protocol === "https:" ? https : http;

  return await new Promise<Record<string, unknown>>((resolve, reject) => {
    const request = transport.request(
      url,
      {
        method,
        headers: payload
          ? {
              "Content-Type": "application/json",
              "Content-Length": Buffer.byteLength(payload).toString(),
            }
          : undefined,
      },
      (response) => {
        response.setEncoding("utf8");
        let raw = "";
        response.on("data", (chunk) => {
          raw += chunk;
        });
        response.on("end", () => {
          const statusCode = response.statusCode ?? 0;
          const decoded = decodeJsonBody(raw);
          if (statusCode < 200 || statusCode >= 300) {
            const message =
              typeof decoded.error === "string"
                ? decoded.error
                : JSON.stringify(decoded);
            reject(new AgentPanelApiError(statusCode, message || "agent request failed"));
            return;
          }
          resolve(decoded);
        });
      },
    );

    request.on("error", (error) => {
      reject(new AgentPanelApiError(0, error.message));
    });

    if (payload) {
      request.write(payload);
    }
    request.end();
  });
}

function normalizeBaseUrl(value: string): string {
  const trimmed = value.trim();
  if (trimmed.length === 0) {
    throw new AgentPanelApiError(0, "agent base url is empty");
  }
  return trimmed.endsWith("/") ? trimmed : `${trimmed}/`;
}

function decodeJsonBody(value: string): Record<string, unknown> {
  if (value.trim().length === 0) {
    return {};
  }
  try {
    const decoded = JSON.parse(value);
    if (decoded != null && typeof decoded === "object" && !Array.isArray(decoded)) {
      return decoded as Record<string, unknown>;
    }
    return { data: decoded };
  } catch {
    return { raw: value };
  }
}

function objectArray(value: unknown): Record<string, unknown>[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .filter(
      (item): item is Record<string, unknown> =>
        item != null && typeof item === "object" && !Array.isArray(item),
    )
    .map((item) => ({ ...item }));
}

function objectValue(value: unknown): Record<string, unknown> {
  if (value != null && typeof value === "object" && !Array.isArray(value)) {
    return { ...(value as Record<string, unknown>) };
  }
  return {};
}

function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.map((item) => text(item)).filter((item) => item.length > 0);
}

function text(value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  if (value == null) {
    return "";
  }
  return String(value);
}

function numberValue(value: unknown): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return Math.trunc(value);
  }
  const parsed = Number.parseInt(text(value), 10);
  return Number.isFinite(parsed) ? parsed : 0;
}