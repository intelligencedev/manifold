import MarkdownIt from 'markdown-it'
import hljs from 'highlight.js'

const md = new MarkdownIt({
  html: false,
  linkify: true,
  typographer: true,
  breaks: true,
  highlight(code, language) {
    if (language && hljs.getLanguage(language)) {
      try {
        const { value } = hljs.highlight(code, { language, ignoreIllegals: true })
        return `<pre class="hljs"><code class="hljs language-${language}">${value}</code></pre>`
      } catch (error) {
        console.warn('markdown highlight failed', error)
      }
    }

    const escaped = md.utils.escapeHtml(code)
    return `<pre class="hljs"><code class="hljs">${escaped}</code></pre>`
  }
})

export function renderMarkdown(value: string): string {
  if (!value) {
    return ''
  }
  return md.render(value)
}
