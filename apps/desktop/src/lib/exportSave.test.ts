import { describe, expect, it } from "vitest";
import { describeSaveLocation } from "./exportSave";

describe("describeSaveLocation", () => {
  it("reports the folder the file landed in", () => {
    expect(describeSaveLocation("/Users/w/Documents/quiet-city.md").folder)
      .toBe("/Users/w/Documents");
    expect(describeSaveLocation("C:\\Users\\w\\Documents\\quiet-city.md").folder)
      .toBe("C:\\Users\\w\\Documents");
  });

  it("leaves an ordinary folder unflagged", () => {
    expect(describeSaveLocation("/Users/w/Documents/quiet-city.md").synced).toBe(false);
    expect(describeSaveLocation("C:\\Users\\w\\Desktop\\quiet-city.md").synced).toBe(false);
  });

  // iCloud Drive is "Mobile Documents" on disk, which is what the save panel
  // hands back and what the writer will not recognise.
  it("spots iCloud Drive under its on-disk name", () => {
    const got = describeSaveLocation(
      "/Users/w/Library/Mobile Documents/com~apple~CloudDocs/Linetta/quiet-city.md",
    );
    expect(got.synced).toBe(true);
  });

  it("spots the other consumer sync clients", () => {
    for (const p of [
      "/Users/w/Dropbox/novel/quiet-city.md",
      "C:\\Users\\w\\OneDrive - Contoso\\quiet-city.md",
      "C:\\Users\\w\\Google Drive\\My Drive\\quiet-city.md",
      "/Users/w/Library/CloudStorage/GoogleDrive-a@b.c/My Drive/quiet-city.md",
    ]) {
      expect(describeSaveLocation(p).synced, p).toBe(true);
    }
  });

  it("does not flag a folder that merely mentions a service", () => {
    // Matching a bare substring would catch a novel filed under research.
    expect(describeSaveLocation("/Users/w/Documents/dropbox-research/notes.md").synced)
      .toBe(false);
  });
});
