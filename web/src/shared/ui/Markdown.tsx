import type { Inline } from '../lib/markdown';
import { blocks } from '../lib/markdown';
import { Glossed } from './Term';

function Spans({ c }: { c: Inline[] }) {
  return <>{c.map((x, i) => {
    switch (x.t) {
      case 'b': return <strong key={i}><Glossed text={x.s} /></strong>;
      case 'i': return <em key={i}><Glossed text={x.s} /></em>;
      case 'code': return <code key={i}>{x.s}</code>;
      case 'a': return <a key={i} href={x.href} target="_blank" rel="noopener noreferrer nofollow" className="nodrag">{x.s}</a>;
      default: return <Glossed key={i} text={x.s} />;
    }
  })}</>;
}

/** A note's text as markdown blocks. Only tokens become elements: nothing in the text is ever read as HTML. */
export function Markdown({ text }: { text: string }) {
  return <div className="md">{blocks(text).map((b, i) => {
    switch (b.t) {
      case 'h': return <h4 key={i}><Spans c={b.c} /></h4>;
      case 'p': return <p key={i}><Spans c={b.c} /></p>;
      case 'ul': return <ul key={i}>{b.items.map((it, j) => <li key={j}><Spans c={it} /></li>)}</ul>;
      case 'ol': return <ol key={i}>{b.items.map((it, j) => <li key={j}><Spans c={it} /></li>)}</ol>;
    }
  })}</div>;
}
