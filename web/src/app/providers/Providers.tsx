import type { ReactNode } from 'react';
import { ReactFlowProvider } from '@xyflow/react';
import { ApiContext, type Api } from '@shared/api';
import { ModeContext, type PageMode } from '@shared/lib';

/** The page mode and the API client (which follows it) are decided once and provided to everything below. */
export function Providers({ mode, api, children }: { mode: PageMode; api: Api; children: ReactNode }) {
  return (
    <ModeContext value={mode}>
      <ApiContext value={api}>
        <ReactFlowProvider>{children}</ReactFlowProvider>
      </ApiContext>
    </ModeContext>
  );
}
