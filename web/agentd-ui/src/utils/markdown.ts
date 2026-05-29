import MarkdownIt from "markdown-it";
import hljs from "highlight.js/lib/core";
import bash from "highlight.js/lib/languages/bash";
import css from "highlight.js/lib/languages/css";
import diff from "highlight.js/lib/languages/diff";
import dockerfile from "highlight.js/lib/languages/dockerfile";
import go from "highlight.js/lib/languages/go";
import javascript from "highlight.js/lib/languages/javascript";
import json from "highlight.js/lib/languages/json";
import markdown from "highlight.js/lib/languages/markdown";
import plaintext from "highlight.js/lib/languages/plaintext";
import python from "highlight.js/lib/languages/python";
import shell from "highlight.js/lib/languages/shell";
import sql from "highlight.js/lib/languages/sql";
import typescript from "highlight.js/lib/languages/typescript";
import xml from "highlight.js/lib/languages/xml";
import yaml from "highlight.js/lib/languages/yaml";
import DOMPurify from "dompurify";

const SANITIZE_OPTIONS = {
  ALLOW_DATA_ATTR: true,
};

hljs.registerLanguage("bash", bash);
hljs.registerLanguage("css", css);
hljs.registerLanguage("diff", diff);
hljs.registerLanguage("dockerfile", dockerfile);
hljs.registerLanguage("go", go);
hljs.registerLanguage("javascript", javascript);
hljs.registerLanguage("json", json);
hljs.registerLanguage("markdown", markdown);
hljs.registerLanguage("plaintext", plaintext);
hljs.registerLanguage("python", python);
hljs.registerLanguage("shell", shell);
hljs.registerLanguage("sql", sql);
hljs.registerLanguage("typescript", typescript);
hljs.registerLanguage("xml", xml);
hljs.registerLanguage("yaml", yaml);

hljs.registerAliases(["sh", "zsh"], { languageName: "bash" });
hljs.registerAliases(["js", "jsx"], { languageName: "javascript" });
hljs.registerAliases(["md"], { languageName: "markdown" });
hljs.registerAliases(["text", "txt"], { languageName: "plaintext" });
hljs.registerAliases(["py"], { languageName: "python" });
hljs.registerAliases(["ts", "tsx"], { languageName: "typescript" });
hljs.registerAliases(["html", "svg", "vue"], { languageName: "xml" });
hljs.registerAliases(["yml"], { languageName: "yaml" });

export interface RenderMarkdownOptions {
  mermaid?: boolean;
}

function wrapHighlighted(value: string, language?: string) {
  const languageClass = language ? ` language-${language}` : "";
  const langLabel = language
    ? `<span class="md-lang">${md.utils.escapeHtml(language)}</span>`
    : "";
  return `
    <div class="md-codeblock">
      <div class="md-codeblock-toolbar">
        ${langLabel}
        <button type="button" class="md-copy-btn" data-copy>Copy</button>
      </div>
      <pre class="hljs${languageClass}"><code class="hljs${languageClass}">${value}</code></pre>
    </div>
  `;
}

function renderMermaidPlaceholder(value: string) {
  return `
    <div class="md-mermaid" data-mermaid-diagram="true" role="img" aria-label="Mermaid diagram">
      <pre class="md-mermaid-source"><code>${md.utils.escapeHtml(value)}</code></pre>
    </div>
  `;
}

const md = new MarkdownIt({
  html: true,
  linkify: true,
  typographer: true,
  breaks: true,
  highlight(code: string, language?: string) {
    const lang = language?.trim().split(/\s+/)[0];
    if (lang && hljs.getLanguage(lang)) {
      try {
        const { value } = hljs.highlight(code, {
          language: lang,
          ignoreIllegals: true,
        });
        return wrapHighlighted(value, lang);
      } catch (error) {
        console.warn("markdown highlight failed", error);
      }
    }

    const escaped = md.utils.escapeHtml(code);
    return wrapHighlighted(escaped);
  },
});

const defaultFence =
  md.renderer.rules.fence ??
  ((tokens: any[], idx: number, options: any, env: any, self: any) =>
    self.renderToken(tokens, idx, options));

function normalizeIndentedHtmlBlocks(value: string): string {
  const lines = value.split("\n");
  let inFence = false;

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    if (/^ {0,3}```/.test(line)) {
      inFence = !inFence;
      continue;
    }
    if (inFence) continue;
    if (!/^\s{4,}(?:<|<!--)/.test(line)) continue;

    lines[index] = line.trimStart();
  }

  return lines.join("\n");
}

md.renderer.rules.fence = (
  tokens: any[],
  idx: number,
  options: any,
  env: any,
  self: any,
) => {
  const token = tokens[idx];
  const info = token.info ? token.info.trim().split(/\s+/)[0] : "";
  if (env?.mermaid === true && (info === "mermaid" || info === "mmd")) {
    return `${renderMermaidPlaceholder(token.content)}\n`;
  }
  const highlighted = options.highlight
    ? options.highlight(token.content, info)
    : "";
  if (highlighted && highlighted !== token.content) {
    return `${highlighted}\n`;
  }
  return defaultFence(tokens, idx, options, env, self);
};

export function renderMarkdown(
  value: string,
  options: RenderMarkdownOptions = {},
): string {
  if (!value) {
    return "";
  }
  // During streaming, the content may include an unclosed fenced code block (```)
  // which prevents proper formatting until the final chunk arrives. To improve
  // UX, temporarily close an unbalanced fence before rendering.
  let text = normalizeIndentedHtmlBlocks(value);
  try {
    const re = /(^|\n)```/g;
    let count = 0;
    while (re.exec(text)) count += 1;
    if (count % 2 === 1) {
      text += "\n```";
    }
  } catch {
    // no-op: fallback to raw value
  }
  return DOMPurify.sanitize(
    md.render(text, { mermaid: options.mermaid === true }),
    SANITIZE_OPTIONS,
  );
}
