import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({ items: vi.fn(), setItems: vi.fn() }));

vi.mock("../../lib/rpc", () => ({ vaultApi: { items: rpc.items, setItems: rpc.setItems } }));
vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({ language: "ko", t: (key: string) => key }),
  localeForLanguage: () => "ko-KR",
}));

import { VaultItemList } from "./VaultItemList";

const CATEGORIES = [
  { id: "", bytes: 3 << 20, enabled: true, required: true },
  { id: "images", bytes: 9 << 20, enabled: true, required: false },
  { id: "history", bytes: 2 << 20, enabled: false, required: false },
  { id: "work_agent", bytes: 1 << 10, enabled: true, required: false },
  { id: "global_skills", bytes: 2 << 10, enabled: true, required: false },
  { id: "writer_profile", bytes: 1 << 10, enabled: true, required: false },
];

describe("VaultItemList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.items.mockResolvedValue({ categories: CATEGORIES });
    rpc.setItems.mockImplementation(async (omit: string[]) => ({
      categories: CATEGORIES.map((c) => ({ ...c, enabled: c.required || !omit.includes(c.id) })),
    }));
  });

  it("lists what travels with its size, and the required line cannot be turned off", async () => {
    render(<VaultItemList editable />);
    expect(await screen.findByTestId("vault-category-required")).toBeDisabled();
    expect(screen.getByTestId("vault-category-size-images")).toHaveTextContent("9.0 MiB");
    expect(screen.getByTestId("vault-category-images")).toBeChecked();
    expect(screen.getByTestId("vault-category-history")).not.toBeChecked();
    expect(screen.getByText("settings.vault.whatTravelsNote")).toBeInTheDocument();
  });

  it("turning a line off sends every line that is off", async () => {
    render(<VaultItemList editable />);
    await userEvent.click(await screen.findByTestId("vault-category-images"));
    expect(rpc.setItems).toHaveBeenCalledWith(["images", "history"]);
    await waitFor(() => expect(screen.getByTestId("vault-category-images")).not.toBeChecked());
  });

  it("turning a line on sends the rest", async () => {
    render(<VaultItemList editable />);
    await userEvent.click(await screen.findByTestId("vault-category-history"));
    expect(rpc.setItems).toHaveBeenCalledWith([]);
  });

  it("the selection can be held read only", async () => {
    render(<VaultItemList editable={false} />);
    expect(await screen.findByTestId("vault-category-images")).toBeDisabled();
  });

  it("shows a failed selection and keeps the boxes as they were", async () => {
    rpc.setItems.mockRejectedValue(new Error("boom"));
    render(<VaultItemList editable />);
    await userEvent.click(await screen.findByTestId("vault-category-images"));
    expect(await screen.findByTestId("vault-items-error")).toBeInTheDocument();
    expect(screen.getByTestId("vault-category-images")).toBeChecked();
  });
});
