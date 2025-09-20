import MarkdownIt from 'markdown-it'
import hljs from 'highlight.js'

function wrapHighlighted(value: string, language?: string) {
  const languageClass = language ? ` language-${language}` : ''
  const langLabel = language ? `<span class="md-lang">${md.utils.escapeHtml(language)}</span>` : ''
  return `
    <div class="md-codeblock">
      <div class="md-codeblock-toolbar">
        ${langLabel}
        <button type="button" class="md-copy-btn" data-copy>Copy</button>
      </div>
      <pre class="hljs${languageClass}"><code class="hljs${languageClass}">${value}</code></pre>
    </div>
  `
}

const md = new MarkdownIt({
  html: false,
  linkify: true,
  typographer: true,
  breaks: true,
  highlight(code: string, language?: string) {
    const lang = language?.trim().split(/\s+/)[0]
    if (lang && hljs.getLanguage(lang)) {
      try {
        const { value } = hljs.highlight(code, { language: lang, ignoreIllegals: true })
        return wrapHighlighted(value, lang)
      } catch (error) {
        console.warn('markdown highlight failed', error)
      }
    }

    const escaped = md.utils.escapeHtml(code)
    return wrapHighlighted(escaped)
  }
})

const defaultFence = md.renderer.rules.fence ?? ((tokens: any[], idx: number, options: any, env: any, self: any) => self.renderToken(tokens, idx, options))

md.renderer.rules.fence = (tokens: any[], idx: number, options: any, env: any, self: any) => {
  const token = tokens[idx]
  const info = token.info ? token.info.trim().split(/\s+/)[0] : ''
  const highlighted = options.highlight ? options.highlight(token.content, info) : ''
  if (highlighted && highlighted !== token.content) {
    return `${highlighted}\n`
  }
  return defaultFence(tokens, idx, options, env, self)
}

export function renderMarkdown(value: string): string {
  if (!value) {
    return ''
  }
  return md.render(value)
}
