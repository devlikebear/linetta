import { describe, expect, it } from "vitest";
import { displayNodeLabel } from "./i18n";

describe("displayNodeLabel", () => {
  it("translates web-novel part/chapter labels to English", () => {
    expect(displayNodeLabel("en", "1권")).toBe("Arc 1");
    expect(displayNodeLabel("en", "1화")).toBe("Episode 1");
  });

  it("translates web-novel part/chapter labels to Japanese", () => {
    expect(displayNodeLabel("ja", "1권")).toBe("第1巻");
    expect(displayNodeLabel("ja", "1화")).toBe("第1話");
  });

  it("keeps Korean web-novel labels under Korean", () => {
    expect(displayNodeLabel("ko", "2권")).toBe("2권");
    expect(displayNodeLabel("ko", "3화")).toBe("3화");
  });

  it("still handles scene and chapter labels", () => {
    expect(displayNodeLabel("en", "씬 1")).toBe("Scene 1");
    expect(displayNodeLabel("en", "1장")).toBe("Chapter 1");
  });

  it("passes through unknown labels unchanged", () => {
    expect(displayNodeLabel("en", "Prologue")).toBe("Prologue");
  });
});
