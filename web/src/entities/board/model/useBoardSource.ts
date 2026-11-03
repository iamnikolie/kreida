import { useCallback, useEffect, useState } from 'react';
import { useApi, type BoardItem, type BoardResp } from '@shared/api';
import { useMode } from '@shared/lib';

/**
 * The open board: which one (`name`), the boards to pick from (`list`) and its latest server response (`resp`).
 * Reloads itself when the server reports a change. Works the same in every page mode - the API client decides
 * where the data comes from (a list only exists in the editor; a share link or an export has the one board).
 */
export function useBoardSource() {
  const api = useApi();
  const page = useMode();
  const [list, setList] = useState<BoardItem[]>([]);
  const [name, setName] = useState<string | null>(page.board);
  const [resp, setResp] = useState<BoardResp | null>(api.inlined);

  const loadList = useCallback(async () => {
    const l = await api.list();
    setList(l); setName(n => n ?? l[0]?.name ?? null);
  }, [api]);
  const load = useCallback(async (key: string) => { setResp(await api.board(key)); }, [api]);

  useEffect(() => { void loadList(); }, [loadList]);
  useEffect(() => {
    if (!name) return;
    void load(name);
    if (!page.readonly) { const u = new URL(location.href); u.searchParams.set('b', name); history.replaceState(null, '', u); }
  }, [name, load, page.readonly]);
  useEffect(() => api.events(key => { void loadList(); if (key === name) void load(key); }), [api, name, load, loadList]);

  return { list, name, setName, resp, reloadList: loadList };
}
