import { describe, expect, it } from "vitest";
import { offeredProviders, takesApiKey } from "./providers";
import type { ProviderStatus } from "./types";

const row = (id: ProviderStatus["id"]): ProviderStatus => ({
  id,
  auth: "api_key",
  active: false,
  configured: false,
  consented: false,
});

describe("offeredProviders", () => {
  it("follows the engine's rows, in the fixed order", () => {
    const ids = offeredProviders([row("claude-code"), row("anthropic"), row("openai-codex")]).map((p) => p.id);
    expect(ids).toEqual(["openai-codex", "anthropic", "claude-code"]);
  });

  it("holds the CLI back until the engine has said whether this build has it", () => {
    expect(offeredProviders([]).map((p) => p.id)).not.toContain("claude-code");
  });
});

describe("takesApiKey", () => {
  it("is false for the providers that sign in some other way", () => {
    expect(takesApiKey("openai-codex")).toBe(false);
    expect(takesApiKey("claude-code")).toBe(false);
    expect(takesApiKey("anthropic")).toBe(true);
    expect(takesApiKey("openai")).toBe(true);
  });
});
