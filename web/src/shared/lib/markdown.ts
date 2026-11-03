// The markdown of a note card: headings, lists, paragraphs, **bold**, *italic*, `code` and links. Board text reaches
// strangers through share links, so this never yields HTML: it yields tokens React renders as text, and a link survives
// only with an http(s) or mailto target.

export type Inline = { t: 'text' | 'b' | 'i' | 'code'; s: string } | { t: 'a'; s: string; href: string };
export type Block = { t: 'p' | 'h'; c: Inline[] } | { t: 'ul' | 'ol'; items: Inline[][] };

const SAFE = new Set(['http:', 'https:', 'mailto:']);

/** The link target if it is an absolute http(s) or mailto URL, else null. */
export function safeHref(href: string): string | null {
  try {
    const u = new URL(href.trim());
    return SAFE.has(u.protocol) && href.trim().toLowerCase().startsWith(u.protocol) ? u.href : null;
  } catch { return null; }
}

const MARK = /\*\*([^*]+?)\*\*|\*([^*\s][^*]*?)\*|`([^`]+)`|\[([^\]]+)\]\(([^)\s]+)\)/g;

/** One line's inline marks; an unclosed mark stays text. */
export function inline(s: string): Inline[] {
  const out: Inline[] = [];
  const text = (x: string) => {
    if (!x) return;
    const last = out[out.length - 1];
    if (last?.t === 'text') last.s += x; else out.push({ t: 'text', s: x });
  };
  let at = 0;
  for (const m of s.matchAll(MARK)) {
    text(s.slice(at, m.index));
    at = m.index + m[0].length;
    if (m[1] != null) out.push({ t: 'b', s: m[1] });
    else if (m[2] != null) out.push({ t: 'i', s: m[2] });
    else if (m[3] != null) out.push({ t: 'code', s: m[3] });
    else {
      const href = safeHref(m[5]);
      if (href) out.push({ t: 'a', s: m[4], href }); else text(m[4]);
    }
  }
  text(s.slice(at));
  return out;
}

/** The blocks of a note's text: `#`/`##` headings, `-`/`*`/`1.` lists, paragraphs (lines joined, a blank line ends one). */
export function blocks(src: string): Block[] {
  const out: Block[] = [];
  let para: string[] = [];
  const flush = () => { if (para.length) out.push({ t: 'p', c: inline(para.join(' ')) }); para = []; };
  for (const raw of src.split('\n')) {
    const line = raw.trim();
    const h = line.match(/^#{1,3}\s+(.*)$/), ul = line.match(/^[-*]\s+(.*)$/), ol = line.match(/^\d+[.)]\s+(.*)$/);
    if (!line) { flush(); continue; }
    if (h) { flush(); out.push({ t: 'h', c: inline(h[1]) }); continue; }
    const item = ul ?? ol;
    if (item) {
      flush();
      const t = ul ? 'ul' : 'ol', last = out[out.length - 1];
      if (last?.t === t) last.items.push(inline(item[1])); else out.push({ t, items: [inline(item[1])] });
      continue;
    }
    para.push(line);
  }
  flush();
  return out;
}
