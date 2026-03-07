import net from "node:net";
import readline from "node:readline";

import { handleBridgeLine } from "./bridgeRpc.js";
import type { BridgeResponse } from "./stdioProtocol.js";
import type { WorkspaceAdapter } from "./types.js";

export interface SocketBridgeOptions {
  host?: string;
  port: number;
}

export interface SocketBridgeServer {
  host: string;
  port: number;
  address: string;
  close(): Promise<void>;
}

export async function serveSocketBridge(
  adapter: WorkspaceAdapter,
  options: SocketBridgeOptions,
): Promise<SocketBridgeServer> {
  const host = normalizeHost(options.host);
  const sockets = new Set<net.Socket>();
  const server = net.createServer((socket) => {
    sockets.add(socket);
    socket.setEncoding("utf8");
    socket.setNoDelay(true);
    socket.on("close", () => {
      sockets.delete(socket);
    });
    socket.on("error", () => {
      socket.destroy();
    });

    const lineReader = readline.createInterface({
      input: socket,
      crlfDelay: Infinity,
    });

    lineReader.on("line", (line) => {
      void handleBridgeLine(adapter, line, (response) => {
        writeSocketResponse(socket, response);
      });
    });

    lineReader.on("close", () => {
      socket.destroy();
    });
  });

  await listen(server, host, options.port);

  const addressInfo = server.address();
  if (!addressInfo || typeof addressInfo === "string") {
    throw new Error("socket bridge failed to resolve address");
  }

  return {
    host: addressInfo.address,
    port: addressInfo.port,
    address: `${addressInfo.address}:${addressInfo.port}`,
    async close(): Promise<void> {
      for (const socket of sockets) {
        socket.destroy();
      }
      await closeServer(server);
    },
  };
}

function writeSocketResponse(socket: net.Socket, response: BridgeResponse): void {
  if (socket.destroyed) {
    return;
  }
  socket.write(`${JSON.stringify(response)}\n`);
}

function normalizeHost(host?: string): string {
  const trimmed = host?.trim();
  if (!trimmed) {
    return "127.0.0.1";
  }
  return trimmed;
}

function listen(server: net.Server, host: string, port: number): Promise<void> {
  return new Promise((resolve, reject) => {
    const onError = (error: Error) => {
      server.off("listening", onListening);
      reject(error);
    };
    const onListening = () => {
      server.off("error", onError);
      resolve();
    };

    server.once("error", onError);
    server.once("listening", onListening);
    server.listen(port, host);
  });
}

function closeServer(server: net.Server): Promise<void> {
  return new Promise((resolve, reject) => {
    server.close((error) => {
      if (error) {
        reject(error);
        return;
      }
      resolve();
    });
  });
}
