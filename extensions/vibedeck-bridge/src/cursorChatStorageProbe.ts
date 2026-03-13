import { existsSync, readdirSync } from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";

export interface CursorChatStorageConversationSummary {
  readonly composerId: string;
  readonly createdAt: string;
  readonly updatedAt: string;
  readonly status: string;
  readonly isAgentic: boolean;
  readonly headerCount: number;
  readonly contextCount: number;
  readonly firstUserText: string;
  readonly firstAssistantText: string;
  readonly latestUserText: string;
  readonly latestAssistantText: string;
  readonly latestUserAt: string;
  readonly latestAssistantAt: string;
}

export interface CursorChatStorageReport {
  readonly checkedAt: string;
  readonly cursorUserRoot: string;
  readonly globalStorageDbPath: string;
  readonly workspaceDbCount: number;
  readonly backend: "node_sqlite" | "python" | "unavailable";
  readonly backendDetail: string | undefined;
  readonly composerCount: number;
  readonly bubbleCount: number;
  readonly contextCount: number;
  readonly conversations: readonly CursorChatStorageConversationSummary[];
  readonly findings: readonly string[];
  readonly conclusions: {
    readonly canReadStorage: boolean;
    readonly summary: string;
  };
}

export interface CursorChatStorageProbeOptions {
  readonly cursorUserRoot?: string;
  readonly maxConversations?: number;
}

interface StorageProbeBackendResult {
  readonly backend: "node_sqlite" | "python";
  readonly backendDetail: string;
  readonly composerCount: number;
  readonly bubbleCount: number;
  readonly contextCount: number;
  readonly conversations: readonly CursorChatStorageConversationSummary[];
}

interface SqliteRow {
  readonly key: string;
  readonly value: string;
}

const DEFAULT_MAX_CONVERSATIONS = 6;

const PYTHON_SCRIPT = [
  "import json, sqlite3, sys, datetime",
  "db_path = sys.argv[1]",
  "limit = int(sys.argv[2])",
  "conn = sqlite3.connect(db_path)",
  "cur = conn.cursor()",
  "composer_rows = cur.execute(\"SELECT key, value FROM cursorDiskKV WHERE key LIKE 'composerData:%'\").fetchall()",
  "bubble_count = cur.execute(\"SELECT COUNT(*) FROM cursorDiskKV WHERE key LIKE 'bubbleId:%'\").fetchone()[0]",
  "context_count = cur.execute(\"SELECT COUNT(*) FROM cursorDiskKV WHERE key LIKE 'messageRequestContext:%'\").fetchone()[0]",
  "",
  "def normalize_created_at(value):",
  "    if isinstance(value, (int, float)):",
  "        return int(value)",
  "    if isinstance(value, str):",
  "        try:",
  "            return int(datetime.datetime.fromisoformat(value.replace('Z', '+00:00')).timestamp() * 1000)",
  "        except Exception:",
  "            return 0",
  "    return 0",
  "",
  "def stringify_timestamp(timestamp_ms):",
  "    if not isinstance(timestamp_ms, (int, float)) or timestamp_ms <= 0:",
  "        return ''",
  "    return datetime.datetime.utcfromtimestamp(int(timestamp_ms) / 1000).isoformat() + 'Z'",
  "",
  "def flatten_rich_text(node):",
  "    if isinstance(node, dict):",
  "        if isinstance(node.get('text'), str) and node.get('text').strip():",
  "            return node.get('text').strip()",
  "        pieces = []",
  "        for child in node.get('children', []) or []:",
  "            child_text = flatten_rich_text(child)",
  "            if child_text:",
  "                pieces.append(child_text)",
  "        return ' '.join(piece for piece in pieces if piece)",
  "    if isinstance(node, list):",
  "        pieces = []",
  "        for child in node:",
  "            child_text = flatten_rich_text(child)",
  "            if child_text:",
  "                pieces.append(child_text)",
  "        return ' '.join(piece for piece in pieces if piece)",
  "    return ''",
  "",
  "def extract_text(data):",
  "    text = (data.get('text') or '').strip()",
  "    if text:",
  "        return text",
  "    rich = data.get('richText')",
  "    if isinstance(rich, str) and rich.strip():",
  "        try:",
  "            rich_json = json.loads(rich)",
  "            flattened = flatten_rich_text(rich_json)",
  "            if flattened:",
  "                return flattened",
  "        except Exception:",
  "            return rich.strip()",
  "    return ''",
  "",
  "parsed = []",
  "for key, value in composer_rows:",
  "    try:",
  "        data = json.loads(value)",
  "    except Exception:",
  "        continue",
  "    parsed.append((normalize_created_at(data.get('createdAt')), key, data))",
  "parsed.sort(key=lambda item: item[0], reverse=True)",
  "",
  "recent = []",
  "for _, key, data in parsed:",
  "    composer_id = data.get('composerId') or key.split(':', 1)[1]",
  "    headers = data.get('fullConversationHeadersOnly') or []",
  "    first_user = ''",
  "    first_assistant = ''",
  "    latest_user = ''",
  "    latest_assistant = ''",
  "    latest_user_at = 0",
  "    latest_assistant_at = 0",
  "    latest_activity_at = normalize_created_at(data.get('createdAt'))",
  "    for header in headers:",
  "        if not isinstance(header, dict):",
  "            continue",
  "        bubble_id = header.get('bubbleId')",
  "        if not bubble_id:",
  "            continue",
  "        bubble_key = f'bubbleId:{composer_id}:{bubble_id}'",
  "        bubble_row = cur.execute(\"SELECT value FROM cursorDiskKV WHERE key = ?\", (bubble_key,)).fetchone()",
  "        if not bubble_row:",
  "            continue",
  "        try:",
  "            bubble = json.loads(bubble_row[0])",
  "        except Exception:",
  "            continue",
  "        bubble_text = extract_text(bubble)",
  "        bubble_type = bubble.get('type')",
  "        bubble_created_at = normalize_created_at(bubble.get('createdAt'))",
  "        if bubble_created_at > latest_activity_at:",
  "            latest_activity_at = bubble_created_at",
  "        if bubble_type == 1 and bubble_text and not first_user:",
  "            first_user = bubble_text",
  "        if bubble_type == 2 and bubble_text and not first_assistant:",
  "            first_assistant = bubble_text",
  "        if bubble_type == 1 and bubble_text and bubble_created_at >= latest_user_at:",
  "            latest_user = bubble_text",
  "            latest_user_at = bubble_created_at",
  "        if bubble_type == 2 and bubble_text and bubble_created_at >= latest_assistant_at:",
  "            latest_assistant = bubble_text",
  "            latest_assistant_at = bubble_created_at",
  "    if not headers and not first_user and not first_assistant:",
  "        continue",
  "    context_for_composer = cur.execute(\"SELECT COUNT(*) FROM cursorDiskKV WHERE key LIKE ?\", (f'messageRequestContext:{composer_id}:%',)).fetchone()[0]",
  "    created_at = data.get('createdAt')",
  "    if isinstance(created_at, (int, float)):",
  "        created_at = datetime.datetime.utcfromtimestamp(int(created_at) / 1000).isoformat() + 'Z'",
  "    elif not isinstance(created_at, str):",
  "        created_at = ''",
  "    recent.append({",
  "        'composerId': composer_id,",
  "        'createdAt': created_at,",
  "        'updatedAt': stringify_timestamp(latest_activity_at) or created_at,",
  "        'status': data.get('status') or '',",
  "        'isAgentic': bool(data.get('isAgentic')),",
  "        'headerCount': len(headers),",
  "        'contextCount': int(context_for_composer),",
  "        'firstUserText': first_user,",
  "        'firstAssistantText': first_assistant,",
  "        'latestUserText': latest_user,",
  "        'latestAssistantText': latest_assistant,",
  "        'latestUserAt': stringify_timestamp(latest_user_at),",
  "        'latestAssistantAt': stringify_timestamp(latest_assistant_at),",
  "    })",
  "recent.sort(key=lambda item: normalize_created_at(item.get('updatedAt') or item.get('createdAt')), reverse=True)",
  "recent = recent[:limit]",
  "",
  "print(json.dumps({",
  "    'composerCount': len(parsed),",
  "    'bubbleCount': int(bubble_count),",
  "    'contextCount': int(context_count),",
  "    'conversations': recent,",
  "}, ensure_ascii=False))",
].join("\n");

export async function probeCursorChatStorage(
  options: CursorChatStorageProbeOptions = {},
): Promise<CursorChatStorageReport> {
  const cursorUserRoot = resolveCursorUserRoot(options.cursorUserRoot);
  const globalStorageDbPath = path.join(cursorUserRoot, "globalStorage", "state.vscdb");
  const workspaceStorageRoot = path.join(cursorUserRoot, "workspaceStorage");
  const workspaceDbCount = countWorkspaceDbs(workspaceStorageRoot);

  if (!existsSync(globalStorageDbPath)) {
    return {
      checkedAt: new Date().toISOString(),
      cursorUserRoot,
      globalStorageDbPath,
      workspaceDbCount,
      backend: "unavailable",
      backendDetail: undefined,
      composerCount: 0,
      bubbleCount: 0,
      contextCount: 0,
      conversations: [],
      findings: ["globalStorage/state.vscdb를 찾지 못했습니다."],
      conclusions: {
        canReadStorage: false,
        summary: "Cursor globalStorage DB가 없어 로컬 채팅 reader를 확인하지 못했습니다.",
      },
    };
  }

  const backendResult = await runStorageProbe(globalStorageDbPath, options.maxConversations ?? DEFAULT_MAX_CONVERSATIONS);
  return {
    checkedAt: new Date().toISOString(),
    cursorUserRoot,
    globalStorageDbPath,
    workspaceDbCount,
    backend: backendResult.backend,
    backendDetail: backendResult.backendDetail,
    composerCount: backendResult.composerCount,
    bubbleCount: backendResult.bubbleCount,
    contextCount: backendResult.contextCount,
    conversations: backendResult.conversations,
    findings: [
      "globalStorage/state.vscdb의 cursorDiskKV에 composerData:* skeleton rows가 있습니다.",
      "bubbleId:<composerId>:<bubbleId> rows에서 실제 메시지 text/richText를 읽을 수 있습니다.",
      "messageRequestContext:<composerId>:<bubbleId> rows에는 terminal/file 같은 요청 맥락이 들어갑니다.",
    ],
    conclusions: {
      canReadStorage: true,
      summary:
        "Cursor regular chat는 globalStorage/state.vscdb를 통해 읽을 수 있는 구조로 보입니다. reader 1차 구현은 composerData + bubbleId + messageRequestContext 조합으로 가능합니다.",
    },
  };
}

export function formatCursorChatStorageReport(report: CursorChatStorageReport): string {
  const lines = [
    "VibeDeck Cursor Chat Storage Probe",
    `checked at: ${report.checkedAt}`,
    `cursor user root: ${report.cursorUserRoot}`,
    `global storage db: ${report.globalStorageDbPath}`,
    `workspace db count: ${report.workspaceDbCount}`,
    `backend: ${report.backend}`,
    `backend detail: ${report.backendDetail ?? "(none)"}`,
    `composerData rows: ${report.composerCount}`,
    `bubble rows: ${report.bubbleCount}`,
    `messageRequestContext rows: ${report.contextCount}`,
  ];

  if (report.conversations.length > 0) {
    lines.push("recent conversations:");
    for (const conversation of report.conversations) {
      lines.push(
        `- ${conversation.composerId} | created=${conversation.createdAt || "(no timestamp)"} | updated=${conversation.updatedAt || "(no timestamp)"} | status=${conversation.status || "(none)"} | agentic=${conversation.isAgentic ? "yes" : "no"} | headers=${conversation.headerCount} | context=${conversation.contextCount}`,
      );
      lines.push(`  user: ${truncatePreview(conversation.firstUserText)}`);
      lines.push(`  assistant: ${truncatePreview(conversation.firstAssistantText)}`);
      lines.push(`  latest user: ${truncatePreview(conversation.latestUserText)}`);
      lines.push(`  latest assistant: ${truncatePreview(conversation.latestAssistantText)}`);
    }
  }

  lines.push("findings:");
  for (const finding of report.findings) {
    lines.push(`- ${finding}`);
  }

  lines.push("conclusion:");
  lines.push(`- can read storage: ${report.conclusions.canReadStorage ? "yes" : "no"}`);
  lines.push(`- summary: ${report.conclusions.summary}`);
  return lines.join("\n");
}

function resolveCursorUserRoot(overridePath: string | undefined): string {
  if (overridePath?.trim()) {
    return overridePath.trim();
  }
  if (process.platform === "win32") {
    const appData = process.env.APPDATA;
    if (!appData) {
      throw new Error("APPDATA is not set.");
    }
    return path.join(appData, "Cursor", "User");
  }
  const home = process.env.HOME;
  if (!home) {
    throw new Error("HOME is not set.");
  }
  if (process.platform === "darwin") {
    return path.join(home, "Library", "Application Support", "Cursor", "User");
  }
  return path.join(home, ".config", "Cursor", "User");
}

function countWorkspaceDbs(workspaceStorageRoot: string): number {
  if (!existsSync(workspaceStorageRoot)) {
    return 0;
  }
  let count = 0;
  for (const entry of readdirSync(workspaceStorageRoot, { withFileTypes: true })) {
    if (!entry.isDirectory()) {
      continue;
    }
    if (existsSync(path.join(workspaceStorageRoot, entry.name, "state.vscdb"))) {
      count += 1;
    }
  }
  return count;
}

async function runStorageProbe(
  dbPath: string,
  maxConversations: number,
): Promise<StorageProbeBackendResult> {
  const nodeResult = await tryNodeSqliteProbe(dbPath, maxConversations);
  if (nodeResult) {
    return nodeResult;
  }
  return runPythonProbe(dbPath, maxConversations);
}

async function tryNodeSqliteProbe(
  dbPath: string,
  maxConversations: number,
): Promise<StorageProbeBackendResult | undefined> {
  try {
    const sqliteModule = (await import("node:sqlite")) as {
      DatabaseSync?: new (filename: string) => {
        prepare(sql: string): {
          all(...params: unknown[]): unknown[];
          get(...params: unknown[]): unknown;
        };
        close(): void;
      };
    };
    if (!sqliteModule.DatabaseSync) {
      return undefined;
    }

    const db = new sqliteModule.DatabaseSync(dbPath);
    try {
      const composerRows = db
        .prepare("SELECT key, value FROM cursorDiskKV WHERE key LIKE 'composerData:%'")
        .all() as SqliteRow[];
      const bubbleCount = readCount(db.prepare("SELECT COUNT(*) AS count FROM cursorDiskKV WHERE key LIKE 'bubbleId:%'").get());
      const contextCount = readCount(
        db.prepare("SELECT COUNT(*) AS count FROM cursorDiskKV WHERE key LIKE 'messageRequestContext:%'").get(),
      );
      const conversations = summarizeConversationsFromRows(db, composerRows, maxConversations);
      return {
        backend: "node_sqlite",
        backendDetail: "node:sqlite",
        composerCount: composerRows.length,
        bubbleCount,
        contextCount,
        conversations,
      };
    } finally {
      db.close();
    }
  } catch {
    return undefined;
  }
}

function summarizeConversationsFromRows(
  db: {
    prepare(sql: string): {
      all(...params: unknown[]): unknown[];
      get(...params: unknown[]): unknown;
    };
  },
  composerRows: readonly SqliteRow[],
  maxConversations: number,
): CursorChatStorageConversationSummary[] {
  const parsed = composerRows
    .map((row) => {
      try {
        return {
          key: row.key,
          data: JSON.parse(String(row.value)) as Record<string, unknown>,
        };
      } catch {
        return undefined;
      }
    })
    .filter((row): row is { key: string; data: Record<string, unknown> } => Boolean(row))
    .sort((left, right) => normalizeCreatedAt(right.data.createdAt) - normalizeCreatedAt(left.data.createdAt));

  const conversations: CursorChatStorageConversationSummary[] = [];
  for (const row of parsed) {
    const composerId = String(row.data.composerId ?? row.key.split(":", 2)[1] ?? "");
    const headers = Array.isArray(row.data.fullConversationHeadersOnly)
      ? (row.data.fullConversationHeadersOnly as Array<Record<string, unknown>>)
      : [];
    let firstUserText = "";
    let firstAssistantText = "";
    let latestUserText = "";
    let latestAssistantText = "";
    let latestUserAt = 0;
    let latestAssistantAt = 0;
    let latestActivityAt = normalizeCreatedAt(row.data.createdAt);

    for (const header of headers) {
      const bubbleId = typeof header?.bubbleId === "string" ? header.bubbleId : undefined;
      if (!bubbleId) {
        continue;
      }
      const bubbleKey = `bubbleId:${composerId}:${bubbleId}`;
      const bubbleRow = db.prepare("SELECT value FROM cursorDiskKV WHERE key = ?").get(bubbleKey) as
        | { value?: string }
        | undefined;
      if (!bubbleRow?.value) {
        continue;
      }
      let bubble: Record<string, unknown>;
      try {
        bubble = JSON.parse(String(bubbleRow.value));
      } catch {
        continue;
      }
      const bubbleText = extractBubbleText(bubble);
      const bubbleType = typeof bubble.type === "number" ? bubble.type : 0;
      const bubbleCreatedAt = normalizeCreatedAt(bubble.createdAt);
      if (bubbleCreatedAt > latestActivityAt) {
        latestActivityAt = bubbleCreatedAt;
      }
      if (bubbleType === 1 && bubbleText && !firstUserText) {
        firstUserText = bubbleText;
      }
      if (bubbleType === 2 && bubbleText && !firstAssistantText) {
        firstAssistantText = bubbleText;
      }
      if (bubbleType === 1 && bubbleText && bubbleCreatedAt >= latestUserAt) {
        latestUserText = bubbleText;
        latestUserAt = bubbleCreatedAt;
      }
      if (bubbleType === 2 && bubbleText && bubbleCreatedAt >= latestAssistantAt) {
        latestAssistantText = bubbleText;
        latestAssistantAt = bubbleCreatedAt;
      }
    }

    if (headers.length === 0 && !firstUserText && !firstAssistantText) {
      continue;
    }

    const contextCount = readCount(
      db.prepare("SELECT COUNT(*) AS count FROM cursorDiskKV WHERE key LIKE ?").get(
        `messageRequestContext:${composerId}:%`,
      ),
    );
    conversations.push({
      composerId,
      createdAt: stringifyCreatedAt(row.data.createdAt),
      updatedAt: stringifyCreatedAt(latestActivityAt || row.data.createdAt),
      status: typeof row.data.status === "string" ? row.data.status : "",
      isAgentic: Boolean(row.data.isAgentic),
      headerCount: headers.length,
      contextCount,
      firstUserText,
      firstAssistantText,
      latestUserText,
      latestAssistantText,
      latestUserAt: stringifyCreatedAt(latestUserAt),
      latestAssistantAt: stringifyCreatedAt(latestAssistantAt),
    });
  }

  return conversations
    .sort(
      (left, right) =>
        normalizeCreatedAt(right.updatedAt || right.createdAt) -
        normalizeCreatedAt(left.updatedAt || left.createdAt),
    )
    .slice(0, maxConversations);
}

function extractBubbleText(bubble: Record<string, unknown>): string {
  const text = typeof bubble.text === "string" ? bubble.text.trim() : "";
  if (text) {
    return text;
  }
  const richText = typeof bubble.richText === "string" ? bubble.richText.trim() : "";
  if (!richText) {
    return "";
  }
  try {
    return flattenRichText(JSON.parse(richText));
  } catch {
    return richText;
  }
}

function flattenRichText(value: unknown): string {
  if (Array.isArray(value)) {
    return value.map((item) => flattenRichText(item)).filter(Boolean).join(" ").trim();
  }
  if (!value || typeof value !== "object") {
    return "";
  }
  const record = value as Record<string, unknown>;
  const text = typeof record.text === "string" ? record.text.trim() : "";
  if (text) {
    return text;
  }
  const children = Array.isArray(record.children) ? record.children : [];
  return children.map((child) => flattenRichText(child)).filter(Boolean).join(" ").trim();
}

function readCount(value: unknown): number {
  if (!value || typeof value !== "object") {
    return 0;
  }
  const count = (value as Record<string, unknown>).count;
  return typeof count === "number" ? count : 0;
}

function normalizeCreatedAt(value: unknown): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return Math.trunc(value);
  }
  if (typeof value === "string" && value.trim()) {
    const parsed = Date.parse(value);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

function stringifyCreatedAt(value: unknown): string {
  if (typeof value === "number" && Number.isFinite(value)) {
    return new Date(value).toISOString();
  }
  return typeof value === "string" ? value : "";
}

function runPythonProbe(dbPath: string, maxConversations: number): StorageProbeBackendResult {
  const commands = process.platform === "win32"
    ? [
        { command: "python", args: ["-c", PYTHON_SCRIPT, dbPath, String(maxConversations)] },
        { command: "py", args: ["-3", "-c", PYTHON_SCRIPT, dbPath, String(maxConversations)] },
      ]
    : [
        { command: "python3", args: ["-c", PYTHON_SCRIPT, dbPath, String(maxConversations)] },
        { command: "python", args: ["-c", PYTHON_SCRIPT, dbPath, String(maxConversations)] },
      ];

  let lastError = "python command is not available";
  for (const candidate of commands) {
    const result = spawnSync(candidate.command, candidate.args, {
      encoding: "utf8",
      windowsHide: true,
    });
    if (result.error) {
      lastError = result.error.message;
      continue;
    }
    if (result.status !== 0) {
      lastError = [result.stdout, result.stderr].filter(Boolean).join("\n") || `exit ${result.status}`;
      continue;
    }
    const stdout = (result.stdout ?? "").trim();
    if (!stdout) {
      lastError = "python probe returned empty output";
      continue;
    }
    const parsed = JSON.parse(stdout) as Omit<StorageProbeBackendResult, "backend" | "backendDetail">;
    return {
      backend: "python",
      backendDetail: candidate.command,
      composerCount: parsed.composerCount,
      bubbleCount: parsed.bubbleCount,
      contextCount: parsed.contextCount,
      conversations: parsed.conversations,
    };
  }

  throw new Error(`Cursor chat storage probe failed: ${lastError}`);
}

function truncatePreview(value: string): string {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (!normalized) {
    return "(empty)";
  }
  return normalized.length > 140 ? `${normalized.slice(0, 137)}...` : normalized;
}
