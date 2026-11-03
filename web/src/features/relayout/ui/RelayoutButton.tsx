import { LayoutGrid } from 'lucide-react';
import { t } from '@shared/lib';
import { ToolButton } from '@shared/ui';
import { useRelayout } from '../model/useRelayout';

export function RelayoutButton({ board }: { board: string | null }) {
  const relayout = useRelayout(board);
  return <ToolButton title={t('tool.relayout')} onClick={relayout}><LayoutGrid size={16} /></ToolButton>;
}
