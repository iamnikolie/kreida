import { pageLocale } from '../lib/i18n';
import type { PageMode } from '../lib/mode';
import type { BoardItem, BoardResp, BoardStyle, LiveValues, Note, Pos } from './contract';

/**
 * The one place that talks to the server. The page mode picks the implementation, so nothing above this file
 * knows whether it runs in the editor, behind a share link or in a standalone export:
 *  - reads (board, events, live values) work in every mode and go to the right URL;
 *  - writes exist only in the editor and reject with ReadOnlyError elsewhere.
 */
export interface Api {
  /** The board response already inlined into the page (a standalone export), else null. */
  readonly inlined: BoardResp | null;

  /** Boards the editor can switch between; empty where there is no such list. */
  list(): Promise<BoardItem[]>;
  /** One board with its saved card positions. `key` is the board name (the share token behind a share link). */
  board(key: string): Promise<BoardResp>;
  /** Calls back with the board key whenever a board changed on the server. Returns the unsubscribe. */
  events(onChange: (key: string) => void): () => void;
  /** Fetches the board's live card values now and then every `everySec`. Returns the stop function. */
  watchLive(key: string, everySec: number, onValues: (v: LiveValues) => void): () => void;
  /** Notes for the agent: the board's open ones in the editor, the holder's own comments on a kreida cloud link. */
  notes(key: string): Promise<Note[]>;
  /** Writes a note (the editor) or a comment (a cloud link that takes them). Resolves to the server's error text, or null. */
  addNote(key: string, note: { node: string; text: string }): Promise<string | null>;

  /** Creates a board. Resolves to the server's error text, or null when it worked. */
  create(name: string): Promise<string | null>;
  savePos(board: string, pos: Pos): Promise<void>;
  resetPos(board: string): Promise<void>;
  /** Steps the board file back one version. Resolves to an error text, or null when it worked. */
  undo(board: string): Promise<string | null>;
  setStyle(board: string, style: BoardStyle): Promise<void>;
  patchNode(board: string, id: string, patch: { title: string; text: string }): Promise<void>;
}

export class ReadOnlyError extends Error {
  constructor(op: string) { super(`${op}: this page is read-only`); }
}

const noop = () => {};
const getJson = async <T>(url: string): Promise<T> => (await fetch(url)).json();
const send = async (method: string, url: string, body?: unknown) => {
  const init: RequestInit = { method };
  if (body !== undefined) Object.assign(init, { body: JSON.stringify(body), headers: { 'content-type': 'application/json' } });
  return fetch(url, init);
};
// The server's `{error}` text of a failed response ('' when it sent none: the page words that itself), null for a good one.
const failure = async (r: Response): Promise<string | null> => (r.ok ? null : ((await r.json()) as { error?: string }).error ?? '');

// Polls `url` now and every `everySec` (once only on a capture page, which has to go idle).
function pollLive(url: string, everySec: number, capture: boolean, onValues: (v: LiveValues) => void) {
  let on = true;
  const tick = async () => {
    try { const r = await fetch(url); if (on && r.ok) onValues(await r.json()); }
    catch { /* server gone: keep the last values */ }
  };
  void tick();
  const t = capture ? undefined : setInterval(tick, everySec * 1000);
  return () => { on = false; clearInterval(t); };
}

function editorApi(page: PageMode): Api {
  const board = (n: string) => `/api/boards/${n}`;
  return {
    inlined: null,
    list: () => getJson<BoardItem[]>('/api/boards'),
    board: n => getJson<BoardResp>(board(n)),
    events(onChange) {
      if (page.capture) return noop; // an open stream never lets a headless render go idle
      const es = new EventSource('/api/events');
      es.onmessage = ev => onChange((JSON.parse(ev.data) as { name: string }).name);
      return () => es.close();
    },
    watchLive: (n, everySec, onValues) => pollLive(`${board(n)}/live`, everySec, page.capture, onValues),
    notes: n => getJson<Note[]>(`${board(n)}/notes`),
    addNote: async (n, note) => failure(await send('POST', `${board(n)}/notes`, note)),

    create: async n => failure(await send('POST', board(n))),
    savePos: async (n, pos) => { await send('PUT', `${board(n)}/pos`, pos); },
    resetPos: async n => { await send('DELETE', `${board(n)}/pos`); },
    undo: async n => failure(await send('POST', `${board(n)}/undo`)),
    setStyle: async (n, style) => { await send('PUT', `${board(n)}/style`, { style }); },
    patchNode: async (n, id, patch) => { await send('PATCH', `${board(n)}/node/${encodeURIComponent(id)}`, patch); },
  };
}

const readOnlyWrites: Pick<Api, 'create' | 'savePos' | 'resetPos' | 'undo' | 'setStyle' | 'patchNode'> = {
  create: () => Promise.reject(new ReadOnlyError('create')),
  savePos: () => Promise.reject(new ReadOnlyError('savePos')),
  resetPos: () => Promise.reject(new ReadOnlyError('resetPos')),
  undo: () => Promise.reject(new ReadOnlyError('undo')),
  setStyle: () => Promise.reject(new ReadOnlyError('setStyle')),
  patchNode: () => Promise.reject(new ReadOnlyError('patchNode')),
};

// /s/<token> and /b/<token>: the one shared board, whatever key the page asks for. Its events carry no payload worth reading.
// The board read names the page's language: kreida cloud picks the owner's name in it when the link's own language is
// "auto" (the local server ignores query parameters it does not know).
function shareApi(page: PageMode): Api {
  const root = `/api/share/${page.share}`;
  return {
    ...readOnlyWrites,
    inlined: null,
    list: async () => [],
    board: () => getJson<BoardResp>(`${root}?lang=${pageLocale()}`),
    events(onChange) {
      if (page.capture) return noop;
      const es = new EventSource(`${root}/events`);
      es.onmessage = () => onChange(page.board ?? '');
      return () => es.close();
    },
    watchLive: (_, everySec, onValues) => pollLive(`${root}/live`, everySec, page.capture, onValues),
    // only kreida cloud takes comments, and only on a link whose board read says `comments: true`
    notes: () => getJson<Note[]>(`${root}/comments`),
    addNote: async (_, note) => failure(await send('POST', `${root}/comments`, note)),
  };
}

// Standalone export: the board is in the page, there is no server to ask.
function staticApi(page: PageMode): Api {
  return {
    ...readOnlyWrites,
    inlined: page.inlined,
    list: async () => [],
    board: async () => page.inlined!,
    events: () => noop,
    watchLive: () => noop,
    notes: async () => [],
    addNote: () => Promise.reject(new ReadOnlyError('addNote')),
  };
}

export function createApi(page: PageMode): Api {
  switch (page.mode) {
    case 'editor': return editorApi(page);
    case 'share': return shareApi(page);
    case 'static': return staticApi(page);
  }
}
