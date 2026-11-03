import { resolve } from 'node:path';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const src = (dir: string) => resolve(import.meta.dirname, 'src', dir);

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@app': src('app'),
      '@pages': src('pages'),
      '@widgets': src('widgets'),
      '@features': src('features'),
      '@entities': src('entities'),
      '@shared': src('shared'),
    },
  },
  // Fonts go inline into the CSS, so `kreida export` (one HTML file) keeps the sketch font.
  build: { assetsInlineLimit: (file: string) => (file.endsWith('.woff2') ? true : undefined) },
  // changeOrigin: the editor API only answers a Host that names its own port.
  server: { host: '127.0.0.1', port: 7789, proxy: { '/api': { target: 'http://127.0.0.1:7788', changeOrigin: true } } },
});
