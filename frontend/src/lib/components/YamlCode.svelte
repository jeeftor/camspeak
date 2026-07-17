<script>
  // Lightweight YAML syntax highlighter — same pattern as JsonCode.
  // Handles: comments (# ...), keys (word:), strings, numbers,
  // booleans/null, Jinja templates ({{ ... }}), and list dashes.
  // No external deps; colors are theme-aware via CSS variables.
  let { code, class: klass = '' } = $props()

  function escapeHtml(s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  }

  function tokenize(src) {
    const tokens = []
    // Order: comments, jinja templates, strings, numbers, booleans/null, keys, list dashes
    const re =
      /(^|\n)([ \t]*#[^\n]*)|(\{\{[^}]*\}\})|("[^"]*"|'[^']*')|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|(\btrue\b|\bfalse\b|\bnull\b|\bnone\b)|(^|\n)([ \t]*)([\w-]+)(\s*:)|(^|\n)([ \t]*)(-)/gm
    let last = 0
    let m
    while ((m = re.exec(src)) !== null) {
      if (m.index > last) tokens.push({ type: 'text', value: src.slice(last, m.index) })
      if (m[2]) {
        // comment (includes leading newline)
        tokens.push({ type: 'text', value: m[1] })
        tokens.push({ type: 'comment', value: m[2] })
      } else if (m[3]) {
        tokens.push({ type: 'jinja', value: m[3] })
      } else if (m[4]) {
        tokens.push({ type: 'string', value: m[4] })
      } else if (m[5]) {
        tokens.push({ type: 'number', value: m[5] })
      } else if (m[6]) {
        tokens.push({ type: 'boolean', value: m[6] })
      } else if (m[10]) {
        // key: (includes leading newline + indent)
        tokens.push({ type: 'text', value: m[7] })
        tokens.push({ type: 'text', value: m[8] })
        tokens.push({ type: 'key', value: m[9] })
        tokens.push({ type: 'punctuation', value: m[10] })
      } else if (m[13]) {
        tokens.push({ type: 'text', value: m[11] })
        tokens.push({ type: 'text', value: m[12] })
        tokens.push({ type: 'punctuation', value: m[13] })
      }
      last = re.lastIndex
    }
    if (last < src.length) tokens.push({ type: 'text', value: src.slice(last) })
    return tokens
  }

  let html = $derived(
    tokenize(code)
      .map((t) => {
        const v = escapeHtml(t.value)
        return t.type === 'text' ? v : `<span class="tok-${t.type}">${v}</span>`
      })
      .join(''),
  )
</script>

<pre class="bg-background border rounded-md p-3 overflow-x-auto text-xs text-foreground/80 syntax {klass}">{@html html}</pre>

<style>
  .syntax :global(.tok-key) { color: hsl(var(--syntax-key)); font-weight: 500; }
  .syntax :global(.tok-string) { color: hsl(var(--syntax-string)); }
  .syntax :global(.tok-number) { color: hsl(var(--syntax-number)); }
  .syntax :global(.tok-boolean) { color: hsl(var(--syntax-boolean)); }
  .syntax :global(.tok-comment) { color: hsl(var(--syntax-comment)); font-style: italic; }
  .syntax :global(.tok-punctuation) { color: hsl(var(--syntax-punctuation)); }
  .syntax :global(.tok-jinja) { color: hsl(var(--syntax-number)); font-weight: 500; }
</style>
