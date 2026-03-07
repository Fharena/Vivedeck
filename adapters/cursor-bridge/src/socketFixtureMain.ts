import { createFixtureCursorBridge } from "./fixtureCursorBridge.js";
import { serveSocketBridge } from "./socketBridgeServer.js";

const port = parsePort(process.env.CURSOR_BRIDGE_TCP_PORT, 7797);
const host = process.env.CURSOR_BRIDGE_TCP_HOST?.trim() || "127.0.0.1";

try {
  const server = await serveSocketBridge(createFixtureCursorBridge(), { host, port });
  console.error(`fixture socket bridge listening on ${server.address}`);
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}

function parsePort(value: string | undefined, fallback: number): number {
  if (!value) {
    return fallback;
  }

  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return fallback;
  }
  return parsed;
}
