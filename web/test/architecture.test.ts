// Feature-Sliced Design, enforced: layers import only downward, slices only through their public API
// (index.ts), and same-layer slices only know each other through an explicit @x door.
import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';

const SRC = path.resolve(__dirname, '../src');
const LAYERS = ['app', 'pages', 'widgets', 'features', 'entities', 'shared'] as const; // top to bottom
type Layer = (typeof LAYERS)[number];
const SLICELESS: Layer[] = ['app', 'shared']; // organised by segment (ui, lib, api ...), not by slice

const files = (dir: string): string[] => fs.readdirSync(dir, { withFileTypes: true })
  .flatMap(e => (e.isDirectory() ? files(path.join(dir, e.name)) : /\.(ts|tsx)$/.test(e.name) ? [path.join(dir, e.name)] : []));

// `import x from 'a'`, `export { y } from 'b'` (possibly over several lines) and `import 'c.css'`.
const importsOf = (file: string) => {
  const text = fs.readFileSync(file, 'utf8');
  return [
    ...text.matchAll(/^\s*(?:import|export)\s[^;'"]*?\bfrom\s*['"]([^'"]+)['"]/gm),
    ...text.matchAll(/^\s*import\s*['"]([^'"]+)['"]/gm),
  ].map(m => m[1]);
};

// Where a file or an import target lives: layer, slice (null for sliceless layers) and what is left of the path.
function place(rel: string) {
  const [layer, a, ...rest] = rel.split('/') as [Layer, string, ...string[]];
  return SLICELESS.includes(layer) ? { layer, slice: null, rest: [a, ...rest] } : { layer, slice: a, rest };
}

const sources = files(SRC).map(f => path.relative(SRC, f)).filter(f => LAYERS.includes(f.split('/')[0] as Layer));

describe('FSD layout', () => {
  it('every source file sits in a known layer', () => {
    const stray = files(SRC).map(f => path.relative(SRC, f)).filter(f => !LAYERS.includes(f.split('/')[0] as Layer) && f !== 'vite-env.d.ts');
    expect(stray).toEqual([]);
  });

  it('every slice has a public API (index.ts)', () => {
    const missing = new Set<string>();
    for (const layer of LAYERS.filter(l => !SLICELESS.includes(l)))
      for (const s of fs.readdirSync(path.join(SRC, layer)))
        if (!fs.existsSync(path.join(SRC, layer, s, 'index.ts'))) missing.add(`${layer}/${s}`);
    expect([...missing]).toEqual([]);
  });

  it('imports go down the layers, through public APIs only', () => {
    const bad: string[] = [];
    for (const file of sources) {
      const from = place(file);
      for (const spec of importsOf(path.join(SRC, file))) {
        const alias = spec.match(/^@(app|pages|widgets|features|entities|shared)\/(.+)$/);
        if (alias) {
          const to = place(`${alias[1]}/${alias[2]}`);
          const down = LAYERS.indexOf(to.layer) > LAYERS.indexOf(from.layer);
          const sameLayer = to.layer === from.layer;
          const door = to.rest[0] === '@x'; // slice/@x/<consumer>.ts: an explicit cross-slice door
          if (sameLayer && !SLICELESS.includes(from.layer) && to.slice !== from.slice && !door) bad.push(`${file} -> ${spec} (same-layer slice)`);
          else if (!down && !sameLayer) bad.push(`${file} -> ${spec} (upward)`);
          else if (!sameLayer && !SLICELESS.includes(to.layer) && !door && to.rest.length > 0) bad.push(`${file} -> ${spec} (past the public API)`);
          else if (!sameLayer && to.layer === 'shared' && to.rest.length > 1) bad.push(`${file} -> ${spec} (past the segment index)`);
          continue;
        }
        if (spec.startsWith('.')) {
          const target = place(path.relative(SRC, path.resolve(path.dirname(path.join(SRC, file)), spec)));
          if (target.layer !== from.layer || target.slice !== from.slice) bad.push(`${file} -> ${spec} (leaves its slice)`);
        }
      }
    }
    expect(bad).toEqual([]);
  });
});
