import DOMPurify from 'dompurify'
import { marked } from 'marked'

/** Render untrusted model text without executable HTML or unsafe links. */
export function renderMarkdown(content: string): string {
  return DOMPurify.sanitize(marked.parse(content, { async: false, breaks: true, gfm: true }), {
    USE_PROFILES: { html: true },
  })
}
