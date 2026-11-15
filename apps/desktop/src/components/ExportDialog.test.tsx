import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  document: vi.fn(),
  project: vi.fn(),
  node: vi.fn(),
  saveFile: vi.fn(),
  saveMarkdown: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  exportApi: { document: mocks.document, project: mocks.project, node: mocks.node },
}));

vi.mock("../lib/exportSave", () => ({
  saveExportedFile: mocks.saveFile,
  saveExportedMarkdown: mocks.saveMarkdown,
  exportDestinationMessage: (_t: unknown, path: string) => `saved ${path}`,
}));

vi.mock("../lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.entries(vars).map(([k, v]) => `${k}=${v}`).join(",")}` : key,
  }),
}));

import { ExportDialog, type ExportScope } from "./ExportDialog";

const SCOPES: ExportScope[] = [
  { label: "work" },
  { nodeId: "episode-1", label: "1화 전체" },
  { nodeId: "scene-1", label: "이 장면", leaf: true },
];

function renderDialog(onMessage = vi.fn(), onClose = vi.fn()) {
  render(<ExportDialog projectId="p1" scopes={SCOPES} onClose={onClose} onMessage={onMessage} />);
  return { onMessage, onClose };
}

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  mocks.document.mockImplementation((req: { preview?: boolean; node_id?: string }) =>
    Promise.resolve(req.preview
      ? { scenes: req.node_id ? 2 : 9, chars: 100 }
      : { scenes: 2, chars: 100, bytes_base64: "UEs=", suggested_filename: "w.docx", mime: "x" }));
  mocks.saveFile.mockResolvedValue("/tmp/w.docx");
  mocks.saveMarkdown.mockResolvedValue("/tmp/w.md");
});

describe("ExportDialog", () => {
  it("counts the chosen scope and exports it as Word with the layout choices", async () => {
    const { onMessage, onClose } = renderDialog();
    expect(await screen.findByText("exportDialog.summary:scenes=9,chars=100")).toBeTruthy();

    fireEvent.click(screen.getByTestId("export-scope-1"));
    expect(await screen.findByText("exportDialog.summary:scenes=2,chars=100")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("exportDialog.pageBreak"));
    await act(async () => {
      fireEvent.click(screen.getByTestId("export-run"));
    });

    expect(mocks.document).toHaveBeenLastCalledWith({
      project_id: "p1",
      node_id: "episode-1",
      format: "docx",
      options: expect.objectContaining({ font: "함초롬바탕", size_pt: 10, line_spacing_pct: 160, page_break_per_episode: false }),
    });
    expect(mocks.saveFile).toHaveBeenCalledWith(expect.objectContaining({ suggested_filename: "w.docx" }), "docx");
    expect(onMessage).toHaveBeenCalledWith("saved /tmp/w.docx");
    expect(onClose).toHaveBeenCalled();
  });

  it("offers Markdown only for the whole work or a scene", async () => {
    renderDialog();
    fireEvent.click(screen.getByTestId("export-scope-1"));
    expect((screen.getByTestId("export-format-md") as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByTestId("export-scope-2"));
    expect((screen.getByTestId("export-format-md") as HTMLButtonElement).disabled).toBe(false);

    fireEvent.click(screen.getByTestId("export-format-md"));
    expect(screen.queryByTestId("export-layout")).toBeNull();
    await act(async () => {
      fireEvent.click(screen.getByTestId("export-run"));
    });
    expect(mocks.node).toHaveBeenCalledWith("scene-1");
    expect(mocks.saveMarkdown).toHaveBeenCalled();
  });

  it("remembers the format and layout for the work", async () => {
    renderDialog();
    fireEvent.click(screen.getByTestId("export-format-txt"));
    expect(screen.queryByTestId("export-layout")).toBeNull();
    fireEvent.click(screen.getByLabelText("exportDialog.illustrations"));
    await waitFor(() => expect(window.localStorage.getItem("linetta.exportOptions.p1")).toContain('"illustrations":true'));
  });

  it("says why an export failed and stays open", async () => {
    mocks.document.mockImplementation((req: { preview?: boolean }) =>
      req.preview ? Promise.resolve({ scenes: 1, chars: 1 }) : Promise.reject(new Error("boom")));
    const { onMessage, onClose } = renderDialog();
    await act(async () => {
      fireEvent.click(screen.getByTestId("export-run"));
    });
    expect(onMessage).toHaveBeenCalledWith("exportDialog.failed:error=Error: boom");
    expect(onClose).not.toHaveBeenCalled();
  });
});
