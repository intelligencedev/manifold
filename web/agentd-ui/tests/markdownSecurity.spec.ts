import { createRequire } from "node:module";
import { describe, expect, it } from "vitest";

import { renderMarkdown } from "@/utils/markdown";

const require = createRequire(import.meta.url);

describe("markdown parser security regressions", () => {
  it("resolves the patched markdown-it 14.x release", () => {
    // Both GHSA-gfr2-rv44-r23c and GHSA-r7fv-28h4-cvq7 are fixed in 14.3.3.
    // Check the installed dependency, not just the manifest's allowed range.
    const { version } = require("markdown-it/package.json");
    const [major, minor, patch] = version.split(".").map(Number);
    expect(major).toBe(14);
    expect(minor > 3 || (minor === 3 && patch >= 3)).toBe(true);
  });

  it("preserves reference links and typographic quotes", () => {
    const html = renderMarkdown(
      '[docs]: https://example.com "Documentation"\n\n[docs] and "quoted" text',
    );
    expect(html).toContain('href="https://example.com"');
    expect(html).toContain('title="Documentation"');
    expect(html).toContain("“quoted”");
  });

  it("renders bounded unfinished multiline references as text", () => {
    // Modest fixtures exercise the affected grammar without a DoS benchmark
    // or flaky wall-clock thresholds in the unit suite.
    for (const prefix of [
      "[unfinished",
      '[docs]: https://example.com "unfinished',
    ]) {
      const html = renderMarkdown(`${prefix}\n${"continuation\n".repeat(128)}`);
      expect(html).toContain("unfinished");
      expect(html).toContain("continuation");
      expect(html).not.toContain('title="unfinished');
    }
  });

  it("handles unmatched smartquote runs in both directions", () => {
    for (const [opener, closer] of [
      ['"a ', "b' "],
      ["'a ", 'b" '],
    ]) {
      const html = renderMarkdown(opener.repeat(128) + closer.repeat(128));
      const element = document.createElement("div");
      element.innerHTML = html;
      expect(element.textContent?.match(/a/g)).toHaveLength(128);
      expect(element.textContent?.match(/b/g)).toHaveLength(128);
    }
  });
});
