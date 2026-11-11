import { describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FindingList, sortFindings } from "./FindingList";
import type { Finding } from "../lib/types";

const f = (over: Partial<Finding>): Finding => ({
  rule: "ai_phrase",
  severity: "strong",
  start: 0,
  end: 0,
  message: "m",
  evidence: "",
  ...over,
});

describe("sortFindings", () => {
  it("orders strong, medium, weak and then by start without mutating the input", () => {
    const input = [
      f({ message: "w1", severity: "weak", start: 1, end: 2 }),
      f({ message: "s2", severity: "strong", start: 9, end: 10 }),
      f({ message: "m1", severity: "medium", start: 3, end: 4 }),
      f({ message: "s1", severity: "strong", start: 2, end: 3 }),
    ];
    expect(sortFindings(input).map((x) => x.message)).toEqual(["s1", "s2", "m1", "w1"]);
    expect(input[0].message).toBe("w1");
  });
});

describe("FindingList", () => {
  it("shows message and evidence, in severity order", () => {
    render(
      <FindingList
        onSelect={() => {}}
        findings={[
          f({ message: "약한 지적", severity: "weak", start: 1, end: 2, evidence: "근거 약" }),
          f({ message: "강한 지적", severity: "strong", start: 5, end: 6, evidence: "내 X 글 112편 중 0편" }),
        ]}
      />,
    );
    const items = screen.getAllByRole("listitem");
    expect(within(items[0]).getByText("강한 지적")).toBeInTheDocument();
    expect(within(items[0]).getByText("내 X 글 112편 중 0편")).toBeInTheDocument();
    expect(within(items[1]).getByText("약한 지적")).toBeInTheDocument();
  });

  it("selects a ranged finding on click but leaves whole-post findings inert", async () => {
    const onSelect = vi.fn();
    const ranged = f({ message: "범위 지적", start: 2, end: 5 });
    render(<FindingList onSelect={onSelect} findings={[ranged, f({ message: "전체 지적", severity: "medium" })]} />);

    expect(screen.getAllByRole("button")).toHaveLength(1);
    await userEvent.setup().click(screen.getByRole("button", { name: /범위 지적/ }));
    expect(onSelect).toHaveBeenCalledWith(ranged);
    expect(screen.getByText("전체 지적")).toBeInTheDocument();
  });

  it("says so when there is nothing to show", () => {
    render(<FindingList onSelect={() => {}} findings={[]} />);
    expect(screen.getByText("짚은 곳이 없어요.")).toBeInTheDocument();
  });
});
