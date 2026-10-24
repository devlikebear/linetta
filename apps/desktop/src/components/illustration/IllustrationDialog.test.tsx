import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  providers: vi.fn(),
  entities: vi.fn(),
  mentions: vi.fn(),
  compose: vi.fn(),
  generate: vi.fn(),
  get: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  imageProviders: { list: rpc.providers },
  entities: { list: rpc.entities },
  mentions: { listForNode: rpc.mentions },
  illustrations: { compose: rpc.compose, generate: rpc.generate, get: rpc.get },
}));

vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars
        ? `${key}:${Object.entries(vars)
            .map(([name, value]) => `${name}=${value}`)
            .join(",")}`
        : key,
  }),
}));

import { IllustrationDialog } from "./IllustrationDialog";

const ready = [{ id: "gemini-native", active: true, configured: true, consented: true, model: "gemini-3.1-flash-image" }];
const cast = [
  { id: "e1", kind: "character", name: "해진" },
  { id: "e2", kind: "character", name: "민호" },
  { id: "p1", kind: "place", name: "서울" },
];

function renderDialog(extra: Partial<Parameters<typeof IllustrationDialog>[0]> = {}) {
  const props = { projectId: "p", onClose: vi.fn(), onSaved: vi.fn(), onOpenSettings: vi.fn(), ...extra };
  render(<IllustrationDialog {...props} />);
  return props;
}

describe("IllustrationDialog", () => {
  beforeEach(() => {
    Object.values(rpc).forEach((f) => f.mockReset());
    rpc.providers.mockResolvedValue(ready);
    rpc.entities.mockResolvedValue(cast);
    rpc.mentions.mockResolvedValue([]);
    rpc.get.mockReturnValue(new Promise(() => {}));
  });

  it("prefills the scene and the scene's characters, and offers only characters", async () => {
    rpc.mentions.mockResolvedValue([{ id: "e1" }, { id: "p1" }]);
    renderDialog({ sourceNodeId: "n1", initialScene: "둘이 우산을 쓴다" });

    expect(screen.getByTestId("illustration-scene")).toHaveValue("둘이 우산을 쓴다");
    await waitFor(() => expect(screen.getByTestId("illustration-cast-e1")).toHaveAttribute("aria-pressed", "true"));
    expect(screen.getByTestId("illustration-cast-e2")).toHaveAttribute("aria-pressed", "false");
    expect(screen.queryByTestId("illustration-cast-p1")).toBeNull();
    expect(rpc.mentions).toHaveBeenCalledWith("n1");
  });

  it("sends exactly the prompt the writer edited", async () => {
    rpc.compose.mockResolvedValue({
      prompt: "Art style:\nwebtoon",
      negative_prompt: "",
      format: "single",
      characters: [{ entity_id: "e2", name: "민호" }],
      reference_image_ids: [],
      missing_sheets: ["민호"],
    });
    rpc.generate.mockResolvedValue({ id: "i1" });
    const props = renderDialog({ sourceNodeId: "n1", initialScene: "비" });

    fireEvent.click(await screen.findByTestId("illustration-cast-e2"));
    expect(screen.getByTestId("illustration-generate")).toBeDisabled();
    fireEvent.click(screen.getByTestId("illustration-compose"));
    await waitFor(() =>
      expect(rpc.compose).toHaveBeenCalledWith({ project_id: "p", entity_ids: ["e2"], scene: "비", format: "single" }),
    );
    const prompt = await screen.findByTestId("illustration-prompt");
    expect(prompt).toHaveValue("Art style:\nwebtoon");
    expect(screen.getByTestId("illustration-missing")).toHaveTextContent("names=민호");

    fireEvent.change(prompt, { target: { value: "Art style:\nwatercolor" } });
    fireEvent.click(screen.getByTestId("illustration-aspect-landscape"));
    fireEvent.click(screen.getByTestId("illustration-generate"));
    await waitFor(() =>
      expect(rpc.generate).toHaveBeenCalledWith({
        project_id: "p",
        source_node_id: "n1",
        prompt: "Art style:\nwatercolor",
        character_ids: ["e2"],
        include_references: true,
        aspect: "landscape",
      }),
    );
    await waitFor(() => expect(props.onSaved).toHaveBeenCalledWith({ id: "i1" }));
    expect(screen.getByTestId("illustration-result")).toHaveTextContent("illustration.saved");
  });

  it("says why it cannot draw and links to the settings", async () => {
    rpc.providers.mockResolvedValue([{ id: "gemini-native", active: true, configured: true, consented: false }]);
    const props = renderDialog();
    expect(await screen.findByTestId("illustration-off")).toHaveTextContent("illustration.off");
    fireEvent.click(screen.getByText("illustration.openSettings"));
    expect(props.onOpenSettings).toHaveBeenCalled();
  });

  it("shows a refusal in words", async () => {
    rpc.compose.mockResolvedValue({ prompt: "p", negative_prompt: "", format: "single", characters: [], reference_image_ids: [], missing_sheets: [] });
    rpc.generate.mockRejectedValue({ code: -32602, message: "refused", data: { reason: "image_refused" } });
    renderDialog({ initialScene: "s" });
    fireEvent.click(await screen.findByTestId("illustration-cast-e1"));
    fireEvent.click(screen.getByTestId("illustration-compose"));
    await screen.findByTestId("illustration-prompt");
    fireEvent.click(screen.getByTestId("illustration-generate"));
    expect(await screen.findByTestId("illustration-error")).toHaveTextContent("errors.imageRefused");
  });
});
