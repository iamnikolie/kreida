import { createContext, use } from 'react';
import type { Api } from './client';

export const ApiContext = createContext<Api | null>(null);

export function useApi(): Api {
  const a = use(ApiContext);
  if (!a) throw new Error('useApi outside ApiContext');
  return a;
}
