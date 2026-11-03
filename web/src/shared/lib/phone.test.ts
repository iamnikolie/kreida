import { describe, expect, it } from 'vitest';
import { isPhone, phoneViewport } from './phone';

describe('phoneViewport', () => {
  it('portrait phones', () => {
    expect(phoneViewport(375, 667)).toBe(true);
    expect(phoneViewport(390, 844)).toBe(true);
    expect(phoneViewport(430, 932)).toBe(true);
  });
  it('landscape keeps today\'s viewer', () => {
    expect(phoneViewport(844, 390)).toBe(false);
    expect(phoneViewport(667, 375)).toBe(false);
  });
  it('the short side must be under 600', () => {
    expect(phoneViewport(599, 900)).toBe(true);
    expect(phoneViewport(600, 900)).toBe(false);
    expect(phoneViewport(768, 1024)).toBe(false);
    expect(phoneViewport(1600, 900)).toBe(false);
  });
  it('a square window counts as portrait, as in the orientation media query', () => {
    expect(phoneViewport(500, 500)).toBe(true);
  });
});

describe('isPhone', () => {
  it('only a share link can be a phone', () => {
    expect(isPhone('share', 390, 844)).toBe(true);
    expect(isPhone('editor', 390, 844)).toBe(false);
    expect(isPhone('static', 390, 844)).toBe(false);
  });
  it('a share link turned sideways is not', () => {
    expect(isPhone('share', 844, 390)).toBe(false);
  });
});
