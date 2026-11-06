import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { invoke } from "@tauri-apps/api/core";
import { Import } from "./Import";

const preview = {
  total: 150,
  contexts: [
    { platform: "x", kind: "post", count: 112 },
    { platform: "x", kind: "reply", count: 38 },
  ],
  skipped: { retweet: 4, short: 7, duplicate: 0 },
  preview: [{ body: "첫 번째 글", platform: "x", kind: "post", source: "import", external_id: "", posted_at: 0 }],
};

describe("Import", () => {
  beforeEach(() => {
    vi.mocked(invoke).mockReset();
    vi.mocked(invoke).mockImplementation(async (_cmd, args) => {
      const { method } = args as { method: string };
      if (method === "posts.import_preview") return preview;
      if (method === "posts.import") return { inserted: 150, skipped: 3 };
      throw new Error(`unexpected ${method}`);
    });
  });

  it("shows the preview counts and imports", async () => {
    render(<MemoryRouter><Import /></MemoryRouter>);
    const user = userEvent.setup();
    const file = new File(["[]"], "x-posts.json", { type: "application/json" });
    await user.upload(screen.getByLabelText("파일"), file);

    expect(await screen.findByText("가져올 글 150편")).toBeInTheDocument();
    expect(screen.getByText("X 글")).toBeInTheDocument();
    expect(screen.getByText("112")).toBeInTheDocument();
    expect(screen.getByText("X 답글")).toBeInTheDocument();
    expect(screen.getByText("38")).toBeInTheDocument();
    expect(screen.getByText("리트윗")).toBeInTheDocument();
    expect(screen.getByText("너무 짧음")).toBeInTheDocument();
    expect(screen.getByText("첫 번째 글")).toBeInTheDocument();

    expect(invoke).toHaveBeenCalledWith("engine_call", {
      method: "posts.import_preview",
      params: { file_name: "x-posts.json", content: "[]", default_kind: "post", platform: "x", own_user_id: "" },
    });

    await user.click(screen.getByRole("button", { name: "가져오기" }));
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("150편 가져옴, 3편 건너뜀"));
    expect(screen.getByRole("link", { name: "내 말투 보기" })).toHaveAttribute("href", "/voice");
    expect(invoke).toHaveBeenCalledWith("engine_call", expect.objectContaining({ method: "posts.import" }));
  });

  it("re-runs the preview when the default kind changes", async () => {
    render(<MemoryRouter><Import /></MemoryRouter>);
    const user = userEvent.setup();
    await user.upload(screen.getByLabelText("파일"), new File(["x"], "a.txt"));
    await screen.findByText("가져올 글 150편");
    await user.selectOptions(screen.getByLabelText("기본 종류"), "reply");
    await waitFor(() =>
      expect(invoke).toHaveBeenLastCalledWith("engine_call", {
        method: "posts.import_preview",
        params: expect.objectContaining({ default_kind: "reply" }),
      }),
    );
  });
});
