import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({ gallery: vi.fn(), cover: vi.fn(), open: vi.fn() }));

vi.mock("../lib/rpc", () => ({
  worksApi: { gallery: rpc.gallery, cover: rpc.cover },
  openExternalUrl: rpc.open,
}));
vi.mock("../lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string, vars?: Record<string, unknown>) => (vars ? `${key} ${JSON.stringify(vars)}` : key),
  }),
}));

import { WorksGallery } from "./WorksGallery";

const work = (id: string, over: Record<string, unknown> = {}) => ({
  id,
  slug: `slug-${id}`,
  title: `작품 ${id}`,
  blurb: "소개",
  genres: ["무협"],
  category: "martial",
  serial_status: "ongoing",
  author: { handle: "writer", pen_name: "필명" },
  episode_count: 3,
  latest_number: 3,
  latest_at: "",
  cover: "",
  url: `https://works.test/writer/slug-${id}`,
  ...over,
});

const page = (items: unknown[], over: Record<string, unknown> = {}) => ({
  q: "",
  genre: "",
  page: 1,
  total_pages: 1,
  total: items.length,
  items,
  site: "https://works.test",
  ...over,
});

const show = () =>
  render(
    <MemoryRouter>
      <WorksGallery />
    </MemoryRouter>,
  );

describe("WorksGallery", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.gallery.mockResolvedValue(page([work("aaaaaaaaaaaaaaa"), work("bbbbbbbbbbbbbbb", { cover: "sha" })]));
    rpc.cover.mockResolvedValue({ data_url: "data:image/png;base64,AA==" });
    rpc.open.mockResolvedValue(undefined);
  });

  it("lists what readers see and opens a work on the site", async () => {
    show();
    expect(await screen.findByTestId("works-card-aaaaaaaaaaaaaaa")).toHaveTextContent("작품 aaaaaaaaaaaaaaa");
    expect(rpc.gallery).toHaveBeenCalledWith({ q: "", genre: "", page: 1 });
    await waitFor(() => expect(rpc.cover).toHaveBeenCalledWith("bbbbbbbbbbbbbbb"));
    expect(rpc.cover).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByTestId("works-card-aaaaaaaaaaaaaaa"));
    expect(rpc.open).toHaveBeenCalledWith("https://works.test/writer/slug-aaaaaaaaaaaaaaa");
  });

  it("filters by genre and searches", async () => {
    show();
    await screen.findByTestId("works-grid");
    await userEvent.click(screen.getByTestId("works-genre-romance"));
    await waitFor(() => expect(rpc.gallery).toHaveBeenLastCalledWith({ q: "", genre: "romance", page: 1 }));
    await userEvent.type(screen.getByTestId("works-search"), "  검   객 ");
    await userEvent.click(screen.getByTestId("works-search-go"));
    await waitFor(() => expect(rpc.gallery).toHaveBeenLastCalledWith({ q: "검 객", genre: "romance", page: 1 }));
  });

  it("adds the next page under the first", async () => {
    rpc.gallery
      .mockResolvedValueOnce(page([work("aaaaaaaaaaaaaaa")], { total_pages: 2, total: 2 }))
      .mockResolvedValueOnce(page([work("ccccccccccccccc")], { page: 2, total_pages: 2, total: 2 }));
    show();
    await userEvent.click(await screen.findByTestId("works-more"));
    expect(await screen.findByTestId("works-card-ccccccccccccccc")).toBeInTheDocument();
    expect(screen.getByTestId("works-card-aaaaaaaaaaaaaaa")).toBeInTheDocument();
    expect(rpc.gallery).toHaveBeenLastCalledWith({ q: "", genre: "", page: 2 });
    expect(screen.queryByTestId("works-more")).toBeNull();
  });

  it("says when the site cannot be reached, and tries again", async () => {
    rpc.gallery.mockRejectedValueOnce(new Error("offline"));
    show();
    expect(await screen.findByTestId("works-error")).toBeInTheDocument();
    await userEvent.click(screen.getByText("works.retry"));
    expect(await screen.findByTestId("works-card-aaaaaaaaaaaaaaa")).toBeInTheDocument();
  });

  it("says so when nothing is public", async () => {
    rpc.gallery.mockResolvedValue(page([]));
    show();
    expect(await screen.findByTestId("works-empty")).toHaveTextContent("works.empty");
  });
});
