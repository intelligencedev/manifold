import { describe, expect, it } from "vitest";

import { renderMarkdown } from "@/utils/markdown";

describe("renderMarkdown", () => {
  it("renders safe embedded HTML inside markdown", () => {
    const html = renderMarkdown(
      "Hello <mark>world</mark>\n\n<details><summary>More</summary><p>Body</p></details>",
    );

    expect(html).toContain("<mark>world</mark>");
    expect(html).toContain("<details>");
    expect(html).toContain("<summary>More</summary>");
  });

  it("strips unsafe tags and inline event handlers", () => {
    const html = renderMarkdown(
      '<img src="x" onerror="alert(1)"><script>alert(1)</script><div onclick="evil()">ok</div>',
    );

    expect(html).not.toContain("<script");
    expect(html).not.toContain("onerror=");
    expect(html).not.toContain("onclick=");
    expect(html).toContain("<img src=\"x\">");
    expect(html).toContain("<div>ok</div>");
  });

  it("blocks javascript links while preserving safe links", () => {
    const html = renderMarkdown(
      '[safe](https://example.com) <a href="javascript:alert(1)">bad</a>',
    );

    expect(html).toContain('href="https://example.com"');
    expect(html).not.toContain("javascript:alert(1)");
  });

  it("preserves code block wrappers and escapes embedded HTML inside fences", () => {
    const html = renderMarkdown("```html\n<section onclick=\"evil()\">Hi</section>\n```");

    expect(html).toContain("md-codeblock");
    expect(html).toContain("data-copy");
    expect(html).toContain("md-copy-btn");
    expect(html).toContain("&lt;");
    expect(html).toContain("section");
    expect(html).toContain("onclick");
    expect(html).toContain("evil()");
    expect(html).not.toContain("<section onclick=");
  });

  it("temporarily closes an unbalanced code fence during streaming", () => {
    const html = renderMarkdown("```html\n<div>partial");

    expect(html).toContain("md-codeblock");
    expect(html).toContain("&lt;");
    expect(html).toContain("div");
    expect(html).toContain("partial");
  });

  it("normalizes indented HTML lines so nested markup still renders", () => {
    const html = renderMarkdown(`Step 3\n\n<div style="display:flex"><div>a</div></div>\n\n    <!-- Token row -->\n    <div style="display:flex">\n      <div>nested</div>\n    </div>`);

    expect(html).not.toContain("&lt;!-- Token row --&gt;");
    expect(html).not.toContain("<pre><code>");
    expect(html).toContain("nested");
    expect(html).toContain("display:flex");
  });
});