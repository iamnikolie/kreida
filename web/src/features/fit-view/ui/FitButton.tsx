import { Maximize } from 'lucide-react';
import { t } from '@shared/lib';
import { ToolButton } from '@shared/ui';
import { useFitView } from '../model/fit';

export function FitButton() {
  const fit = useFitView();
  return <ToolButton title={t('tool.fit')} onClick={fit}><Maximize size={16} /></ToolButton>;
}
