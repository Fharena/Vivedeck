import {
  probeCursorChatStorage,
  type CursorChatStorageConversationSummary,
  type CursorChatStorageReport,
} from "./cursorChatStorageProbe.js";
import type { CursorNativePromptSubmitStatus } from "./cursorNativePromptSubmit.js";

export interface CursorChatPromptSubmission {
  readonly threadId: string;
  readonly prompt: string;
  readonly submitStatus: CursorNativePromptSubmitStatus;
  readonly strategyKind: string;
  readonly submittedAt?: string;
}

export interface CursorChatLinkedThread {
  readonly threadId: string;
  readonly composerId: string;
  readonly matchedPrompt: string;
  readonly submitStatus: CursorNativePromptSubmitStatus;
  readonly strategyKind: string;
  readonly linkedAt: string;
  readonly submittedAt: string;
  readonly matchReason:
    | "same_composer_latest_user_text_exact"
    | "latest_user_text_exact"
    | "first_user_text_exact";
  readonly matchConfidence: "high" | "medium";
  readonly composerCreatedAt: string;
  readonly composerUpdatedAt: string;
  readonly latestUserText: string;
  readonly latestAssistantText: string;
  readonly status: string;
  readonly headerCount: number;
  readonly contextCount: number;
}

export interface CursorChatPendingSubmission {
  readonly threadId: string;
  readonly prompt: string;
  readonly submitStatus: CursorNativePromptSubmitStatus;
  readonly strategyKind: string;
  readonly submittedAt: string;
}

export interface CursorChatLinkTrackerSnapshot {
  readonly checkedAt: string;
  readonly storageCheckedAt: string;
  readonly pollerActive: boolean;
  readonly pollIntervalMs: number;
  readonly backend: CursorChatStorageReport["backend"] | "unknown";
  readonly backendDetail: string | undefined;
  readonly composerCount: number;
  readonly bubbleCount: number;
  readonly contextCount: number;
  readonly linkedThreads: readonly CursorChatLinkedThread[];
  readonly pendingSubmissions: readonly CursorChatPendingSubmission[];
  readonly recentConversations: readonly CursorChatStorageConversationSummary[];
  readonly lastError: string | undefined;
}

export interface CursorChatLinkTracker {
  notePromptSubmission(input: CursorChatPromptSubmission): Promise<CursorChatLinkTrackerSnapshot>;
  refreshNow(): Promise<CursorChatLinkTrackerSnapshot>;
  snapshot(): CursorChatLinkTrackerSnapshot;
  dispose(): void;
}

export interface CursorChatLinkTrackerOptions {
  readonly pollIntervalMs?: number;
  readonly maxConversations?: number;
}

interface PendingSubmissionInternal extends CursorChatPendingSubmission {
  readonly submittedAtMs: number;
}

interface MatchCandidate {
  readonly conversation: CursorChatStorageConversationSummary;
  readonly reason: CursorChatLinkedThread["matchReason"];
  readonly confidence: CursorChatLinkedThread["matchConfidence"];
  readonly score: number;
}

const DEFAULT_POLL_INTERVAL_MS = 1500;
const DEFAULT_MAX_CONVERSATIONS = 24;
const MAX_PENDING_AGE_MS = 5 * 60 * 1000;
const NEW_THREAD_MATCH_WINDOW_MS = 2 * 60 * 1000;

export function createCursorChatLinkTracker(
  options: CursorChatLinkTrackerOptions = {},
): CursorChatLinkTracker {
  return new DefaultCursorChatLinkTracker(options);
}

export function formatCursorChatLinkTrackerReport(
  snapshot: CursorChatLinkTrackerSnapshot,
): string {
  const lines = [
    "VibeDeck Cursor Chat Link Report",
    `checked at: ${snapshot.checkedAt}`,
    `storage checked at: ${snapshot.storageCheckedAt || "(none)"}`,
    `poller active: ${snapshot.pollerActive ? "yes" : "no"}`,
    `poll interval ms: ${snapshot.pollIntervalMs}`,
    `storage backend: ${snapshot.backend}`,
    `backend detail: ${snapshot.backendDetail ?? "(none)"}`,
    `composerData rows: ${snapshot.composerCount}`,
    `bubble rows: ${snapshot.bubbleCount}`,
    `messageRequestContext rows: ${snapshot.contextCount}`,
    `linked threads: ${snapshot.linkedThreads.length}`,
  ];

  for (const link of snapshot.linkedThreads) {
    lines.push(
      `- ${link.threadId} -> ${link.composerId} | reason=${link.matchReason} | confidence=${link.matchConfidence} | submit=${link.submitStatus} | status=${link.status || "(none)"} | updated=${link.composerUpdatedAt || "(none)"}`,
    );
    lines.push(`  prompt: ${truncatePreview(link.matchedPrompt)}`);
    lines.push(`  latest user: ${truncatePreview(link.latestUserText)}`);
    lines.push(`  latest assistant: ${truncatePreview(link.latestAssistantText)}`);
  }

  lines.push(`pending submissions: ${snapshot.pendingSubmissions.length}`);
  for (const pending of snapshot.pendingSubmissions) {
    lines.push(
      `- ${pending.threadId} | submit=${pending.submitStatus} | strategy=${pending.strategyKind} | at=${pending.submittedAt}`,
    );
    lines.push(`  prompt: ${truncatePreview(pending.prompt)}`);
  }

  lines.push("recent conversations:");
  for (const conversation of snapshot.recentConversations) {
    lines.push(
      `- ${conversation.composerId} | updated=${conversation.updatedAt || conversation.createdAt || "(none)"} | status=${conversation.status || "(none)"} | headers=${conversation.headerCount} | context=${conversation.contextCount}`,
    );
    lines.push(`  latest user: ${truncatePreview(conversation.latestUserText)}`);
    lines.push(`  latest assistant: ${truncatePreview(conversation.latestAssistantText)}`);
  }

  if (snapshot.lastError) {
    lines.push(`last error: ${snapshot.lastError}`);
  }

  return lines.join("\n");
}

class DefaultCursorChatLinkTracker implements CursorChatLinkTracker {
  private readonly pollIntervalMs: number;
  private readonly maxConversations: number;
  private readonly linksByThread = new Map<string, CursorChatLinkedThread>();
  private pendingSubmissions: PendingSubmissionInternal[] = [];
  private lastReport: CursorChatStorageReport | undefined;
  private lastError: string | undefined;
  private poller: NodeJS.Timeout | undefined;
  private refreshInFlight: Promise<CursorChatLinkTrackerSnapshot> | undefined;

  constructor(options: CursorChatLinkTrackerOptions) {
    this.pollIntervalMs = normalizePositiveInteger(
      options.pollIntervalMs,
      DEFAULT_POLL_INTERVAL_MS,
    );
    this.maxConversations = normalizePositiveInteger(
      options.maxConversations,
      DEFAULT_MAX_CONVERSATIONS,
    );
  }

  async notePromptSubmission(
    input: CursorChatPromptSubmission,
  ): Promise<CursorChatLinkTrackerSnapshot> {
    const threadId = input.threadId.trim();
    const prompt = input.prompt.trim();
    if (!threadId || !prompt) {
      return this.snapshot();
    }

    const submittedAt = normalizeIsoTimestamp(input.submittedAt);
    this.pendingSubmissions = [
      ...this.pendingSubmissions,
      {
        threadId,
        prompt,
        submitStatus: input.submitStatus,
        strategyKind: input.strategyKind,
        submittedAt,
        submittedAtMs: Date.parse(submittedAt),
      },
    ];
    this.prunePendingSubmissions();
    this.ensurePoller();
    return await this.refreshNow();
  }

  async refreshNow(): Promise<CursorChatLinkTrackerSnapshot> {
    if (this.refreshInFlight) {
      return await this.refreshInFlight;
    }
    this.refreshInFlight = this.refreshCore();
    try {
      return await this.refreshInFlight;
    } finally {
      this.refreshInFlight = undefined;
    }
  }

  snapshot(): CursorChatLinkTrackerSnapshot {
    return {
      checkedAt: new Date().toISOString(),
      storageCheckedAt: this.lastReport?.checkedAt ?? "",
      pollerActive: Boolean(this.poller),
      pollIntervalMs: this.pollIntervalMs,
      backend: this.lastReport?.backend ?? "unknown",
      backendDetail: this.lastReport?.backendDetail,
      composerCount: this.lastReport?.composerCount ?? 0,
      bubbleCount: this.lastReport?.bubbleCount ?? 0,
      contextCount: this.lastReport?.contextCount ?? 0,
      linkedThreads: [...this.linksByThread.values()].sort((left, right) =>
        right.linkedAt.localeCompare(left.linkedAt),
      ),
      pendingSubmissions: this.pendingSubmissions
        .map((pending) => ({
          threadId: pending.threadId,
          prompt: pending.prompt,
          submitStatus: pending.submitStatus,
          strategyKind: pending.strategyKind,
          submittedAt: pending.submittedAt,
        }))
        .sort((left, right) => right.submittedAt.localeCompare(left.submittedAt)),
      recentConversations: this.lastReport?.conversations ?? [],
      lastError: this.lastError,
    };
  }

  dispose(): void {
    if (this.poller) {
      clearInterval(this.poller);
      this.poller = undefined;
    }
  }

  private async refreshCore(): Promise<CursorChatLinkTrackerSnapshot> {
    this.prunePendingSubmissions();
    try {
      const report = await probeCursorChatStorage({
        maxConversations: this.maxConversations,
      });
      this.lastReport = report;
      this.lastError = undefined;
      this.matchPendingSubmissions(report.conversations);
    } catch (error) {
      this.lastError = describeError(error);
    }
    this.ensurePoller();
    return this.snapshot();
  }

  private ensurePoller(): void {
    const shouldRun = this.pendingSubmissions.length > 0 || this.linksByThread.size > 0;
    if (!shouldRun) {
      if (this.poller) {
        clearInterval(this.poller);
        this.poller = undefined;
      }
      return;
    }
    if (this.poller) {
      return;
    }
    this.poller = setInterval(() => {
      void this.refreshNow();
    }, this.pollIntervalMs);
  }

  private prunePendingSubmissions(): void {
    const now = Date.now();
    this.pendingSubmissions = this.pendingSubmissions.filter(
      (pending) => now - pending.submittedAtMs <= MAX_PENDING_AGE_MS,
    );
  }

  private matchPendingSubmissions(
    conversations: readonly CursorChatStorageConversationSummary[],
  ): void {
    const claimedComposerIds = new Map<string, string>();
    for (const link of this.linksByThread.values()) {
      claimedComposerIds.set(link.composerId, link.threadId);
    }

    const nextPending: PendingSubmissionInternal[] = [];
    for (const pending of this.pendingSubmissions) {
      const existingLink = this.linksByThread.get(pending.threadId);
      const candidate = findBestCandidate(
        pending,
        existingLink,
        conversations,
        claimedComposerIds,
      );
      if (!candidate) {
        nextPending.push(pending);
        continue;
      }

      this.linksByThread.set(pending.threadId, {
        threadId: pending.threadId,
        composerId: candidate.conversation.composerId,
        matchedPrompt: pending.prompt,
        submitStatus: pending.submitStatus,
        strategyKind: pending.strategyKind,
        linkedAt: new Date().toISOString(),
        submittedAt: pending.submittedAt,
        matchReason: candidate.reason,
        matchConfidence: candidate.confidence,
        composerCreatedAt: candidate.conversation.createdAt,
        composerUpdatedAt:
          candidate.conversation.updatedAt || candidate.conversation.createdAt,
        latestUserText:
          candidate.conversation.latestUserText || candidate.conversation.firstUserText,
        latestAssistantText:
          candidate.conversation.latestAssistantText ||
          candidate.conversation.firstAssistantText,
        status: candidate.conversation.status,
        headerCount: candidate.conversation.headerCount,
        contextCount: candidate.conversation.contextCount,
      });
      claimedComposerIds.set(candidate.conversation.composerId, pending.threadId);
    }

    this.pendingSubmissions = nextPending;
  }
}

function findBestCandidate(
  pending: PendingSubmissionInternal,
  existingLink: CursorChatLinkedThread | undefined,
  conversations: readonly CursorChatStorageConversationSummary[],
  claimedComposerIds: ReadonlyMap<string, string>,
): MatchCandidate | undefined {
  const normalizedPrompt = normalizePrompt(pending.prompt);
  if (!normalizedPrompt) {
    return undefined;
  }

  let best: MatchCandidate | undefined;
  for (const conversation of conversations) {
    const claimedThreadId = claimedComposerIds.get(conversation.composerId);
    if (claimedThreadId && claimedThreadId !== pending.threadId) {
      continue;
    }

    const candidate = scoreConversationMatch(
      pending,
      existingLink,
      conversation,
      normalizedPrompt,
    );
    if (!candidate) {
      continue;
    }

    if (!best || candidate.score > best.score) {
      best = candidate;
    }
  }

  return best;
}

function scoreConversationMatch(
  pending: PendingSubmissionInternal,
  existingLink: CursorChatLinkedThread | undefined,
  conversation: CursorChatStorageConversationSummary,
  normalizedPrompt: string,
): MatchCandidate | undefined {
  const normalizedLatestUser = normalizePrompt(
    conversation.latestUserText || conversation.firstUserText,
  );
  const normalizedFirstUser = normalizePrompt(conversation.firstUserText);
  const updatedAtMs = parseTimestamp(conversation.updatedAt || conversation.latestUserAt);
  const createdAtMs = parseTimestamp(conversation.createdAt);
  const nearNewThreadWindow =
    createdAtMs === 0 ||
    Math.abs(createdAtMs - pending.submittedAtMs) <= NEW_THREAD_MATCH_WINDOW_MS;
  const nearLatestWindow =
    updatedAtMs === 0 ||
    Math.abs(updatedAtMs - pending.submittedAtMs) <= NEW_THREAD_MATCH_WINDOW_MS;

  if (
    existingLink &&
    conversation.composerId === existingLink.composerId &&
    normalizedLatestUser === normalizedPrompt
  ) {
    return {
      conversation,
      reason: "same_composer_latest_user_text_exact",
      confidence: "high",
      score: 3000 + recencyBonus(updatedAtMs, pending.submittedAtMs),
    };
  }

  if (normalizedLatestUser === normalizedPrompt && nearLatestWindow) {
    return {
      conversation,
      reason: "latest_user_text_exact",
      confidence: "high",
      score: 2000 + recencyBonus(updatedAtMs, pending.submittedAtMs),
    };
  }

  if (normalizedFirstUser === normalizedPrompt && nearNewThreadWindow) {
    return {
      conversation,
      reason: "first_user_text_exact",
      confidence: "medium",
      score: 1000 + recencyBonus(createdAtMs, pending.submittedAtMs),
    };
  }

  return undefined;
}

function recencyBonus(candidateAtMs: number, submittedAtMs: number): number {
  if (!candidateAtMs || !submittedAtMs) {
    return 0;
  }
  const delta = Math.abs(candidateAtMs - submittedAtMs);
  if (delta >= NEW_THREAD_MATCH_WINDOW_MS) {
    return 0;
  }
  return Math.max(0, 300 - Math.trunc(delta / 1000));
}

function normalizePrompt(value: string): string {
  return value.replace(/\s+/g, " ").trim();
}

function parseTimestamp(value: string): number {
  if (!value.trim()) {
    return 0;
  }
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function normalizeIsoTimestamp(value: string | undefined): string {
  if (typeof value === "string" && value.trim()) {
    const parsed = Date.parse(value);
    if (Number.isFinite(parsed)) {
      return new Date(parsed).toISOString();
    }
  }
  return new Date().toISOString();
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

function truncatePreview(value: string): string {
  const normalized = normalizePrompt(value);
  if (!normalized) {
    return "(empty)";
  }
  return normalized.length > 140 ? `${normalized.slice(0, 137)}...` : normalized;
}
