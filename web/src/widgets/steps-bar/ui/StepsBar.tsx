import type { Ref } from 'react';
import { Panel } from '@xyflow/react';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import type { Presentation } from '@features/present';
import { t, useMode } from '@shared/lib';
import './steps-bar.css';

/** Bottom bar of a presentation: ← step n / N → and the step's title. A headless capture shows the caption only. */
export function StepsBar({ presentation, ref }: { presentation: Presentation; /** The panel, for the page to measure what the bar takes of the canvas's foot. */ ref?: Ref<HTMLDivElement> }) {
  const { capture } = useMode();
  if (!presentation.barVisible) return null;
  const { step, steps, go } = presentation;
  return (
    <Panel ref={ref} position="bottom-center" className="steps" role="group" aria-label={t('present.label')}>
      {!capture && <button aria-label={t('bar.prev')} onClick={() => go(-1)}><ChevronLeft size={18} /></button>}
      <span>{step == null ? t('present.all') : `${step + 1} / ${steps.length}`}{step != null && steps[step]?.title && <b>{steps[step].title}</b>}</span>
      {!capture && <button aria-label={t('bar.next')} onClick={() => go(1)}><ChevronRight size={18} /></button>}
    </Panel>
  );
}
