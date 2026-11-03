import { useEffect, useState } from 'react';

/** Where an element's edges are, in px from the top of the screen. */
export interface Edges { top: number; bottom: number }

const NONE: Edges = { top: 0, bottom: 0 };

/**
 * Where an element's upper and lower edge are, kept up to date as it grows, shrinks or comes and goes (a title that wraps,
 * a font that arrives, a legend that is hidden). Returns a callback ref and the edges; both 0 until it is measured, and
 * again when the element is gone. The fit reads them to keep the board clear of what is laid over the canvas.
 */
export function useEdges(): [(el: HTMLElement | null) => void, Edges] {
  const [el, setEl] = useState<HTMLElement | null>(null);
  const [edges, setEdges] = useState<Edges>(NONE);
  useEffect(() => {
    if (!el) { setEdges(NONE); return; }
    const read = () => {
      const r = el.getBoundingClientRect(), top = Math.floor(r.top), bottom = Math.ceil(r.bottom);
      setEdges(e => (e.top === top && e.bottom === bottom ? e : { top, bottom }));
    };
    read();
    const watch = new ResizeObserver(read);
    watch.observe(el);
    return () => watch.disconnect();
  }, [el]);
  return [setEl, edges];
}
