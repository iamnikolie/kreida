// No user-facing text written straight into a component: every string a client or the editor reads goes through the
// catalogs (src/shared/lib/catalog), so a page in uk never shows a Russian word and a new string cannot skip the
// translation. A small scanner reads the sources (no parser ships with TypeScript 7's native compiler) and reports:
//  - text between JSX tags, and string values of title / aria-label / placeholder / alt / label that hold a letter;
//  - any Cyrillic inside a string literal (a Russian word cannot be anything but UI text), comments aside.
// What really is not UI text (a command, the brand) is listed in ALLOWED with the reason; an entry nothing needs fails.
import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';

const SRC = path.resolve(__dirname, '../src');

const ALLOWED: { file: string; text: string; why: string }[] = [
  { file: 'widgets/head/ui/Head.tsx', text: 'kreida', why: 'the product name next to the editor\'s board switch' },
  { file: 'widgets/notice/ui/Notice.tsx', text: 'kreida skill', why: 'a command the user types, shown as code' },
  { file: 'shared/lib/brands.ts', text: '*', why: 'Russian spellings of tool names that a card title is matched against, not text shown' },
];

const LABELS = new Set(['title', 'aria-label', 'aria-description', 'aria-roledescription', 'aria-valuetext', 'placeholder', 'alt', 'label']);

interface Finding { line: number; kind: 'jsx text' | 'attribute' | 'cyrillic'; text: string }

const hasLetter = (s: string) => /\p{L}/u.test(s.replace(/&\w+;|&#\d+;/g, ''));
const hasCyrillic = (s: string) => /\p{Script=Cyrillic}/u.test(s);

/** Reads TS/TSX source and returns the user-facing text written into it. Strings, templates, regexes, comments and JSX are told apart; nothing else is parsed. */
export function scan(s: string): Finding[] {
  const out: Finding[] = [];
  const line = (at: number) => s.slice(0, at).split('\n').length;
  const ident = /[\w$]/;

  const string = (i: number): number => { // at a quote: returns the index after the closing one
    const q = s[i];
    let j = i + 1;
    while (j < s.length && s[j] !== q) j += s[j] === '\\' ? 2 : 1;
    return j + 1;
  };
  const template = (i: number): number => { // at a backtick; `${ }` nests, and only the text around it is checked
    let j = i + 1, quasi = '', at = i;
    const flush = () => { if (hasCyrillic(quasi)) out.push({ line: line(at), kind: 'cyrillic', text: quasi.trim() }); quasi = ''; };
    while (j < s.length && s[j] !== '`') {
      if (s[j] === '\\') { quasi += s[j + 1]; j += 2; }
      else if (s[j] === '$' && s[j + 1] === '{') { flush(); j = code(j + 2, true); at = j; }
      else quasi += s[j++];
    }
    flush();
    return j + 1;
  };
  const regex = (i: number): number => {
    let j = i + 1, cls = false;
    while (j < s.length && (s[j] !== '/' || cls) && s[j] !== '\n') { if (s[j] === '\\') j++; else if (s[j] === '[') cls = true; else if (s[j] === ']') cls = false; j++; }
    j++;
    while (/[a-z]/.test(s[j] ?? '')) j++;
    return j;
  };
  const literal = (i: number): number => {
    if (s[i] === '`') return template(i);
    const end = string(i);
    if (hasCyrillic(s.slice(i, end))) out.push({ line: line(i), kind: 'cyrillic', text: s.slice(i + 1, end - 1) });
    return end;
  };

  /** Code up to the `}` that closes the block we are in (or to the end of the file). */
  function code(i: number, inBraces: boolean): number {
    let prev = '';
    while (i < s.length) {
      const c = s[i];
      if (c === '/' && s[i + 1] === '/') { while (i < s.length && s[i] !== '\n') i++; continue; }
      if (c === '/' && s[i + 1] === '*') { i = s.indexOf('*/', i + 2) + 2; continue; }
      if (c === '"' || c === "'" || c === '`') { i = literal(i); prev = 'a'; continue; }
      if (c === '/' && (prev === '' || '(,=:[!&|?{};'.includes(prev))) { i = regex(i); prev = 'a'; continue; }
      if (c === '{') { i = code(i + 1, true); prev = '}'; continue; }
      if (c === '}') { if (inBraces) return i + 1; i++; continue; }
      // `<` opens JSX when a tag name (or a fragment) follows and what comes before is no value: `a<b>(c)` and `x < y` are not
      if (c === '<' && /[A-Za-z>]/.test(s[i + 1] ?? '') && !ident.test(prev) && prev !== ')' && prev !== ']') { i = element(i); prev = '>'; continue; }
      if (!/\s/.test(c)) prev = c;
      i++;
    }
    return i;
  }

  /** At `<` of an opening tag: the tag, its attributes, its children and its closing tag. */
  function element(i: number): number {
    i++;
    while (/[\w.:>-]/.test(s[i] ?? '') && s[i] !== '>') i++;
    for (;;) { // attributes
      while (/\s/.test(s[i] ?? '')) i++;
      if (s[i] === '/' && s[i + 1] === '>') return i + 2;
      if (s[i] === '>') { i++; break; }
      if (s[i] === '{') { i = code(i + 1, true); continue; } // {...props}
      if (i >= s.length) return i;
      const start = i;
      while (/[\w:-]/.test(s[i] ?? '')) i++;
      const name = s.slice(start, i);
      if (i === start) { i++; continue; }
      if (s[i] !== '=') continue;
      i++;
      if (s[i] === '"' || s[i] === "'") {
        const end = string(i), text = s.slice(i + 1, end - 1);
        if (LABELS.has(name) && hasLetter(text)) out.push({ line: line(i), kind: 'attribute', text });
        else if (hasCyrillic(text)) out.push({ line: line(i), kind: 'cyrillic', text });
        i = end;
      } else if (s[i] === '{') {
        let j = i + 1;
        while (/\s/.test(s[j])) j++;
        if (LABELS.has(name) && /["'`]/.test(s[j])) { // title={'text'}
          const end = s[j] === '`' ? template(j) : string(j), text = s.slice(j + 1, end - 1);
          if (!text.includes('${') && hasLetter(text)) out.push({ line: line(j), kind: 'attribute', text });
        }
        i = code(i + 1, true);
      }
    }
    for (;;) { // children
      const start = i;
      while (i < s.length && s[i] !== '<' && s[i] !== '{') i++;
      const text = s.slice(start, i).replace(/\s+/g, ' ').trim();
      if (hasLetter(text)) out.push({ line: line(start), kind: 'jsx text', text });
      if (i >= s.length) return i;
      if (s[i] === '{') { i = code(i + 1, true); continue; }
      if (s[i + 1] === '/') return s.indexOf('>', i) + 1;
      i = element(i);
    }
  }

  code(0, false);
  return out;
}

const files = (dir: string): string[] => fs.readdirSync(dir, { withFileTypes: true })
  .flatMap(e => (e.isDirectory() ? files(path.join(dir, e.name)) : /\.tsx?$/.test(e.name) ? [path.join(dir, e.name)] : []));

const sources = files(SRC).map(f => path.relative(SRC, f))
  .filter(f => !/\.test\.tsx?$/.test(f) && !f.startsWith('shared/lib/catalog/'));

describe('the scanner', () => {
  const kinds = (src: string) => scan(src).map(f => `${f.kind}: ${f.text}`);

  it('finds text between tags, in any depth, in Cyrillic and in Latin', () => {
    expect(kinds('const A = () => <div><b>Вся схема</b> and <i>more</i></div>;')).toEqual(['jsx text: Вся схема', 'jsx text: and', 'jsx text: more']);
    expect(kinds('const A = () => (\n  <Panel x="1">\n    Close\n    {n}\n  </Panel>\n);')).toEqual(['jsx text: Close']);
  });

  it('finds literal title, aria-label, placeholder and alt, written either way', () => {
    expect(kinds('const A = () => <button title="Тема (t)" aria-label=\'Close\' placeholder={"name"} alt={`logo`} />;'))
      .toEqual(['attribute: Тема (t)', 'attribute: Close', 'attribute: name', 'attribute: logo']);
  });

  it('finds Cyrillic in any string, template or nested template, but not in a comment or a regex', () => {
    expect(kinds("const a = 'ошибка'; const b = `шаг ${n}`; const c = `${x ? `да` : ''}`;")).toEqual(['cyrillic: ошибка', 'cyrillic: шаг', 'cyrillic: да']);
    expect(kinds("// «Вся схема»\n/* ещё */ const r = /[а-я]'/u; const s = 'ok';")).toEqual([]);
  });

  it('leaves alone what is not text: keys, symbols, classes, generics, comparisons, expressions', () => {
    const ok = [
      "const A = () => <button className=\"all\" title={t('tool.fit')} aria-label={t('bar.prev')}>{t('bar.all')}</button>;",
      'const A = () => <span>{a} / {b}</span>;',
      'const A = () => <div>→ · … ×</div>;',
      "const [x, setX] = useState<string>(''); const y = a < b && c > d; const z = Array<number>(3);",
      'const A = () => <Panel position="top-left" className="head" data-x="y">{ok && <i />}</Panel>;',
      "const f = (e: Event) => { if (e.key === 'Enter') return a > 0 ? 'x' : 'y'; };",
      'const A = () => <button onClick={() => go(-1)} title={on ? a : b}>{icon}</button>;',
    ];
    for (const src of ok) expect(kinds(src), src).toEqual([]);
  });

  it('reports the line', () => {
    expect(scan("const a = 1;\nconst b = 'ещё';")[0].line).toBe(2);
  });
});

describe('no hard-coded user-facing text', () => {
  const found = sources.flatMap(file => scan(fs.readFileSync(path.join(SRC, file), 'utf8'))
    .filter(f => f.kind === 'cyrillic' || file.endsWith('.tsx'))
    .map(f => ({ file, ...f })));
  const allowed = (f: { file: string; text: string }) => ALLOWED.some(a => a.file === f.file && (a.text === '*' || a.text === f.text));

  it('every string a person reads is in a catalog', () => {
    expect(found.filter(f => !allowed(f)).map(f => `${f.file}:${f.line} ${f.kind} «${f.text}»`)).toEqual([]);
  });

  it('the allow-list holds only what is still there', () => {
    expect(ALLOWED.filter(a => !found.some(f => f.file === a.file && (a.text === '*' || a.text === f.text))).map(a => `${a.file} «${a.text}»`)).toEqual([]);
  });
});
