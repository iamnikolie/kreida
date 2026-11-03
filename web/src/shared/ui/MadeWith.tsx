import { t } from '../lib/i18n';
import { useMode } from '../lib/mode';
import './made-with.css';

/**
 * «Сделано в kreida»: a small muted link to kreida's front page. Only on the page kreida cloud serves (`/b/<token>`) -
 * never on the local server's share link, in the editor, in an exported file or in a headless capture. `_top`, so it
 * leaves an iframe the board is embedded in.
 */
export function MadeWith({ className = '' }: { className?: string }) {
  const { cloud, capture } = useMode();
  if (!cloud || capture) return null;
  return <a className={`made-with ${className}`.trim()} href="/" target="_top">{t('credit.made')}</a>;
}
