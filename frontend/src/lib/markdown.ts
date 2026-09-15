import DOMPurify from 'dompurify'
import { marked } from 'marked'

/** Render untrusted model text without executable HTML or unsafe links. */
export function renderMarkdown(content: string): string {
  return DOMPurify.sanitize(marked.parse(content, { async: false, breaks: true, gfm: true }), {
    USE_PROFILES: { html: true },
  })
}

/** Report whether Markdown syntax changes how a response is presented. */
export function hasRichMarkdown(content: string): boolean {
  let rich = false
  marked.walkTokens(marked.lexer(content), token => {
    if (!['paragraph', 'text', 'br', 'space'].includes(token.type)) rich = true
  })

  return rich
}
