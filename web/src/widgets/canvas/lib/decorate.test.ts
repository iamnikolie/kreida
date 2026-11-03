import { describe, expect, it } from 'vitest';
import type { Edge, Node } from '@xyflow/react';
import { decorateEdges, decorateNodes } from './decorate';

const node = (id: string): Node => ({ id, position: { x: 0, y: 0 }, data: { title: id } });
const edge = (id: string, source: string, target: string, className?: string): Edge => ({ id, source, target, className });
const none = { fresh: new Set<string>(), fog: null, live: {}, sketch: false };

describe('decorateNodes', () => {
  it('leaves plain nodes untouched (same objects, so React skips them)', () => {
    const ns = [node('a'), node('b')];
    const out = decorateNodes(ns, none);
    expect(out[0]).toBe(ns[0]);
    expect(out[1]).toBe(ns[1]);
  });

  it('adds fresh and fog classes', () => {
    const out = decorateNodes([node('a'), node('b'), node('c')], {
      ...none, fresh: new Set(['a', 'b']), fog: id => id === 'b' || id === 'c',
    });
    expect(out.map(n => n.className)).toEqual(['fresh', 'fresh fog', 'fog']);
  });

  it('hands a live value and the sketch flag to the node data', () => {
    const live = { a: { value: 'up' } };
    const [a, b] = decorateNodes([node('a'), node('b')], { ...none, live });
    expect(a.data).toEqual({ title: 'a', liveVal: { value: 'up' }, sketch: false });
    expect(b.data).toEqual({ title: 'b' });
    const [s] = decorateNodes([node('b')], { ...none, sketch: true });
    expect(s.data).toEqual({ title: 'b', liveVal: undefined, sketch: true });
  });
});

describe('decorateEdges', () => {
  it('fades an edge when either end is hidden, keeping its own classes', () => {
    const out = decorateEdges([edge('e1', 'a', 'b', 'dashed'), edge('e2', 'b', 'c'), edge('e3', 'a', 'c')], {
      fog: id => id === 'b', sketch: false,
    });
    expect(out.map(e => e.className)).toEqual(['dashed fog', ' fog', undefined]);
  });

  it('marks sketch edges; no fog, no sketch: untouched', () => {
    const es = [edge('e', 'a', 'b')];
    expect(decorateEdges(es, { fog: null, sketch: false })[0]).toBe(es[0]);
    expect(decorateEdges(es, { fog: null, sketch: true })[0].data).toEqual({ sketch: true });
  });
});
