import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Markdown } from "./Markdown";

function strongTexts(container: HTMLElement) {
  return Array.from(container.querySelectorAll("strong")).map((el) => el.textContent);
}

describe("Markdown", () => {
  // CommonMark does not treat a closing ** as right-flanking when it follows
  // punctuation and precedes a letter, which is every particle attached to a
  // bold phrase ending in ) or 」.
  it.each([
    ["Korean after ASCII punctuation", "**백가 선대(설화의 할아버지)**이고", "백가 선대(설화의 할아버지)", "이고"],
    ["Korean after a quote", '**"설화"**는 떠났다', '"설화"', "는 떠났다"],
    ["Japanese after a full-width paren", "**百家の先代(祖父)**です", "百家の先代(祖父)", "です"],
    ["Japanese after a corner bracket", "**「説話」**は去った", "「説話」", "は去った"],
    ["Japanese after a full stop", "**これは重要。**次に進む", "これは重要。", "次に進む"],
  ])("renders bold closed before CJK text: %s", (_name, source, bold, rest) => {
    const { container } = render(<Markdown text={source} />);

    expect(strongTexts(container)).toEqual([bold]);
    expect(container.textContent).toBe(`${bold}${rest}`);
  });

  it("renders bold opened after CJK text", () => {
    const { container } = render(<Markdown text="그는**(설화의 할아버지)** 이고 彼は**「説話」** です" />);

    expect(strongTexts(container)).toEqual(["(설화의 할아버지)", "「説話」"]);
  });

  it("renders italic closed before CJK text", () => {
    const { container } = render(<Markdown text="*(祖父)*です" />);

    expect(container.querySelector("em")?.textContent).toBe("(祖父)");
  });

  it("still renders ordinary bold", () => {
    const { container } = render(<Markdown text="An **ordinary** word and **word**." />);

    expect(strongTexts(container)).toEqual(["ordinary", "word"]);
  });

  it("keeps the Latin flanking rules", () => {
    const { container } = render(<Markdown text="2 * 3 * 4 and **(word)**next" />);

    expect(strongTexts(container)).toEqual([]);
    expect(container.textContent).toBe("2 * 3 * 4 and **(word)**next");
  });

  it("leaves code spans and fenced code untouched", () => {
    const source = "`**(할아버지)**이고`\n\n```\n**「説話」**は\n```";
    const { container } = render(<Markdown text={source} />);

    expect(strongTexts(container)).toEqual([]);
    expect(Array.from(container.querySelectorAll("code")).map((el) => el.textContent?.trim())).toEqual([
      "**(할아버지)**이고",
      "**「説話」**は",
    ]);
  });
});
