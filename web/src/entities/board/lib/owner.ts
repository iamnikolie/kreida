import type { BoardResp } from '@shared/api';
import type { Mode } from '@shared/lib';

/**
 * The owner's name to show with the board: only on a share link, and only when kreida cloud sent one. The local
 * server sends none, an absent, empty or blank name means no line at all.
 */
export function ownerOf(resp: Pick<BoardResp, 'owner'> | null | undefined, mode: Mode): string {
  return mode === 'share' ? resp?.owner?.trim() ?? '' : '';
}
