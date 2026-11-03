import type { ReactElement } from 'react';
import { icons } from 'lucide-react';
import { brandColor, brandOf } from '@shared/lib';

export type IconC = (p: { className?: string; size?: number; strokeWidth?: number }) => ReactElement | null;

const pascal = (s: string) => s.replace(/(^|[-_ ])(\w)/g, (_, __, c: string) => c.toUpperCase());

// Brand (simple-icons) first, then a lucide name; `icon: none` turns the auto brand icon off.
export function iconFor(icon: string | null, title: string): IconC | null {
  if (icon === 'none') return null;
  const b = brandOf(icon, title);
  if (b) return ({ className, size }) => (
    <svg className={`${className ?? ''} brand`} width={size} height={size} viewBox="0 0 24 24" fill={brandColor(b)} role="img" aria-label={b.title}>
      <path d={b.path} />
    </svg>
  );
  return icon ? (icons as Record<string, IconC>)[pascal(icon)] ?? null : null;
}
