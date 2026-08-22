import { beforeEach, describe, expect, it, vi } from "vitest";
import { companionScopeStorageKey, readStoredCompanionScope, storeCompanionScope } from "./companionScope";

// The Storage prototype that owns getItem/setItem differs between jsdom and
// Node's own web storage, so the failure cases replace the property itself.
function withStorage(descriptor: PropertyDescriptor, run: () => void) {
  const own = Object.getOwnPropertyDescriptor(window, "localStorage");
  Object.defineProperty(window, "localStorage", { configurable: true, ...descriptor });
  try {
    run();
  } finally {
    if (own) Object.defineProperty(window, "localStorage", own);
    else delete (window as { localStorage?: Storage }).localStorage;
  }
}

describe("companion scope memory", () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.restoreAllMocks();
  });

  it("remembers the last scope per project", () => {
    storeCompanionScope("p1", "project");
    storeCompanionScope("p2", "scene");

    expect(readStoredCompanionScope("p1")).toBe("project");
    expect(readStoredCompanionScope("p2")).toBe("scene");
  });

  it("returns null for an unknown project so the caller can pick its own default", () => {
    expect(readStoredCompanionScope("never-opened")).toBeNull();
    expect(readStoredCompanionScope("")).toBeNull();
  });

  it("ignores a stored value that is not a scope", () => {
    window.localStorage.setItem(companionScopeStorageKey("p1"), "global");

    expect(readStoredCompanionScope("p1")).toBeNull();
  });

  it("stays quiet when reading storage throws", () => {
    // Private mode can make the property itself throw on access.
    withStorage({ get() { throw new Error("storage blocked"); } }, () => {
      expect(() => storeCompanionScope("p1", "project")).not.toThrow();
      expect(readStoredCompanionScope("p1")).toBeNull();
    });
  });

  it("stays quiet when storage calls throw", () => {
    const broken = {
      getItem() { throw new Error("blocked"); },
      setItem() { throw new Error("quota exceeded"); },
    } as unknown as Storage;

    withStorage({ value: broken }, () => {
      expect(() => storeCompanionScope("p1", "project")).not.toThrow();
      expect(readStoredCompanionScope("p1")).toBeNull();
    });
  });
});
