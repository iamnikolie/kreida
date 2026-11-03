import type { ReactNode } from 'react';

/** One icon button of the toolbar (the look comes from the toolbar's `.tools button`). `title` is its tooltip and its accessible name. */
export function ToolButton({ title, on, onClick, children }: { title: string; on?: boolean; onClick: () => void; children: ReactNode }) {
  return <button title={title} aria-label={title} aria-pressed={on} className={on ? 'on' : undefined} onClick={onClick}>{children}</button>;
}
