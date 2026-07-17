<script>
  // Lightweight syntax highlighter for JSON-ish reference snippets.
  // Handles: object keys, strings, numbers, booleans/null, punctuation,
  // and `//` line comments (including trailing comments after values).
  // No external deps — keeps the bundle minimal and colors theme-aware.
  let { code, class: klass = '' } = $props()

  function escapeHtml(s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  }

  function tokenize(src) {
    const tokens = []
    // Order matters: comments first so `//` inside strings isn't matched,
    // then strings, numbers, booleans/null, punctuation.
    const re =
      /(\/\/[^\n]*)|("(?:[^"\\]|\\.)*")|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|(\btrue\b|\bfalse\b|\bnull\b)|([{}\[\],:|])/g
    let last = 0
    let m
    while ((m = re.exec(src)) !== null) {
      if (m.index > last) tokens.push({ type: 'text', value: src.slice(last, m.index) })
      if (m[1]) tokens.push({ type: 'comment', value: m[1] })
      else if (m[2]) {
        // A string is a key if a ':' follows (ignoring whitespace).
        const after = src.slice(re.lastIndex)
        tokens.push({ type: /^\s*:/.test(after) ? 'key' : 'string', value: m[2] })
      } else if (m[3]) tokens.push({ type: 'number', value: m[3] })
      else if (m[4]) tokens.push({ type: 'boolean', value: m[4] })
      else if (m[5]) tokens.push({ type: 'punctuation', value: m[5] })
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

<pre class="bg-background border rounded p-3 overflow-x-auto text-foreground/80 syntax {klass}">{@html html}</pre>

<style>
  .syntax :global(.tok-key) { color: hsl(var(--syntax-key)); font-weight: 500; }
  .syntax :global(.tok-string) { color: hsl(var(--syntax-string)); }
  .syntax :global(.tok-number) { color: hsl(var(--syntax-number)); }
  .syntax :global(.tok-boolean) { color: hsl(var(--syntax-boolean)); }
  .syntax :global(.tok-comment) { color: hsl(var(--syntax-comment)); font-style: italic; }
  .syntax :global(.tok-punctuation) { color: hsl(var(--syntax-punctuation)); }
</style>
