import { ImageOff } from 'lucide-react';
import { useApi, type BImage } from '@shared/api';
import { useBoardT } from '@shared/lib';
import { IMAGE_BROKEN, imageHeight } from '../lib/geometry';

/**
 * A card's picture, at the height the layout reserved for it. A picture that cannot be shown says why, on the owner's
 * own page only: an export, a share link and a published board never carry the reason, and their card just has no picture.
 */
export function CardImage({ image, board, width }: { image: BImage; board: string; width: number }) {
  const api = useApi();
  const bt = useBoardT(); // what the card says of itself goes into a PNG: the board's language
  if (image.error || !image.sha256) {
    return (
      <div className="cimg bad" style={{ height: IMAGE_BROKEN }}>
        <ImageOff size={14} strokeWidth={1.8} />
        {image.error && bt.t(`card.image.${image.error}`)}
      </div>
    );
  }
  const style = width ? { height: imageHeight({ image }, width) } : { aspectRatio: `${image.w} / ${image.h}` };
  return <img className="cimg" src={api.imageUrl(board, image.sha256)} alt="" draggable={false} style={style} />;
}
