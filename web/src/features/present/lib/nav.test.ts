import { describe, expect, it } from 'vitest';
import { barVisible, hasSteps, nextStep, startStep } from './nav';

describe('nextStep', () => {
  it('→ from "everything" enters the first step, ← enters the last', () => {
    expect(nextStep(null, 1, 3)).toBe(0);
    expect(nextStep(null, -1, 3)).toBe(2);
  });
  it('walks the steps and falls back to "everything" past either end', () => {
    expect(nextStep(0, 1, 3)).toBe(1);
    expect(nextStep(1, -1, 3)).toBe(0);
    expect(nextStep(2, 1, 3)).toBeNull();
    expect(nextStep(0, -1, 3)).toBeNull();
  });
  it('no steps: nothing to walk', () => {
    expect(nextStep(null, 1, 0)).toBeNull();
    expect(nextStep(4, 1, 0)).toBeNull();
  });
});

describe('startStep', () => {
  it('?step=N is 1-based', () => {
    expect(startStep(new URLSearchParams('step=1'))).toBe(0);
    expect(startStep(new URLSearchParams('step=3'))).toBe(2);
    expect(startStep(new URLSearchParams('present=1'))).toBeNull();
  });
});

describe('hasSteps', () => {
  it('declared steps, or more than one top-level item', () => {
    expect(hasSteps(3, false)).toBe(true);
    expect(hasSteps(1, true)).toBe(true);
    expect(hasSteps(1, false)).toBe(false);
    expect(hasSteps(0, false)).toBe(false);
  });
});

describe('barVisible', () => {
  const base = { on: true, count: 2, declared: false, capture: false, step: null };
  it('only while presenting', () => {
    expect(barVisible(base)).toBe(true);
    expect(barVisible({ ...base, on: false })).toBe(false);
  });
  it('needs more than one step, or declared steps (a single declared step still gets its caption)', () => {
    expect(barVisible({ ...base, count: 1 })).toBe(false);
    expect(barVisible({ ...base, count: 1, declared: true })).toBe(true);
  });
  it('a headless capture shows it only on a chosen step', () => {
    expect(barVisible({ ...base, capture: true })).toBe(false);
    expect(barVisible({ ...base, capture: true, step: 1 })).toBe(true);
  });
});
