import { createRef } from "react";
import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { HighlightedTextarea, buildSegments, type HighlightedTextareaHandle } from "./HighlightedTextarea";

describe("buildSegments", () => {
  it("splits at range boundaries", () => {
    expect(buildSegments("abcdef", [{ start: 2, end: 4, severity: "weak" }])).toEqual([
      { text: "ab", severity: undefined },
      { text: "cd", severity: "weak" },
      { text: "ef", severity: undefined },
    ]);
  });

  it("uses the strongest severity where ranges overlap", () => {
    const segs = buildSegments("abcdef", [
      { start: 0, end: 4, severity: "weak" },
      { start: 2, end: 6, severity: "strong" },
    ]);
    expect(segs.map((s) => [s.text, s.severity])).toEqual([
      ["ab", "weak"],
      ["cd", "strong"],
      ["ef", "strong"],
    ]);
  });

  it("clamps ranges past the text and handles empty text", () => {
    expect(buildSegments("ab", [{ start: 1, end: 9, severity: "medium" }]).map((s) => s.text)).toEqual(["a", "b"]);
    expect(buildSegments("", [])).toEqual([]);
  });
});

describe("HighlightedTextarea", () => {
  it("renders underlined spans by severity in the mirror", () => {
    render(
      <HighlightedTextarea
        value="결론적으로 좋다 ㅎㅎ"
        onChange={() => {}}
        aria-label="본문"
        ranges={[
          { start: 0, end: 4, severity: "strong" },
          { start: 8, end: 10, severity: "weak" },
        ]}
      />,
    );
    const mirror = screen.getByTestId("hl-mirror");
    const spans = mirror.querySelectorAll("span[data-severity]");
    expect(spans).toHaveLength(2);
    expect(spans[0]).toHaveTextContent("결론적으");
    expect(spans[0]).toHaveClass("hl-strong");
    expect(spans[1]).toHaveClass("hl-weak");
    expect(mirror).toHaveAttribute("aria-hidden", "true");
    expect(mirror.textContent).toContain("결론적으로 좋다 ㅎㅎ");
  });

  it("reports edits and syncs scroll to the mirror", () => {
    const onChange = vi.fn();
    render(<HighlightedTextarea value="a" onChange={onChange} ranges={[]} aria-label="본문" />);
    const ta = screen.getByLabelText("본문") as HTMLTextAreaElement;
    fireEvent.change(ta, { target: { value: "ab" } });
    expect(onChange).toHaveBeenCalledWith("ab");

    ta.scrollTop = 42;
    fireEvent.scroll(ta);
    expect(screen.getByTestId("hl-mirror").scrollTop).toBe(42);
  });

  it("selects a range and focuses the textarea through the handle", () => {
    const ref = createRef<HighlightedTextareaHandle>();
    render(<HighlightedTextarea ref={ref} value="결론적으로 끝" onChange={() => {}} ranges={[]} aria-label="본문" />);
    const ta = screen.getByLabelText("본문") as HTMLTextAreaElement;
    ref.current?.selectRange(0, 4);
    expect(document.activeElement).toBe(ta);
    expect(ta.selectionStart).toBe(0);
    expect(ta.selectionEnd).toBe(4);
  });
});
