// localStorage that never throws (private mode, blocked storage). Keys moved from `doska.*` to `kreida.*`:
// reading a `kreida.*` key falls back to its old name, so a choice made before the rename survives.
const legacy = (k: string) => k.replace(/^kreida\./, 'doska.');
const read = (k: string) => { try { return localStorage.getItem(k); } catch { return null; } };

export const stored = (k: string) => read(k) ?? (legacy(k) === k ? null : read(legacy(k)));
export const store = (k: string, v: string) => { try { localStorage.setItem(k, v); } catch { /* private mode */ } };
