import { useState, type ReactNode } from 'react';
import { ChevronLeft, ChevronRight, Ellipsis } from 'lucide-react';
import { useFitView } from '@features/fit-view';
import type { Presentation } from '@features/present';
import type { Theme } from '@features/theme';
import { t } from '@shared/lib';
import { Sheet } from './Sheet';
import './phone-bar.css';

interface Props {
  presentation: Presentation;
  theme: Theme;
  /** PNG file name, without the extension. */
  fileName: string;
  /** The board's colour key for the sheet, null when the board has none. */
  legend: ReactNode;
  /** The board's open questions for the sheet, null when it has none. */
  questions?: ReactNode;
  /** Fold or unfold the board's groups; null when nothing nests. */
  fold?: { folded: boolean; toggle(): void } | null;
  /** Opens the composer for a comment on the board; absent where the page takes none. */
  onNote?: () => void;
}

/**
 * A phone's one control bar, along the bottom: «Вся схема» on the left, the steps in the middle (only when the board
 * has any) and «⋯» on the right, which opens the sheet with the legend, the theme and the PNG. With the whole board
 * on show the steps say «всё» instead of a number, ‹ is off and › starts step 1.
 */
export function PhoneBar({ presentation, theme, fileName, legend, questions, fold, onNote }: Props) {
  const fit = useFitView();
  const [more, setMore] = useState(false);
  const { step, steps, stepped, go, showAll } = presentation;
  return (
    <>
      <nav className="phonebar" aria-label={t('bar.label')}>
        <button className="all" onClick={() => { showAll(); fit(); }}>{t('bar.all')}</button>
        {stepped && (
          <div className="stepper">
            <button aria-label={t('bar.prev')} disabled={step == null} onClick={() => go(-1)}><ChevronLeft size={20} /></button>
            {step == null
              ? <span className="none" aria-label={t('bar.all')}>{t('bar.none')}</span>
              : <span aria-label={t('bar.step', { n: step + 1, total: steps.length })}>{step + 1} / {steps.length}</span>}
            <button aria-label={t('bar.next')} onClick={() => go(1)}><ChevronRight size={20} /></button>
          </div>
        )}
        <button className="more" aria-label={t('bar.more')} aria-expanded={more} onClick={() => setMore(true)}><Ellipsis size={20} /></button>
      </nav>
      {more && <Sheet theme={theme} fileName={fileName} legend={legend} questions={questions} fold={fold} onNote={onNote} onClose={() => setMore(false)} />}
    </>
  );
}
