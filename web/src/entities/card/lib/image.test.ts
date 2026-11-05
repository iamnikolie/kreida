import { describe, expect, it } from 'vitest';
import type { BNode } from '@shared/api';
import { IMAGE_BROKEN, IMAGE_MAX, PHONE_CARD_MAX, imageHeight, size } from './geometry';

const card = (o: Partial<BNode> = {}): BNode => ({
  id: 'a', title: 'Shot', text: '', kind: 'plain', group: null, icon: null, shape: null, width: null, step: null, fields: null, live: null, ...o,
});
const wide = { sha256: 'x', type: 'image/png', w: 1200, h: 600 };

describe('a card with a picture', () => {
  it('reserves the room the picture takes at the card width', () => {
    const plain = size(card()), pic = size(card({ image: wide }));
    expect(pic.width).toBe(300);
    expect(imageHeight(card({ image: wide }), pic.width)).toBe(131);
    expect(pic.height).toBe(plain.height + 131 + 8);
  });

  it('follows the width the board asks for', () => {
    expect(size(card({ image: wide, width: 500 })).width).toBe(500);
    expect(imageHeight(card({ image: wide }), 500)).toBe(231);
  });

  it('never stands taller than the cap', () => {
    expect(imageHeight(card({ image: { ...wide, w: 300, h: 3000 } }), 300)).toBe(IMAGE_MAX);
  });

  it('keeps a small slot for a picture that cannot be shown', () => {
    expect(imageHeight(card({ image: { error: 'missing' } }), 300)).toBe(IMAGE_BROKEN);
  });

  it('fits a phone', () => {
    const s = size(card({ image: wide }), { phone: true });
    expect(s.width).toBe(PHONE_CARD_MAX);
    expect(imageHeight(card({ image: wide }), s.width)).toBe(65);
  });

  it('adds nothing to a card without one', () => {
    expect(size(card()).width).toBe(236);
    expect(imageHeight(card(), 236)).toBe(0);
  });

  it('adds its room to a note', () => {
    const note = card({ kind: 'note', text: '## Head\n- one' });
    expect(size({ ...note, image: wide }).height).toBeGreaterThan(size(note).height);
  });
});
