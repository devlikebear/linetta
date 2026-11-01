import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNodeViewProps } from "@tiptap/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ get: vi.fn(), items: vi.fn() }));

vi.mock("@tiptap/react", () => ({
  NodeViewWrapper: ({ children, as: _as, ...rest }: { children: ReactNode; as?: string }) => <figure {...rest}>{children}</figure>,
}));
vi.mock("../../lib/rpc", () => ({
  illustrations: { get: mocks.get },
  vaultApi: { items: mocks.items },
}));
vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({ t: (key: string) => key }),
  localeForLanguage: () => "ko-KR",
}));

import { forgetImagesOmitted } from "../../lib/vault";
import { IllustrationNodeView } from "./IllustrationNodeView";

function renderNode() {
  const props = { node: { attrs: { id: "i1", caption: "" } }, updateAttributes: vi.fn(), selected: false };
  render(<IllustrationNodeView {...(props as unknown as ReactNodeViewProps)} />);
}

describe("IllustrationNodeView", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    forgetImagesOmitted();
    mocks.get.mockRejectedValue(new Error("not found"));
  });

  it("says a missing picture was deleted when pictures travel", async () => {
    mocks.items.mockResolvedValue({ categories: [{ id: "images", bytes: 0, enabled: true, required: false }] });
    renderNode();
    expect(await screen.findByText("illustration.missing")).toBeInTheDocument();
  });

  it("says a missing picture is not on this device when pictures are left out", async () => {
    mocks.items.mockResolvedValue({ categories: [{ id: "images", bytes: 0, enabled: false, required: false }] });
    renderNode();
    expect(await screen.findByText("illustration.notOnDevice")).toBeInTheDocument();
  });

  it("falls back to deleted on a build with no vault", async () => {
    mocks.items.mockRejectedValue(new Error("method not found"));
    renderNode();
    expect(await screen.findByText("illustration.missing")).toBeInTheDocument();
  });

  it("does not ask about sync for a picture that loads", async () => {
    mocks.get.mockResolvedValue({ data_base64: "AA==", mime: "image/png" });
    URL.createObjectURL = vi.fn(() => "blob:x");
    URL.revokeObjectURL = vi.fn();
    renderNode();
    await waitFor(() => expect(document.querySelector("img")).not.toBeNull());
    expect(mocks.items).not.toHaveBeenCalled();
  });
});
