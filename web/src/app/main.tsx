import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import '@xyflow/react/dist/style.css';
import '@fontsource/caveat/latin-400.css';
import '@fontsource/caveat/cyrillic-400.css';
import '@fontsource/caveat/latin-700.css';
import '@fontsource/caveat/cyrillic-700.css';
import { pageLocale } from '@shared/lib';
import './styles/tokens.css';
import './styles/base.css';
import { App } from './App';

document.documentElement.lang = pageLocale();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
