import { afterEach, describe, expect, it } from "vitest";
import { hasSecureKeyStore, keyStoreKind, keyStoreLabelKey } from "./secretStore";

const original = Object.getOwnPropertyDescriptor(navigator, "platform");

function setPlatform(value: string) {
  Object.defineProperty(navigator, "platform", { value, configurable: true });
}

afterEach(() => {
  if (original) Object.defineProperty(navigator, "platform", original);
});

describe("keyStoreKind", () => {
  // Mirrors defaultSecretStore() in engine/internal/settings: Keychain on
  // darwin, Credential Manager on windows, nothing anywhere else.
  it("maps each platform to the backend the engine actually compiles in", () => {
    setPlatform("MacIntel");
    expect(keyStoreKind()).toBe("macos");

    setPlatform("Win32");
    expect(keyStoreKind()).toBe("windows");

    setPlatform("Linux x86_64");
    expect(keyStoreKind()).toBe("none");
  });

  it("names the store for supported platforms and refuses to for Linux", () => {
    setPlatform("MacIntel");
    expect(keyStoreLabelKey()).toBe("settings.keyStore.macos");

    setPlatform("Win32");
    expect(keyStoreLabelKey()).toBe("settings.keyStore.windows");

    setPlatform("Linux x86_64");
    expect(keyStoreLabelKey()).toBeNull();
  });

  it("treats macOS and Windows as having a secure store", () => {
    setPlatform("MacIntel");
    expect(hasSecureKeyStore()).toBe(true);

    setPlatform("Win32");
    expect(hasSecureKeyStore()).toBe(true);

    setPlatform("Linux x86_64");
    expect(hasSecureKeyStore()).toBe(false);
  });
});
