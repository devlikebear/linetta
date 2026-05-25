import { invoke } from "@tauri-apps/api/core";

export async function enginePing(): Promise<string> {
  return invoke<string>("engine_ping");
}
