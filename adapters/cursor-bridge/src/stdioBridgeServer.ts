import readline from "node:readline";

import type { BridgeRequest, BridgeResponse } from "./stdioProtocol.js";
import type {
  ApplyPatchInput,
  ContextRequest,
  OpenLocationInput,
  RunProfileInput,
  WorkspaceAdapter,
} from "./types.js";

export function serveStdioBridge(adapter: WorkspaceAdapter): void {
  process.stdin.setEncoding("utf8");

  const lineReader = readline.createInterface({
    input: process.stdin,
    crlfDelay: Infinity,
  });

  lineReader.on("line", (line) => {
    void handleLine(adapter, line);
  });

  lineReader.on("close", () => {
    process.exit(0);
  });
}

async function handleLine(adapter: WorkspaceAdapter, line: string): Promise<void> {
  const trimmed = line.trim();
  if (trimmed.length === 0) {
    return;
  }

  let request: BridgeRequest;
  try {
    request = parseRequest(trimmed);
  } catch (error) {
    writeResponse({
      id: "unknown",
      error: {
        message: errorMessage(error),
      },
    });
    return;
  }

  try {
    const result = await dispatch(adapter, request);
    writeResponse({
      id: request.id,
      result,
    });
  } catch (error) {
    writeResponse({
      id: request.id,
      error: {
        message: errorMessage(error),
      },
    });
  }
}

function parseRequest(line: string): BridgeRequest {
  const value: unknown = JSON.parse(line);
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("invalid bridge request");
  }

  const request = value as BridgeRequest;
  if (typeof request.id !== "string" || request.id.length === 0) {
    throw new Error("bridge request id is required");
  }
  if (typeof request.method !== "string" || request.method.length === 0) {
    throw new Error("bridge request method is required");
  }

  return request;
}

async function dispatch(adapter: WorkspaceAdapter, request: BridgeRequest): Promise<unknown> {
  switch (request.method) {
    case "name":
      return adapter.name();
    case "capabilities":
      return adapter.capabilities();
    case "getContext":
      return adapter.getContext(asParams<ContextRequest>(request.params, "getContext"));
    case "submitTask":
      return adapter.submitTask(asParams(request.params, "submitTask"));
    case "getPatch": {
      const params = asRecord(request.params, "getPatch");
      return adapter.getPatch(readRequiredString(params.taskId, "getPatch.taskId"));
    }
    case "applyPatch":
      return adapter.applyPatch(asParams<ApplyPatchInput>(request.params, "applyPatch"));
    case "runProfile":
      return adapter.runProfile(asParams<RunProfileInput>(request.params, "runProfile"));
    case "getRunResult": {
      const params = asRecord(request.params, "getRunResult");
      return adapter.getRunResult(readRequiredString(params.runId, "getRunResult.runId"));
    }
    case "openLocation":
      await adapter.openLocation(asParams<OpenLocationInput>(request.params, "openLocation"));
      return null;
    default:
      throw new Error(`unsupported bridge method: ${request.method}`);
  }
}

function asParams<T>(value: unknown, label: string): T {
  return asRecord(value, label) as T;
}

function asRecord(value: unknown, label: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`invalid ${label} params`);
  }

  return value as Record<string, unknown>;
}

function readRequiredString(value: unknown, label: string): string {
  if (typeof value !== "string" || value.length === 0) {
    throw new Error(`invalid ${label}`);
  }

  return value;
}

function errorMessage(error: unknown): string {
  if (error instanceof Error && error.message.length > 0) {
    return error.message;
  }

  return "unknown bridge error";
}

function writeResponse(response: BridgeResponse): void {
  process.stdout.write(`${JSON.stringify(response)}\n`);
}