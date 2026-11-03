import type { Presentation } from '@features/present';
import type { Sketch } from '@features/sketch-toggle';
import type { Theme } from '@features/theme';
import type { ViewLock } from '@features/view-lock';
import { useHotkeys } from '@shared/lib';

interface Actions {
  presentation: Presentation;
  theme: Theme;
  sketch: Sketch;
  minimap: { toggle(): void };
  fit(): void;
  undo(): Promise<void>;
  lock: ViewLock;
}

/** The page's keyboard, all in one table (SKILL.md lists the same keys). Typing in a field or a card never triggers them. */
export function useBoardHotkeys({ presentation: p, theme, sketch, minimap, fit, undo, lock }: Actions) {
  useHotkeys([
    { keys: ['z'], mod: true, prevent: true, run: () => void undo() },
    { keys: ['p'], run: p.toggle },
    { keys: ['Home'], when: p.on, run: p.showAll },
    { keys: ['Escape'], run: p.exit },
    { keys: ['t'], run: theme.toggle },
    { keys: ['m'], run: minimap.toggle },
    { keys: ['s'], run: sketch.toggle },
    { keys: ['l'], run: lock.toggle },
    { keys: ['f', '0'], run: fit },
    { keys: ['ArrowRight', ' '], when: p.on, prevent: true, run: () => p.go(1) },
    { keys: ['ArrowLeft'], when: p.on, run: () => p.go(-1) },
  ]);
}
