import { expect, it } from 'vitest';
import { tone } from './tone';

it('live values: good words read green, bad words red, the rest neutral', () => {
  expect(['up', 'OK', 'running', 'true', '✓ done'].map(tone)).toEqual(['good', 'good', 'good', 'good', 'good']);
  expect(['down', 'Error 500', 'false', 'offline'].map(tone)).toEqual(['bad', 'bad', 'bad', 'bad']);
  expect(['42', '…', '3 открытых'].map(tone)).toEqual(['', '', '']);
});
