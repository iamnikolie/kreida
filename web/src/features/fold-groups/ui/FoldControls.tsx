import { FoldVertical, UnfoldVertical } from 'lucide-react';
import { t } from '@shared/lib';
import { ToolButton } from '@shared/ui';
import type { Folds } from '../model/useFolds';

const LEVELS = 3;

/** Fold the board's groups to level 1, 2 or 3 (as deep as they nest), or unfold them all. Nothing on a board without groups. */
export function FoldControls({ folds: f }: { folds: Folds }) {
  if (!f.depth) return null;
  return (
    <div className="fold-levels" role="group" aria-label={t('fold.label')}>
      <FoldVertical size={15} className="fold-ic" aria-hidden />
      {Array.from({ length: Math.min(LEVELS, f.depth) }, (_, i) => (
        <ToolButton key={i} title={t('fold.level', { n: i + 1 })} onClick={() => f.level(i + 1)}><span className="lv">{i + 1}</span></ToolButton>
      ))}
      <ToolButton title={t('fold.all')} on={!f.folds.size} onClick={f.unfoldAll}><UnfoldVertical size={16} /></ToolButton>
    </div>
  );
}
