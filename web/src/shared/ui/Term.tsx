import { useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useGlossary, type Term as T } from '../lib/glossary';
import { placePopover } from './place';
import './term.css';

/**
 * A glossary term in a card: dotted underline, and its definition in a popover on hover, focus or tap. The popover
 * lives on <body>, so the canvas zoom never shrinks it and no card clips it, and it opens clear of the card the term is in.
 */
export function Term({ text, term }: { text: string; term: T }) {
  const ref = useRef<HTMLSpanElement>(null);
  const [at, setAt] = useState<{ x: number; y: number; up: boolean } | null>(null);
  const show = () => {
    const el = ref.current;
    if (!el) return;
    const box = (el.closest('.react-flow__node, .questions, .matrix .mq') ?? el).getBoundingClientRect();
    setAt(placePopover(el.getBoundingClientRect(), box, { width: innerWidth, height: innerHeight }));
  };
  const hide = () => setAt(null);
  return (
    <>
      <span ref={ref} className="term" tabIndex={0} onMouseEnter={show} onMouseLeave={hide} onFocus={show} onBlur={hide}
        onClick={e => { e.stopPropagation(); if (at) hide(); else show(); }}>{text}</span>
      {at && createPortal(
        <div className={`term-pop${at.up ? ' up' : ''}`} role="tooltip" style={{ left: at.x, top: at.y }}>
          <b>{term.term}</b> {term.text}
        </div>, document.body)}
    </>
  );
}

/** Text with the board's glossary terms marked. */
export function Glossed({ text }: { text: string }) {
  const split = useGlossary();
  return <>{split(text).map((s, i) => (typeof s === 'string' ? s : <Term key={i} text={s.text} term={s.term} />))}</>;
}
