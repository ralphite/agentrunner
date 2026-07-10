import { describe, expect, it } from "vitest";
import { parseFileDiff, splitPath } from "./diffSummary";

describe("parseFileDiff (Codex-style review rows, W5)", () => {
  it("numbers context/add/del rows from hunk headers and hides meta lines", () => {
    const { badges, rows } = parseFileDiff([
      "index 000..111 100644",
      "--- a/x.ts",
      "+++ b/x.ts",
      "@@ -10,3 +10,4 @@ function x()",
      " keep",
      "-old",
      "+new",
      "+more",
      " tail",
    ]);
    expect(badges).toEqual([]);
    expect(rows[0]).toMatchObject({ kind: "hunk", text: "function x()" });
    expect(rows[1]).toMatchObject({ kind: "ctx", oldNo: 10, newNo: 10, text: "keep" });
    expect(rows[2]).toMatchObject({ kind: "del", oldNo: 11, text: "old" });
    expect(rows[3]).toMatchObject({ kind: "add", newNo: 11, text: "new" });
    expect(rows[4]).toMatchObject({ kind: "add", newNo: 12, text: "more" });
    expect(rows[5]).toMatchObject({ kind: "ctx", oldNo: 12, newNo: 13, text: "tail" });
    expect(rows.some((r) => r.text.startsWith("+++") || r.text.startsWith("---") || r.text.startsWith("index"))).toBe(false);
  });

  it("distills git metadata into header badges", () => {
    expect(parseFileDiff(["new file mode 100644", "@@ -0,0 +1,1 @@", "+hi"]).badges).toEqual(["new file"]);
    expect(parseFileDiff(["deleted file mode 100644"]).badges).toEqual(["deleted"]);
    expect(parseFileDiff(["similarity index 90%", "rename from a", "rename to b"]).badges).toEqual(["renamed"]);
    expect(parseFileDiff(["Binary files a/x.png and b/x.png differ"]).badges).toEqual(["binary"]);
    expect(parseFileDiff(["old mode 100644", "new mode 100755"]).badges).toEqual(["mode changed"]);
  });

  it("keeps no-newline markers as unnumbered context", () => {
    const { rows } = parseFileDiff(["@@ -1 +1 @@", "-a", "+b", "\\ No newline at end of file"]);
    expect(rows[3]).toMatchObject({ kind: "ctx", text: "\\ No newline at end of file" });
    expect(rows[3].oldNo).toBeUndefined();
  });

  it("splits paths into dim dir + strong base", () => {
    expect(splitPath("docs/DESIGN.md")).toEqual({ dir: "docs/", base: "DESIGN.md" });
    expect(splitPath("README.md")).toEqual({ dir: "", base: "README.md" });
  });
});
