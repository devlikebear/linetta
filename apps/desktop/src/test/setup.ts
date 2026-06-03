import "@testing-library/jest-dom/vitest";
import { vi } from "vitest";

vi.mock("@tauri-apps/api/core", () => ({
  invoke: vi.fn(),
}));

vi.mock("@tauri-apps/plugin-dialog", () => ({
  open: vi.fn(),
  save: vi.fn(),
}));

vi.mock("@tauri-apps/plugin-fs", () => ({
  readTextFile: vi.fn(),
  writeTextFile: vi.fn(),
}));

Object.defineProperty(window, "scrollTo", {
  value: vi.fn(),
  writable: true,
});

Object.defineProperty(HTMLElement.prototype, "scrollTo", {
  value: vi.fn(),
  writable: true,
});

const localStorageStore = new Map<string, string>();
const localStorageMock = {
  get length() {
    return localStorageStore.size;
  },
  clear: vi.fn(() => localStorageStore.clear()),
  getItem: vi.fn((key: string) => localStorageStore.get(key) ?? null),
  key: vi.fn((index: number) => Array.from(localStorageStore.keys())[index] ?? null),
  removeItem: vi.fn((key: string) => { localStorageStore.delete(key); }),
  setItem: vi.fn((key: string, value: string) => { localStorageStore.set(key, String(value)); }),
};

if (typeof window.localStorage?.getItem !== "function") {
  Object.defineProperty(window, "localStorage", {
    value: localStorageMock,
    writable: true,
  });
}

class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}

Object.defineProperty(window, "ResizeObserver", {
  value: ResizeObserverMock,
  writable: true,
});
