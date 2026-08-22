import { beforeEach, describe, expect, it, vi } from "vitest";
import { companionScopeStorageKey, readStoredCompanionScope, storeCompanionScope } from "./companionScope";

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

  it("stays quiet when storage is unavailable", () => {
    vi.spyOn(window.localStorage.__proto__, "setItem").mockImplementation(() => {
      throw new Error("quota exceeded");
    });
    vi.spyOn(window.localStorage.__proto__, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });

    expect(() => storeCompanionScope("p1", "project")).not.toThrow();
    expect(readStoredCompanionScope("p1")).toBeNull();
  });
});
