export type BridgeMethod =
  | "name"
  | "capabilities"
  | "getContext"
  | "submitTask"
  | "getPatch"
  | "applyPatch"
  | "runProfile"
  | "getRunResult"
  | "openLocation";

export interface BridgeRequest {
  id: string;
  method: BridgeMethod | string;
  params?: unknown;
}

export interface BridgeErrorBody {
  message: string;
}

export interface BridgeResponse {
  id: string;
  result?: unknown;
  error?: BridgeErrorBody;
}