import { createFixtureCursorBridge } from "./fixtureCursorBridge.js";
import { serveStdioBridge } from "./stdioBridgeServer.js";

serveStdioBridge(createFixtureCursorBridge());