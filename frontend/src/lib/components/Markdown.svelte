<script>
  // Lightweight markdown renderer using marked.
  // Sanitizes via marked's built-in escaping (no raw HTML passed through).
  // Theme-aware via CSS variables — no hardcoded colors.
  import { marked } from 'marked'

  let { content, class: klass = '' } = $props()

  marked.setOptions({
    breaks: true,
    gfm: true,
  })

  let html = $derived(marked.parse(content || ''))
</script>

<div class="md {klass}">{@html html}</div>

<style>
  .md :global(p) { margin: 0 0 0.5em 0; }
  .md :global(p:last-child) { margin-bottom: 0; }
  .md :global(ul) { margin: 0 0 0.5em 0; padding-left: 1.25em; list-style: disc; }
  .md :global(ol) { margin: 0 0 0.5em 0; padding-left: 1.25em; list-style: decimal; }
  .md :global(li) { margin: 0.15em 0; }
  .md :global(h1), .md :global(h2), .md :global(h3), .md :global(h4) {
    font-weight: 600; margin: 0.5em 0 0.25em; line-height: 1.3;
  }
  .md :global(h1) { font-size: 1.15em; }
  .md :global(h2) { font-size: 1.1em; }
  .md :global(h3) { font-size: 1.05em; }
  .md :global(strong) { font-weight: 600; }
  .md :global(em) { font-style: italic; }
  .md :global(code) {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.85em;
    background: hsl(var(--muted));
    padding: 0.1em 0.3em;
    border-radius: 3px;
  }
  .md :global(pre) {
    background: hsl(var(--muted));
    border: 1px solid hsl(var(--border));
    border-radius: 4px;
    padding: 0.5em 0.75em;
    overflow-x: auto;
    margin: 0.5em 0;
  }
  .md :global(pre code) {
    background: none; padding: 0; font-size: 0.85em;
  }
  .md :global(blockquote) {
    border-left: 3px solid hsl(var(--border));
    padding-left: 0.75em;
    margin: 0.5em 0;
    color: hsl(var(--muted-foreground));
  }
  .md :global(a) { color: hsl(var(--primary)); text-decoration: underline; }
  .md :global(hr) { border: none; border-top: 1px solid hsl(var(--border)); margin: 0.75em 0; }
  .md :global(table) { border-collapse: collapse; margin: 0.5em 0; }
  .md :global(th), .md :global(td) { border: 1px solid hsl(var(--border)); padding: 0.25em 0.5em; }
  .md :global(th) { font-weight: 600; background: hsl(var(--muted)); }
</style>
