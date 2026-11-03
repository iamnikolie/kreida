import { Presentation as PresentIcon } from 'lucide-react';
import { t } from '@shared/lib';
import { ToolButton } from '@shared/ui';
import type { Presentation } from '../model/usePresentation';

export function PresentButton({ presentation }: { presentation: Presentation }) {
  return <ToolButton title={t('tool.present')} onClick={presentation.start}><PresentIcon size={16} /></ToolButton>;
}
