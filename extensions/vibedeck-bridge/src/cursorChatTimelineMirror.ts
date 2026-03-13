import { createHash } from "node:crypto";

import {
  createAgentPanelApi,
  type AgentPanelApi,
  type AgentPanelTimelineEventInput,
} from "./agentPanelApi.js";
import {
  readCursorChatConversation,
  type CursorChatStorageBubbleDetail,
  type CursorChatStorageConversationDetail,
} from "./cursorChatStorageReader.js";
import type {
  CursorChatLinkTracker,
  CursorChatLinkedThread,
  CursorChatLinkTrackerSnapshot,
} from "./cursorChatLinkTracker.js";

export interface CursorChatTimelineMirror {
  refreshNow(): Promise<void>;
  dispose(): void;
}

export interface CursorChatTimelineMirrorOptions {
  readonly tracker: CursorChatLinkTracker;
  readonly getAgentBaseUrl: () => string;
  readonly api?: AgentPanelApi;
  readonly pollIntervalMs?: number;
  readonly onError?: (message: string) => void;
}

interface ThreadMirrorState {
  composerId: string;
  seenEventIds: Set<string>;
}

interface TimelineEventBatch {
  readonly events: AgentPanelTimelineEventInput[];
  readonly seenEventIds: string[];
}

const DEFAULT_POLL_INTERVAL_MS = 1800;
const INITIAL_SYNC_WINDOW_MS = 2 * 60 * 1000;

export function createCursorChatTimelineMirror(
  options: CursorChatTimelineMirrorOptions,
): CursorChatTimelineMirror {
  return new DefaultCursorChatTimelineMirror(options);
}

class DefaultCursorChatTimelineMirror implements CursorChatTimelineMirror {
  private readonly tracker: CursorChatLinkTracker;
  private readonly getAgentBaseUrl: () => string;
  private readonly api: AgentPanelApi;
  private readonly onError: ((message: string) => void) | undefined;
  private readonly pollIntervalMs: number;
  private readonly states = new Map<string, ThreadMirrorState>();
  private timer: NodeJS.Timeout | undefined;
  private refreshInFlight: Promise<void> | undefined;

  constructor(options: CursorChatTimelineMirrorOptions) {
    this.tracker = options.tracker;
    this.getAgentBaseUrl = options.getAgentBaseUrl;
    this.api = options.api ?? createAgentPanelApi();
    this.onError = options.onError;
    this.pollIntervalMs = normalizePositiveInteger(
      options.pollIntervalMs,
      DEFAULT_POLL_INTERVAL_MS,
    );
  }

  async refreshNow(): Promise<void> {
    if (this.refreshInFlight) {
      await this.refreshInFlight;
      return;
    }
    this.refreshInFlight = this.refreshCore();
    try {
      await this.refreshInFlight;
    } finally {
      this.refreshInFlight = undefined;
    }
  }

  dispose(): void {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = undefined;
    }
  }

  private async refreshCore(): Promise<void> {
    const snapshot = this.tracker.snapshot();
    this.ensureTimer(snapshot);
    this.pruneStates(snapshot);

    if (snapshot.linkedThreads.length === 0) {
      return;
    }

    const baseUrl = this.getAgentBaseUrl().trim();
    if (!baseUrl) {
      return;
    }

    for (const link of snapshot.linkedThreads) {
      const detail = await readCursorChatConversation({ composerId: link.composerId });
      if (!detail) {
        continue;
      }

      const state = this.ensureThreadState(link);
      const batch = buildTimelineEventBatch(link, detail, state);
      if (batch.events.length === 0) {
        continue;
      }

      try {
        await this.api.appendSessionTimelineEvents(baseUrl, link.threadId, batch.events);
        for (const eventId of batch.seenEventIds) {
          state.seenEventIds.add(eventId);
        }
      } catch (error) {
        this.onError?.(describeError(error));
      }
    }
  }

  private ensureTimer(snapshot: CursorChatLinkTrackerSnapshot): void {
    const shouldRun =
      snapshot.linkedThreads.length > 0 || snapshot.pendingSubmissions.length > 0;
    if (!shouldRun) {
      if (this.timer) {
        clearInterval(this.timer);
        this.timer = undefined;
      }
      return;
    }
    if (this.timer) {
      return;
    }
    this.timer = setInterval(() => {
      void this.refreshNow();
    }, this.pollIntervalMs);
  }

  private pruneStates(snapshot: CursorChatLinkTrackerSnapshot): void {
    const active = new Map(snapshot.linkedThreads.map((item) => [item.threadId, item.composerId]));
    for (const [threadId, state] of this.states) {
      const composerId = active.get(threadId);
      if (!composerId || composerId !== state.composerId) {
        this.states.delete(threadId);
      }
    }
  }

  private ensureThreadState(link: CursorChatLinkedThread): ThreadMirrorState {
    const current = this.states.get(link.threadId);
    if (current && current.composerId === link.composerId) {
      return current;
    }
    const next = {
      composerId: link.composerId,
      seenEventIds: new Set<string>(),
    };
    this.states.set(link.threadId, next);
    return next;
  }
}

function buildTimelineEventBatch(
  link: CursorChatLinkedThread,
  detail: CursorChatStorageConversationDetail,
  state: ThreadMirrorState,
): TimelineEventBatch {
  const events: AgentPanelTimelineEventInput[] = [];
  const seenEventIds: string[] = [];
  const initialBubbles = selectInitialBubbles(link, detail.bubbles, state.seenEventIds.size > 0);
  const candidates = state.seenEventIds.size > 0 ? detail.bubbles : initialBubbles;

  for (const bubble of candidates) {
    const bubbleEvent = mapBubbleToTimelineEvent(link, bubble);
    if (bubbleEvent && !state.seenEventIds.has(bubbleEvent.id ?? "")) {
      events.push(bubbleEvent);
      seenEventIds.push(bubbleEvent.id ?? "");
    }

    const contextEvent = mapContextToTimelineEvent(bubble);
    if (contextEvent && !state.seenEventIds.has(contextEvent.id ?? "")) {
      events.push(contextEvent);
      seenEventIds.push(contextEvent.id ?? "");
    }
  }

  return {
    events,
    seenEventIds: seenEventIds.filter(Boolean),
  };
}

function selectInitialBubbles(
  link: CursorChatLinkedThread,
  bubbles: readonly CursorChatStorageBubbleDetail[],
  keepAll: boolean,
): readonly CursorChatStorageBubbleDetail[] {
  if (keepAll) {
    return bubbles;
  }

  const submittedAtMs = parseTimestamp(link.submittedAt);
  const normalizedPrompt = normalizeText(link.matchedPrompt);
  const anchorIndex = bubbles.findIndex((bubble) => {
    if (bubble.type !== 1) {
      return false;
    }
    if (normalizeText(bubble.text) !== normalizedPrompt) {
      return false;
    }
    const createdAtMs = parseTimestamp(bubble.createdAt);
    if (!submittedAtMs || !createdAtMs) {
      return true;
    }
    return Math.abs(createdAtMs - submittedAtMs) <= INITIAL_SYNC_WINDOW_MS;
  });
  if (anchorIndex >= 0) {
    return bubbles.slice(anchorIndex);
  }

  return bubbles.filter((bubble) => {
    const createdAtMs = parseTimestamp(bubble.createdAt);
    if (!submittedAtMs || !createdAtMs) {
      return true;
    }
    return createdAtMs >= submittedAtMs - INITIAL_SYNC_WINDOW_MS;
  });
}

function mapBubbleToTimelineEvent(
  link: CursorChatLinkedThread,
  bubble: CursorChatStorageBubbleDetail,
): AgentPanelTimelineEventInput | undefined {
  const body = bubble.text.trim();
  if (!body) {
    return undefined;
  }

  const role = bubble.type === 1 ? "user" : bubble.type === 2 ? "assistant" : "system";
  const title =
    role === "assistant"
      ? "Cursor 응답"
      : role === "user"
        ? "Cursor 사용자 메시지"
        : "Cursor 메시지";
  const bodyHash = hashText(body);
  return {
    id: `cursor-bubble:${link.composerId}:${bubble.bubbleId}:${bodyHash}`,
    kind: "provider_message",
    role,
    title,
    body,
    data: {
      source: "cursor_storage",
      composerId: link.composerId,
      bubbleId: bubble.bubbleId,
      cursorType: bubble.type,
    },
    at: parseTimestamp(bubble.createdAt) || Date.now(),
  };
}

function mapContextToTimelineEvent(
  bubble: CursorChatStorageBubbleDetail,
): AgentPanelTimelineEventInput | undefined {
  const summary = summarizeContext(bubble.context);
  if (!summary) {
    return undefined;
  }

  return {
    id: `cursor-context:${bubble.composerId}:${bubble.bubbleId}`,
    kind: "tool_activity",
    role: "system",
    title: "Cursor 요청 맥락",
    body: summary,
    data: {
      source: "cursor_storage",
      composerId: bubble.composerId,
      bubbleId: bubble.bubbleId,
      ...bubble.context,
    },
    at: parseTimestamp(bubble.createdAt) || Date.now(),
  };
}

function summarizeContext(context: Record<string, unknown>): string {
  const keys = Object.keys(context);
  if (keys.length === 0) {
    return "";
  }

  const files = uniqueStrings([
    context.files,
    context.filePaths,
    context.selectedFiles,
    context.selectedFilePaths,
    context.attachedFiles,
    context.mentionedFiles,
    context.folderSelections,
  ]);
  const terminal = uniqueStrings([
    context.terminalFiles,
    context.terminalCommands,
    context.commands,
    context.command,
  ]);
  const todos = Array.isArray(context.todos) ? context.todos.length : 0;

  const parts: string[] = [];
  if (files.length > 0) {
    parts.push(`files: ${files.slice(0, 3).join(", ")}`);
  }
  if (terminal.length > 0) {
    parts.push(`terminal: ${terminal.slice(0, 2).join(", ")}`);
  }
  if (todos > 0) {
    parts.push(`todos: ${todos}`);
  }
  if (parts.length > 0) {
    return parts.join(" | ");
  }
  return `context keys: ${keys.slice(0, 4).join(", ")}`;
}

function uniqueStrings(values: readonly unknown[]): string[] {
  const seen = new Set<string>();
  const items: string[] = [];
  for (const value of values) {
    for (const candidate of flattenUnknownStrings(value)) {
      if (!candidate || seen.has(candidate)) {
        continue;
      }
      seen.add(candidate);
      items.push(candidate);
    }
  }
  return items;
}

function flattenUnknownStrings(value: unknown): string[] {
  if (typeof value === "string") {
    const trimmed = value.trim();
    return trimmed ? [trimmed] : [];
  }
  if (Array.isArray(value)) {
    return value.flatMap((item) => flattenUnknownStrings(item));
  }
  if (value && typeof value === "object") {
    const record = value as Record<string, unknown>;
    return [record.path, record.filePath, record.command, record.label, record.text].flatMap(
      (item) => flattenUnknownStrings(item),
    );
  }
  return [];
}

function hashText(value: string): string {
  return createHash("sha1").update(value).digest("hex").slice(0, 12);
}

function normalizeText(value: string): string {
  return value.replace(/\s+/g, " ").trim();
}

function parseTimestamp(value: string): number {
  if (!value.trim()) {
    return 0;
  }
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function normalizePositiveInteger(value: number | undefined, fallback: number): number {
  if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) {
    return fallback;
  }
  return Math.trunc(value);
}

function describeError(error: unknown): string {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return String(error);
}
