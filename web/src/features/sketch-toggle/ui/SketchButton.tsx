import { PenTool } from 'lucide-react';
import { t } from '@shared/lib';
import { ToolButton } from '@shared/ui';
import type { Sketch } from '../model/useSketch';

export function SketchButton({ sketch }: { sketch: Sketch }) {
  return <ToolButton title={t('tool.sketch')} on={sketch.on} onClick={sketch.toggle}><PenTool size={16} /></ToolButton>;
}
