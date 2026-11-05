// A step fades every card and edge at once while the view pans. On a touch screen that fade turns each card into its own
// composited layer at DPR 3 and iOS Safari's content process runs out of memory (T-278: +340 MB on the first step in
// WebKit with an iPhone 15 profile, the page dies with "A problem repeatedly occurred"). A phone and a tablet change
// steps at once; a desktop keeps the fade.
import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';

const canvas = fs.readFileSync(path.resolve(__dirname, '../src/widgets/canvas/ui/canvas.css'), 'utf8');
const flat = canvas.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\s+/g, ' ');

describe('step fade', () => {
  it('a desktop fades the cards and edges of a step', () => {
    expect(flat).toContain('.react-flow__node, .react-flow__edge { transition: opacity .45s ease; }');
  });

  it('a touch screen changes steps without the fade, whatever its width or orientation', () => {
    expect(flat).toContain('@media (pointer: coarse) { .react-flow__node, .react-flow__edge { transition: none; } }');
  });

  it('a phone share link changes steps without the fade', () => {
    expect(flat).toContain('.phone .react-flow__node, .phone .react-flow__edge { transition: none; }');
  });

  it('the export still shows every step at once', () => {
    expect(flat).toContain('.exporting .react-flow__node.fog, .exporting .react-flow__edge.fog { opacity: 1; transition: none; }');
  });
});
