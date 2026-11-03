import type { ReactNode } from 'react';
import { Download, FoldVertical, MessageSquarePlus, Moon, Sun, UnfoldVertical } from 'lucide-react';
import { useExportPng } from '@features/export-png';
import type { Theme } from '@features/theme';
import { t, useHotkeys } from '@shared/lib';
import { MadeWith } from '@shared/ui';

interface Props {
  theme: Theme;
  fileName: string;
  legend: ReactNode;
  questions?: ReactNode;
  fold?: { folded: boolean; toggle(): void } | null;
  onNote?: () => void;
  onClose(): void;
}

/** The bottom sheet behind «⋯»: Открытые вопросы, Легенда, Свернуть группы, a comment on the board (where the page takes them), Тема, Скачать PNG and, on a kreida cloud link, «Сделано в kreida» at its foot. A tap outside it closes it; so does the PNG, once it is on its way. */
export function Sheet({ theme, fileName, legend, questions, fold, onNote, onClose }: Props) {
  const exportPng = useExportPng();
  useHotkeys([{ keys: ['Escape'], run: onClose }]);
  return (
    <>
      <button className="sheet-backdrop" aria-label={t('sheet.close')} onClick={onClose} />
      <div className="sheet" role="dialog" aria-modal="true" aria-label={t('bar.more')}>
        {questions && <section><h2>{t('sheet.questions')}</h2>{questions}</section>}
        {legend && <section><h2>{t('sheet.legend')}</h2>{legend}</section>}
        {fold && (
          <button className="row" onClick={() => { fold.toggle(); onClose(); }}>
            {fold.folded ? <UnfoldVertical size={18} /> : <FoldVertical size={18} />}{fold.folded ? t('sheet.unfold') : t('sheet.fold')}
          </button>
        )}
        {onNote && <button className="row" onClick={() => { onClose(); onNote(); }}><MessageSquarePlus size={18} />{t('note.commentBoard')}</button>}
        <button className="row" onClick={theme.toggle}>
          {theme.theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}{t('sheet.theme')}
        </button>
        <button className="row" onClick={() => { onClose(); void exportPng(fileName); }}><Download size={18} />{t('sheet.png')}</button>
        <MadeWith className="foot" />
      </div>
    </>
  );
}
