import { Map as MapIcon } from 'lucide-react';
import { ExportPngButton } from '@features/export-png';
import { NoteButton } from '@features/card-note';
import { FitButton } from '@features/fit-view';
import { FoldControls, type Folds } from '@features/fold-groups';
import { PresentButton, type Presentation } from '@features/present';
import { RelayoutButton } from '@features/relayout';
import { SketchButton, type Sketch } from '@features/sketch-toggle';
import { ThemeButton, type Theme } from '@features/theme';
import { UndoButton } from '@features/undo';
import { LockButton, type ViewLock } from '@features/view-lock';
import { t, useEditable, useMode } from '@shared/lib';
import { ToolButton } from '@shared/ui';
import './toolbar.css';

interface Props {
  /** The open board, null before one is chosen. */
  board: string | null;
  /** PNG file name, without the extension. */
  fileName: string;
  presentation: Presentation;
  theme: Theme;
  sketch: Sketch;
  minimap: { on: boolean; toggle(): void };
  undo: () => Promise<void>;
  lock?: ViewLock;
  folds?: Folds;
  /** Opens the composer for a note on the whole board; absent where the page takes no notes. */
  onNote?: (at: { x: number; y: number }) => void;
}

/**
 * The icon row at the top right, hidden while presenting: the view-only lock and the editing tools only in the editor
 * (the tools give way while it is locked), the fold levels where groups nest, the board note where the page takes notes.
 */
export function Toolbar({ board, fileName, presentation, theme, sketch, minimap, undo, lock, folds, onNote }: Props) {
  const { readonly } = useMode();
  const editable = useEditable();
  if (presentation.on) return null;
  return (
    <div className="tools" role="toolbar" aria-label={t('tool.bar')}>
      {!readonly && lock && <LockButton lock={lock} />}
      {editable && <UndoButton onUndo={undo} />}
      <FitButton />
      {editable && <RelayoutButton board={board} />}
      {folds && <FoldControls folds={folds} />}
      {onNote && <NoteButton onOpen={onNote} />}
      <ToolButton title={t('tool.minimap')} on={minimap.on} onClick={minimap.toggle}><MapIcon size={16} /></ToolButton>
      <ThemeButton theme={theme} />
      <SketchButton sketch={sketch} />
      <ExportPngButton fileName={fileName} />
      <PresentButton presentation={presentation} />
    </div>
  );
}
