import { useCallback, useState } from 'react';

export function useToggle(initial = false) {
  const [on, set] = useState(initial);
  const toggle = useCallback(() => set(v => !v), []);
  return { on, toggle };
}
