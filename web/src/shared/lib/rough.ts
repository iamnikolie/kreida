// Hand-drawn style (rough.js): outlines for cards and groups, wobbly edges. Seeded by id, so a shape
// keeps the same strokes across re-renders instead of jittering.
import rough from 'roughjs';

const gen = rough.generator();

export const seedOf = (s: string) => { let h = 7; for (const c of s) h = (h * 31 + c.charCodeAt(0)) | 0; return (Math.abs(h) % 2 ** 31) + 1; };

const paths = (d: string, seed: number, dashed: boolean) =>
  gen.toPaths(gen.path(d, { seed, roughness: 1.1, bowing: 1.2, strokeLineDash: dashed ? [7, 6] : undefined })).map(p => p.d);

export function roundRect(w: number, h: number, r: number) {
  r = Math.min(r, w / 2, h / 2);
  return `M${r},0 H${w - r} Q${w},0 ${w},${r} V${h - r} Q${w},${h} ${w - r},${h} H${r} Q0,${h} 0,${h - r} V${r} Q0,0 ${r},0 Z`;
}

/** Wobbly strokes for a rounded box of the given size. */
export const roughBox = (w: number, h: number, r: number, id: string, dashed: boolean) => paths(roundRect(w, h, r), seedOf(id), dashed);

/** Wobbly strokes for an arbitrary SVG path (an edge). */
export const roughEdge = (d: string, id: string, dashed: boolean) => paths(d, seedOf(id), dashed);
