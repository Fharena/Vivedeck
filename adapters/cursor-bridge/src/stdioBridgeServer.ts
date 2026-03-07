import readline from "node:readline";

import { handleBridgeLine } from "./bridgeRpc.js";
import type { BridgeResponse } from "./stdioProtocol.js";
import type { WorkspaceAdapter } from "./types.js";

export function serveStdioBridge(adapter: WorkspaceAdapter): void {
  process.stdin.setEncoding("utf8");

  const lineReader = readline.createInterface({
    input: process.stdin,
    crlfDelay: Infinity,
  });

  lineReader.on("line", (line) => {
    void handleBridgeLine(adapter, line, writeResponse);
  });

  lineReader.on("close", () => {
    process.exit(0);
  });
}

function writeResponse(response: BridgeResponse): void {
  process.stdout.write(`${JSON.stringify(response)}\n`);
}
