import { BoardPage } from '@pages/board';
import { createApi } from '@shared/api';
import { detectMode } from '@shared/lib';
import { Providers } from './providers/Providers';

// How the page was opened (editor, share link, standalone export) is read once, here.
const mode = detectMode();
const api = createApi(mode);

export function App() {
  return (
    <Providers mode={mode} api={api}>
      <BoardPage />
    </Providers>
  );
}
