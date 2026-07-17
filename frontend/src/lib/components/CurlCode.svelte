<script>
  // Syntax highlighter for curl commands.
  // Colors: the `curl` keyword, flags (-X, -H, -d), HTTP methods,
  // URLs, single-quoted strings, JSON keys/values, and line continuations.
  // No external deps — theme-aware via CSS variables.
  let { code, class: klass = '' } = $props()

  function escapeHtml(s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  }

  function tokenize(src) {
    const tokens = []
    // Order: curl keyword, flags, HTTP methods, URLs, single-quoted strings (incl JSON), line continuations
    const re =
      /(\bcurl\b)|(\s-[XHd]\b)|(\b(GET|POST|PUT|DELETE|PATCH)\b)|(https?:\/\/[^\s'"]+)|('(?:[^'\\]|\\.)*')|(\\\n)/g
    let last = 0
    let m
    while ((m = re.exec(src)) !== null) {
      if (m.index > last) tokens.push({ type: 'text', value: src.slice(last, m.index) })
      if (m[1]) tokens.push({ type: 'keyword', value: m[1] })
      else if (m[2]) tokens.push({ type: 'flag', value: m[2] })
      else if (m[4]) tokens.push({ type: 'method', value: m[4] })
      else if (m[5]) tokens.push({ type: 'url', value: m[5] })
      else if (m[6]) {
        // Single-quoted string — may contain JSON. Sub-tokenize the inner content.
        tokens.push({ type: 'punctuation', value: "'" })
        const inner = m[6].slice(1, -1)
        tokens.push(...tokenizeJson(inner))
        tokens.push({ type: 'punctuation', value: "'" })
      } else if (m[7]) {
        tokens.push({ type: 'continuation', value: m[7] })
      }
      last = re.lastIndex
    }
    if (last < src.length) tokens.push({ type: 'text', value: src.slice(last) })
    return tokens
  }

  // Sub-tokenizer for JSON inside single-quoted strings.
  function tokenizeJson(src) {
    const tokens = []
    const re =
      /("(?:[^"\\]|\\.)*")|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|(\btrue\b|\bfalse\b|\bnull\b)|([{}\[\],:])/g
    let last = 0
    let m
    while ((m = re.exec(src)) !== null) {
      if (m.index > last) tokens.push({ type: 'text', value: src.slice(last, m.index) })
      if (m[1]) {
        // Key if followed by optional whitespace + colon
        const after = src.slice(re.lastIndex)
        tokens.push({ type: /^\s*:/.test(after) ? 'key' : 'string', value: m[1] })
      } else if (m[2]) tokens.push({ type: 'number', value: m[2] })
      else if (m[3]) tokens.push({ type: 'boolean', value: m[3] })
      else if (m[4]) tokens.push({ type: 'punctuation', value: m[4] })
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

<code class="curl-syntax {klass}">{@html html}</code>

<style>
  .curl-syntax :global(.tok-keyword) { color: hsl(var(--syntax-key)); font-weight: 700; }
  .curl-syntax :global(.tok-flag) { color: hsl(var(--syntax-boolean)); font-weight: 600; }
  .curl-syntax :global(.tok-method) { color: hsl(var(--syntax-number)); font-weight: 700; }
  .curl-syntax :global(.tok-url) { color: hsl(var(--syntax-string)); }
  .curl-syntax :global(.tok-key) { color: hsl(var(--syntax-key)); font-weight: 500; }
  .curl-syntax :global(.tok-string) { color: hsl(var(--syntax-string)); }
  .curl-syntax :global(.tok-number) { color: hsl(var(--syntax-number)); }
  .curl-syntax :global(.tok-boolean) { color: hsl(var(--syntax-boolean)); }
  .curl-syntax :global(.tok-punctuation) { color: hsl(var(--syntax-punctuation)); }
  .curl-syntax :global(.tok-continuation) { color: hsl(var(--syntax-comment)); }
</style>
