import { useCallback, useState } from 'react';

/** A short message that clears itself (unless a newer one replaced it). */
export function useToast(ms = 5000) {
  const [msg, setMsg] = useState('');
  const toast = useCallback((m: string) => { setMsg(m); setTimeout(() => setMsg(x => (x === m ? '' : x)), ms); }, [ms]);
  return { msg, toast };
}
