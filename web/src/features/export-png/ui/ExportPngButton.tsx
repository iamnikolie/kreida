import { Download } from 'lucide-react';
import { t } from '@shared/lib';
import { ToolButton } from '@shared/ui';
import { useExportPng } from '../model/useExportPng';

export function ExportPngButton({ fileName }: { fileName: string }) {
  const exportPng = useExportPng();
  return <ToolButton title={t('tool.png')} onClick={() => void exportPng(fileName)}><Download size={16} /></ToolButton>;
}
