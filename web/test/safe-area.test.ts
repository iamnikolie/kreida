// A page that reaches under the notch (viewport-fit=cover) has to keep its controls out of it. Everything at an edge adds the
// `--safe-*` variables of tokens.css; the one place that knows `env()` is tokens.css, so a value set by hand on <html> reaches
// the CSS and the fit code alike (that is how a notch is tried on a browser that reports none).
import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';

const WEB = path.resolve(__dirname, '..');
const read = (...p: string[]) => fs.readFileSync(path.join(WEB, ...p), 'utf8');
const css = (dir: string): string[] => fs.readdirSync(dir, { withFileTypes: true })
  .flatMap(e => (e.isDirectory() ? css(path.join(dir, e.name)) : e.name.endsWith('.css') ? [path.join(dir, e.name)] : []));

describe('safe area', () => {
  it('the page reaches under the notch', () => {
    expect(read('index.html')).toMatch(/<meta name="viewport" content="[^"]*viewport-fit=cover/);
  });

  it('env() is read in tokens.css only: the rest of the CSS uses the variables', () => {
    const raw = css(path.join(WEB, 'src')).filter(f => /env\(\s*safe-area-inset/.test(fs.readFileSync(f, 'utf8'))).map(f => path.relative(WEB, f));
    expect(raw).toEqual(['src/app/styles/tokens.css']);
  });

  it('tokens.css defines all four, with a 0 fallback', () => {
    const tokens = read('src/app/styles/tokens.css');
    for (const side of ['top', 'right', 'bottom', 'left']) expect(tokens).toContain(`--safe-${side}: env(safe-area-inset-${side}, 0px)`);
  });

  it.each([
    ['the phone bar and its sheet', 'src/widgets/phone-bar/ui/phone-bar.css', ['--safe-bottom', '--safe-left', '--safe-right']],
    ['the head, on a phone too', 'src/widgets/head/ui/head.css', ['--safe-left', '--safe-right', '--safe-top']],
    ['the toolbar and the legend', 'src/pages/board/ui/board-page.css', ['--safe-right']],
    ['the zoom and the minimap corners', 'src/widgets/canvas/ui/canvas.css', ['--safe-right', '--safe-left', '--safe-bottom']],
    ['the steps bar', 'src/widgets/steps-bar/ui/steps-bar.css', ['--safe-bottom']],
  ])('%s keep clear of the insets', (_, file, vars) => {
    const text = read(file);
    for (const v of vars) expect(text, v).toContain(`var(${v})`);
  });
});
