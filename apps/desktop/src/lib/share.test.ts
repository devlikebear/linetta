import { describe, expect, it } from "vitest";

import { shareAction } from "./share";

const URL3 = "https://works.marvin-42.com/@devlikebear/k-office-god/3";
const TEXT = "3화 · 첫 출근";

function query(url: string) {
  return Object.fromEntries(new URL(url).searchParams);
}

describe("shareAction", () => {
  it("opens X's post page with the text and the link", () => {
    const a = shareAction("x", URL3, TEXT);
    expect(a.kind).toBe("open");
    if (a.kind !== "open") return;
    expect(a.url.startsWith("https://x.com/intent/post?")).toBe(true);
    expect(query(a.url)).toEqual({ text: TEXT, url: URL3 });
  });

  it("opens Threads' post page with the text and the link", () => {
    const a = shareAction("threads", URL3, TEXT);
    if (a.kind !== "open") throw new Error("expected open");
    expect(new URL(a.url).host).toBe("www.threads.com");
    expect(query(a.url)).toEqual({ text: TEXT, url: URL3 });
  });

  it("opens Naver's share page, which offers the blog", () => {
    const a = shareAction("naver", URL3, TEXT);
    if (a.kind !== "open") throw new Error("expected open");
    expect(new URL(a.url).host).toBe("share.naver.com");
    expect(query(a.url)).toEqual({ url: URL3, title: TEXT });
  });

  it("copies the text and the link for KakaoTalk", () => {
    expect(shareAction("kakao", URL3, TEXT)).toEqual({ kind: "copy", text: `${TEXT}\n${URL3}` });
  });

  it("copies for Instagram and opens it to paste", () => {
    expect(shareAction("instagram", URL3, TEXT)).toEqual({
      kind: "copy",
      text: `${TEXT}\n${URL3}`,
      url: "https://www.instagram.com/",
    });
  });
});
