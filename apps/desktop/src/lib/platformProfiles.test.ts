import { describe, expect, it } from "vitest";
import { normalizePlatformProfile, transformPlatformText } from "./platformProfiles";

describe("platformProfiles", () => {
  it("keeps the plain profile byte-for-byte", () => {
    const text = "첫 줄  \r\n\r\n\r\n... 그대로 ～";

    expect(transformPlatformText(text, "plain")).toBe(text);
  });

  it("normalizes Munpia paste text conservatively", () => {
    const text = "문장...  \n\n\n다음 문단～\n다른 물결〜";

    expect(transformPlatformText(text, "munpia")).toBe("문장…\n\n다음 문단~\n다른 물결~");
  });

  it("normalizes Naver Series paste text without changing tildes", () => {
    const text = "문장...... \r\n\r\n\r\n다음～  ";

    expect(transformPlatformText(text, "series")).toBe("문장…\n\n다음～");
  });

  it("normalizes Joara paste text with the same paragraph rules", () => {
    const text = "대사…... \n\n\n\n다음 줄\t";

    expect(transformPlatformText(text, "joara")).toBe("대사…\n\n다음 줄");
  });

  it("falls back to plain for unknown profile ids", () => {
    expect(normalizePlatformProfile("unknown")).toBe("plain");
  });
});
