import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { detectMode } from '../lib/mode';
import type { BoardResp } from './contract';
import { createApi, ReadOnlyError } from './client';

const inlined = { name: 'inlined' } as BoardResp;
const editor = (search = '') => createApi(detectMode({ pathname: '/', search }, {}));
const share = (search = '') => createApi(detectMode({ pathname: '/s/tok-0123456789abcdef', search }, {}));
const exported = (search = '') => createApi(detectMode({ pathname: '/', search }, { __KREIDA__: inlined }));

const reply = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
let fetchMock: ReturnType<typeof vi.fn>;

class FakeSource {
  static all: FakeSource[] = [];
  onmessage: ((ev: { data: string }) => void) | null = null;
  closed = false;
  constructor(public url: string) { FakeSource.all.push(this); }
  close() { this.closed = true; }
}

/** The page as a browser would hand it to pageLocale(): a `kreida-lang` meta tag (or none) and the browser's languages. */
const stubPage = (meta: string | null, languages: string[] = ['en-US']) => {
  vi.stubGlobal('document', { querySelectorAll: () => (meta ? [{ getAttribute: () => meta }] : []) });
  vi.stubGlobal('location', { search: '', pathname: '/s/tok-0123456789abcdef' });
  vi.stubGlobal('navigator', { languages, language: languages[0] });
};

beforeEach(() => {
  fetchMock = vi.fn(async () => reply({}));
  vi.stubGlobal('fetch', fetchMock);
  stubPage('uk'); // the page language is read once per module load: this file's pages are Ukrainian
  vi.stubGlobal('EventSource', FakeSource);
  FakeSource.all = [];
  vi.useFakeTimers();
});
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

describe('reads follow the mode', () => {
  it('editor: boards by name', async () => {
    fetchMock.mockResolvedValueOnce(reply([{ name: 'a' }])).mockResolvedValueOnce(reply({ name: 'a' }));
    const api = editor();
    expect(await api.list()).toEqual([{ name: 'a' }]);
    expect(await api.board('a')).toEqual({ name: 'a' });
    expect(fetchMock.mock.calls.map(c => c[0])).toEqual(['/api/boards', '/api/boards/a']);
  });

  it('share: always the shared board, no list', async () => {
    fetchMock.mockResolvedValueOnce(reply({ name: 'real' }));
    const api = share();
    expect(await api.list()).toEqual([]);
    expect(await api.board('whatever')).toEqual({ name: 'real' });
    expect(fetchMock.mock.calls.map(c => c[0])).toEqual(['/api/share/tok-0123456789abcdef?lang=uk']);
  });

  it('share: the board read names the page language, the events and live URLs stay as they are', async () => {
    vi.resetModules(); // the language is read once per module load
    stubPage(null, ['ru-RU']);
    const { createApi: create } = await import('./client');
    const { detectMode: detect } = await import('../lib/mode');
    const api = create(detect({ pathname: '/b/tok-0123456789abcdef', search: '' }, {}));
    await api.board('x');
    api.events(() => {})();
    api.watchLive('x', 30, () => {})();
    await vi.advanceTimersByTimeAsync(0);
    expect(fetchMock.mock.calls.map(c => c[0])).toEqual(['/api/share/tok-0123456789abcdef?lang=ru', '/api/share/tok-0123456789abcdef/live']);
    expect(FakeSource.all.map(s => s.url)).toEqual(['/api/share/tok-0123456789abcdef/events']);
  });

  it('editor: the board read carries no language', async () => {
    await editor().board('a');
    expect(fetchMock.mock.calls[0][0]).toBe('/api/boards/a');
  });

  it('static: the inlined board, no network', async () => {
    const api = exported();
    expect(api.inlined).toBe(inlined);
    expect(await api.board('x')).toBe(inlined);
    expect(await api.list()).toEqual([]);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('only the exported page has an inlined board', () => {
    expect(editor().inlined).toBeNull();
    expect(share().inlined).toBeNull();
  });
});

describe('events', () => {
  it('editor: one stream, reports the changed board by name', () => {
    const seen: string[] = [];
    const stop = editor().events(k => seen.push(k));
    expect(FakeSource.all.map(s => s.url)).toEqual(['/api/events']);
    FakeSource.all[0].onmessage!({ data: JSON.stringify({ name: 'workflow' }) });
    expect(seen).toEqual(['workflow']);
    stop();
    expect(FakeSource.all[0].closed).toBe(true);
  });

  it('share: the token stream, every message means the page board changed', () => {
    const seen: string[] = [];
    share().events(k => seen.push(k));
    expect(FakeSource.all.map(s => s.url)).toEqual(['/api/share/tok-0123456789abcdef/events']);
    FakeSource.all[0].onmessage!({ data: JSON.stringify({ name: 'real-name' }) });
    expect(seen).toEqual(['tok-0123456789abcdef']);
  });

  it('static: nothing to listen to', () => {
    exported().events(() => {})();
    expect(FakeSource.all).toEqual([]);
  });

  it('a capture page opens no stream', () => {
    editor('?static').events(() => {})();
    share('?static').events(() => {})();
    expect(FakeSource.all).toEqual([]);
  });
});

describe('watchLive', () => {
  it('editor: the board route, now and on every interval, until stopped', async () => {
    fetchMock.mockImplementation(async () => reply({ c: { value: 'up' } }));
    const seen: unknown[] = [];
    const stop = editor().watchLive('lab', 30, v => seen.push(v));
    await vi.advanceTimersByTimeAsync(0);
    expect(seen).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(seen).toHaveLength(2);
    expect(fetchMock.mock.calls.every(c => c[0] === '/api/boards/lab/live')).toBe(true);
    stop();
    await vi.advanceTimersByTimeAsync(90_000);
    expect(seen).toHaveLength(2);
  });

  it('share: the token route whatever key is passed', async () => {
    share().watchLive('ignored', 30, () => {});
    await vi.advanceTimersByTimeAsync(0);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/share/tok-0123456789abcdef/live');
  });

  it('a capture page polls once', async () => {
    editor('?static').watchLive('lab', 30, () => {});
    await vi.advanceTimersByTimeAsync(120_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('static: never polls', async () => {
    exported().watchLive('lab', 30, () => {})();
    await vi.advanceTimersByTimeAsync(120_000);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('keeps the last values when the server fails or answers badly', async () => {
    const seen: unknown[] = [];
    fetchMock.mockResolvedValueOnce(reply({ error: 'x' }, 500)).mockRejectedValueOnce(new Error('gone'));
    editor().watchLive('lab', 30, v => seen.push(v));
    await vi.advanceTimersByTimeAsync(30_000);
    expect(seen).toEqual([]);
  });
});

describe('editor writes', () => {
  const call = (i = 0) => fetchMock.mock.calls[i] as [string, RequestInit];

  it('positions: PUT the map, DELETE to reset', async () => {
    await editor().savePos('lab', { a: { x: 1, y: 2 } });
    await editor().resetPos('lab');
    expect(call(0)[0]).toBe('/api/boards/lab/pos');
    expect(call(0)[1]).toMatchObject({ method: 'PUT', body: JSON.stringify({ a: { x: 1, y: 2 } }) });
    expect(call(1)).toEqual(['/api/boards/lab/pos', { method: 'DELETE' }]);
  });

  it('style and node patch', async () => {
    await editor().setStyle('lab', 'sketch');
    await editor().patchNode('lab', 'a b', { title: 'T', text: 'D' });
    expect(call(0)[0]).toBe('/api/boards/lab/style');
    expect(call(0)[1]).toMatchObject({ method: 'PUT', body: JSON.stringify({ style: 'sketch' }) });
    expect(call(1)[0]).toBe('/api/boards/lab/node/a%20b');
    expect(call(1)[1]).toMatchObject({ method: 'PATCH', body: JSON.stringify({ title: 'T', text: 'D' }) });
  });

  it('undo and create resolve to the server error, an empty text when it sent none, or null', async () => {
    fetchMock.mockResolvedValueOnce(reply({}, 200))
      .mockResolvedValueOnce(reply({ error: 'nothing to undo' }, 409))
      .mockResolvedValueOnce(reply({}, 500))
      .mockResolvedValueOnce(reply({ error: 'exists' }, 409))
      .mockResolvedValueOnce(reply({ ok: true }));
    const api = editor();
    expect(await api.undo('lab')).toBeNull();
    expect(await api.undo('lab')).toBe('nothing to undo');
    expect(await api.undo('lab')).toBe('');
    expect(await api.create('lab')).toBe('exists');
    expect(await api.create('new')).toBeNull();
    expect(call(0)).toMatchObject(['/api/boards/lab/undo', { method: 'POST' }]);
    expect(call(3)).toMatchObject(['/api/boards/lab', { method: 'POST' }]);
  });
});

describe('writes outside the editor', () => {
  const writes = (api: ReturnType<typeof editor>) => [
    () => api.create('x'), () => api.savePos('x', {}), () => api.resetPos('x'), () => api.undo('x'),
    () => api.setStyle('x', 'sketch'), () => api.patchNode('x', 'a', { title: '', text: '' }),
  ];

  it.each([['share', share], ['static', exported]])('%s rejects every write and sends nothing', async (_, make) => {
    for (const w of writes(make())) await expect(w()).rejects.toBeInstanceOf(ReadOnlyError);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('notes for the agent', () => {
  const note = { id: 'n1', node: 'a', text: 'move it', author: '', created: '2026-10-30T10:00:00Z' };

  it('editor: the board\'s open notes, and a new one posted as JSON', async () => {
    fetchMock.mockResolvedValueOnce(reply([note])).mockResolvedValueOnce(reply(note, 201));
    const api = editor();
    expect(await api.notes('a')).toEqual([note]);
    expect(await api.addNote('a', { node: 'x', text: 'hi' })).toBeNull();
    expect(fetchMock.mock.calls[0][0]).toBe('/api/boards/a/notes');
    const [url, init] = fetchMock.mock.calls[1];
    expect([url, init.method, init.headers['content-type'], JSON.parse(init.body)]).toEqual(['/api/boards/a/notes', 'POST', 'application/json', { node: 'x', text: 'hi' }]);
  });

  it('a cloud link: the holder\'s comments, and the server\'s error text when one is refused', async () => {
    fetchMock.mockResolvedValueOnce(reply([note])).mockResolvedValueOnce(reply({ error: 'слишком часто' }, 429));
    const api = createApi(detectMode({ pathname: '/b/tok-0123456789abcdef', search: '' }, {}));
    expect(await api.notes('whatever')).toEqual([note]);
    expect(await api.addNote('whatever', { node: '', text: 'hi' })).toBe('слишком часто');
    expect(fetchMock.mock.calls.map(c => c[0])).toEqual(['/api/share/tok-0123456789abcdef/comments', '/api/share/tok-0123456789abcdef/comments']);
  });

  it('an export takes no notes', async () => {
    const api = exported();
    expect(await api.notes('a')).toEqual([]);
    await expect(api.addNote('a', { node: '', text: 'x' })).rejects.toBeInstanceOf(ReadOnlyError);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
