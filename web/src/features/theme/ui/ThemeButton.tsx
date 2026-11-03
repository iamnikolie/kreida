import { Moon, Sun } from 'lucide-react';
import { t } from '@shared/lib';
import { ToolButton } from '@shared/ui';
import type { Theme } from '../model/useTheme';

export function ThemeButton({ theme }: { theme: Theme }) {
  return (
    <ToolButton title={t('tool.theme')} onClick={theme.toggle}>
      {theme.theme === 'dark' ? <Sun size={16} /> : <Moon size={16} />}
    </ToolButton>
  );
}
