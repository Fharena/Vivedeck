import { existsSync } from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";

export interface CursorChatStorageBubbleDetail {
  readonly composerId: string;
  readonly bubbleId: string;
  readonly type: number;
  readonly createdAt: string;
  readonly text: string;
  readonly context: Record<string, unknown>;
}

export interface CursorChatStorageConversationDetail {
  readonly composerId: string;
  readonly createdAt: string;
  readonly updatedAt: string;
  readonly status: string;
  readonly isAgentic: boolean;
  readonly bubbles: readonly CursorChatStorageBubbleDetail[];
}

export interface CursorChatStorageConversationReadOptions {
  readonly composerId: string;
  readonly cursorUserRoot?: string;
}

interface SqliteRow {
  readonly value?: string;
}

const PYTHON_DETAIL_SCRIPT = [
  "import json, sqlite3, sys, datetime",
  "db_path = sys.argv[1]",
  "composer_id = sys.argv[2]",
  "conn = sqlite3.connect(db_path)",
  "cur = conn.cursor()",
  "row = cur.execute(\"SELECT value FROM cursorDiskKV WHERE key = ?\", (f'composerData:{composer_id}',)).fetchone()",
  "if not row:",
  "    print(json.dumps({'found': False}))",
  "    sys.exit(0)",
  "data = json.loads(row[0])",
  "headers = data.get('fullConversationHeadersOnly') or []",
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
  "        text = node.get('text')",
  "        if isinstance(text, str) and text.strip():",
  "            return text.strip()",
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
  "def extract_text(item):",
  "    text = (item.get('text') or '').strip()",
  "    if text:",
  "        return text",
  "    rich = item.get('richText')",
  "    if isinstance(rich, str) and rich.strip():",
  "        try:",
  "            return flatten_rich_text(json.loads(rich))",
  "        except Exception:",
  "            return rich.strip()",
  "    return ''",
  "",
  "bubbles = []",
  "latest_at = normalize_created_at(data.get('createdAt'))",
  "for header in headers:",
  "    if not isinstance(header, dict):",
  "        continue",
  "    bubble_id = header.get('bubbleId')",
  "    if not bubble_id:",
  "        continue",
  "    bubble_row = cur.execute(\"SELECT value FROM cursorDiskKV WHERE key = ?\", (f'bubbleId:{composer_id}:{bubble_id}',)).fetchone()",
  "    if not bubble_row:",
  "        continue",
  "    bubble = json.loads(bubble_row[0])",
  "    context_row = cur.execute(\"SELECT value FROM cursorDiskKV WHERE key = ?\", (f'messageRequestContext:{composer_id}:{bubble_id}',)).fetchone()",
  "    context = {}",
  "    if context_row and context_row[0]:",
  "        try:",
  "            context = json.loads(context_row[0])",
  "        except Exception:",
  "            context = {}",
  "    created_at = bubble.get('createdAt')",
  "    created_at_ms = normalize_created_at(created_at)",
  "    if created_at_ms > latest_at:",
  "        latest_at = created_at_ms",
  "    if isinstance(created_at, (int, float)):",
  "        created_at = stringify_timestamp(created_at_ms)",
  "    elif not isinstance(created_at, str):",
  "        created_at = ''",
  "    bubbles.append({",
  "        'composerId': composer_id,",
  "        'bubbleId': str(bubble_id),",
  "        'type': int(bubble.get('type') or header.get('type') or 0),",
  "        'createdAt': created_at,",
  "        'text': extract_text(bubble),",
  "        'context': context if isinstance(context, dict) else {},",
  "    })",
  "",
  "created_at = data.get('createdAt')",
  "if isinstance(created_at, (int, float)):",
  "    created_at = stringify_timestamp(normalize_created_at(created_at))",
  "elif not isinstance(created_at, str):",
  "    created_at = ''",
  "",
  "print(json.dumps({",
  "    'found': True,",
  "    'composerId': composer_id,",
  "    'createdAt': created_at,",
  "    'updatedAt': stringify_timestamp(latest_at) or created_at,",
  "    'status': data.get('status') or '',",
  "    'isAgentic': bool(data.get('isAgentic')),",
  "    'bubbles': bubbles,",
  "}, ensure_ascii=False))",
].join("\n");

export async function readCursorChatConversation(
  options: CursorChatStorageConversationReadOptions,
): Promise<CursorChatStorageConversationDetail | undefined> {
  const composerId = options.composerId.trim();
  if (!composerId) {
    throw new Error("composerId is required");
  }

  const cursorUserRoot = resolveCursorUserRoot(options.cursorUserRoot);
  const dbPath = path.join(cursorUserRoot, "globalStorage", "state.vscdb");
  if (!existsSync(dbPath)) {
    return undefined;
  }

  const nodeResult = await tryNodeSqliteRead(dbPath, composerId);
  if (nodeResult) {
    return nodeResult;
  }
  return runPythonRead(dbPath, composerId);
}

async function tryNodeSqliteRead(
  dbPath: string,
  composerId: string,
): Promise<CursorChatStorageConversationDetail | undefined> {
  try {
    const sqliteModule = (await import("node:sqlite")) as {
      DatabaseSync?: new (filename: string) => {
        prepare(sql: string): {
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
      return readConversationFromDatabase({
        prepare(sql: string) {
          return db.prepare(sql);
        },
      }, composerId);
    } finally {
      db.close();
    }
  } catch {
    return undefined;
  }
}

function readConversationFromDatabase(
  db: {
    prepare(sql: string): {
      get(...params: unknown[]): unknown;
    };
  },
  composerId: string,
): CursorChatStorageConversationDetail | undefined {
  const composerRow = db.prepare("SELECT value FROM cursorDiskKV WHERE key = ?").get(
    `composerData:${composerId}`,
  ) as SqliteRow | undefined;
  if (!composerRow?.value) {
    return undefined;
  }

  const composer = JSON.parse(String(composerRow.value)) as Record<string, unknown>;
  const headers = Array.isArray(composer.fullConversationHeadersOnly)
    ? (composer.fullConversationHeadersOnly as Array<Record<string, unknown>>)
    : [];

  const bubbles: CursorChatStorageBubbleDetail[] = [];
  let latestAt = normalizeCreatedAt(composer.createdAt);
  for (const header of headers) {
    const bubbleId = typeof header?.bubbleId === "string" ? header.bubbleId : "";
    if (!bubbleId) {
      continue;
    }

    const bubbleRow = db.prepare("SELECT value FROM cursorDiskKV WHERE key = ?").get(
      `bubbleId:${composerId}:${bubbleId}`,
    ) as SqliteRow | undefined;
    if (!bubbleRow?.value) {
      continue;
    }

    const bubble = JSON.parse(String(bubbleRow.value)) as Record<string, unknown>;
    const contextRow = db.prepare("SELECT value FROM cursorDiskKV WHERE key = ?").get(
      `messageRequestContext:${composerId}:${bubbleId}`,
    ) as SqliteRow | undefined;
    const context = parseContextValue(contextRow?.value);
    const createdAt = stringifyCreatedAt(bubble.createdAt);
    const createdAtMs = normalizeCreatedAt(bubble.createdAt);
    if (createdAtMs > latestAt) {
      latestAt = createdAtMs;
    }

    bubbles.push({
      composerId,
      bubbleId,
      type: typeof bubble.type === "number" ? bubble.type : normalizeBubbleType(header.type),
      createdAt,
      text: extractBubbleText(bubble),
      context,
    });
  }

  return {
    composerId,
    createdAt: stringifyCreatedAt(composer.createdAt),
    updatedAt: stringifyCreatedAt(latestAt || composer.createdAt),
    status: typeof composer.status === "string" ? composer.status : "",
    isAgentic: Boolean(composer.isAgentic),
    bubbles,
  };
}

function runPythonRead(
  dbPath: string,
  composerId: string,
): CursorChatStorageConversationDetail | undefined {
  const commands =
    process.platform === "win32"
      ? [
          { command: "python", args: ["-c", PYTHON_DETAIL_SCRIPT, dbPath, composerId] },
          { command: "py", args: ["-3", "-c", PYTHON_DETAIL_SCRIPT, dbPath, composerId] },
        ]
      : [
          { command: "python3", args: ["-c", PYTHON_DETAIL_SCRIPT, dbPath, composerId] },
          { command: "python", args: ["-c", PYTHON_DETAIL_SCRIPT, dbPath, composerId] },
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
      lastError = "python reader returned empty output";
      continue;
    }

    const parsed = JSON.parse(stdout) as { found?: boolean } & CursorChatStorageConversationDetail;
    if (parsed.found === false) {
      return undefined;
    }
    return {
      composerId: parsed.composerId,
      createdAt: parsed.createdAt,
      updatedAt: parsed.updatedAt,
      status: parsed.status,
      isAgentic: parsed.isAgentic,
      bubbles: Array.isArray(parsed.bubbles)
        ? parsed.bubbles.map((bubble) => ({
            composerId: String((bubble as CursorChatStorageBubbleDetail).composerId ?? composerId),
            bubbleId: String((bubble as CursorChatStorageBubbleDetail).bubbleId ?? ""),
            type: Number((bubble as CursorChatStorageBubbleDetail).type ?? 0),
            createdAt: String((bubble as CursorChatStorageBubbleDetail).createdAt ?? ""),
            text: String((bubble as CursorChatStorageBubbleDetail).text ?? ""),
            context:
              bubble && typeof bubble === "object" && (bubble as CursorChatStorageBubbleDetail).context
                ? { ...((bubble as CursorChatStorageBubbleDetail).context ?? {}) }
                : {},
          }))
        : [],
    };
  }

  throw new Error(`Cursor chat storage reader failed: ${lastError}`);
}

function parseContextValue(raw: string | undefined): Record<string, unknown> {
  if (!raw) {
    return {};
  }
  try {
    const parsed = JSON.parse(String(raw));
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    return {};
  }
  return {};
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
  if (typeof value === "string") {
    return value;
  }
  return "";
}

function normalizeBubbleType(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? Math.trunc(value) : 0;
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
