import { Undo2 } from 'lucide-react';
import { t } from '@shared/lib';
import { ToolButton } from '@shared/ui';

export function UndoButton({ onUndo }: { onUndo: () => Promise<void> }) {
  return <ToolButton title={t('tool.undo')} onClick={() => void onUndo()}><Undo2 size={16} /></ToolButton>;
}
