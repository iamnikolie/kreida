// A note card (`kind: note`) reads its text as markdown: a framing, a conclusion, "what we decided". Its height comes
// from the blocks it will draw: headings, paragraphs and list items, wrapped at the card's width.
import type { BNode } from '@shared/api';
import { blocks, type Block, type Inline } from '@shared/lib';

const CH = 7.3, LINE = 18, HEAD = 21, GAP = 6, INDENT = 18;

const plain = (c: Inline[]) => c.map(x => x.s).join('');
const wrap = (s: string, w: number) => Math.max(1, Math.ceil((s.length * CH) / w));

/** A note with prose in it is wider than a one-line sticky. */
export const noteWidth = (n: BNode) => n.width ?? (n.text.length > 90 || n.text.includes('\n') ? 300 : 210);

export function noteHeight(n: BNode, width: number): number {
  const inner = width - 36;
  const title = n.title ? Math.ceil((n.title.length * 8.9) / inner) * HEAD : 0;
  const bs = blocks(n.text);
  const tall = (b: Block) => {
    if ('items' in b) return b.items.reduce((k, it) => k + wrap(plain(it), inner - INDENT) * LINE, 0);
    return b.t === 'h' ? wrap(plain(b.c), inner * 0.85) * HEAD : wrap(plain(b.c), inner) * LINE;
  };
  const body = bs.reduce((h, b) => h + GAP + tall(b), 0);
  return Math.max(58, Math.ceil(24 + title + body + (bs.length ? 4 : 0)));
}
