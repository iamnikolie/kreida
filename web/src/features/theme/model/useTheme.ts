import { useCallback, useEffect, useState } from 'react';
import { store, stored, useMode } from '@shared/lib';

const KEY = 'kreida.theme';

export interface Theme { theme: string; toggle(): void }

/** Light or dark: `?theme=`, else the remembered choice, else the system's. Written to <html data-theme>. */
export function useTheme(): Theme {
  const { query } = useMode();
  const [theme, setTheme] = useState(() => query.get('theme') ?? stored(KEY) ??
    (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'));
  useEffect(() => { document.documentElement.dataset.theme = theme; store(KEY, theme); }, [theme]);
  const toggle = useCallback(() => setTheme(v => (v === 'dark' ? 'light' : 'dark')), []);
  return { theme, toggle };
}

/** The colour mode React Flow should draw its own chrome in. */
export const colorModeOf = (theme: string) => (theme === 'dark' ? 'dark' : 'light');
