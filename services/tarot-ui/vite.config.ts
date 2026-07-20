import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

/**
 * Vite build configuration.
 *
 * `base` must match the server's BASE_PATH: the app is served from a
 * sub-path behind the NGINX gateway, so emitted asset URLs have to be
 * prefixed or the browser requests `/assets/...` and receives the SPA shell.
 *
 * Output goes to `dist/client`, which the Node server resolves relative to its
 * own compiled location (`dist/server/index.js`).
 */
export default defineConfig({
  plugins: [react()],
  base: process.env['BASE_PATH'] === undefined ? '/tarot/' : `${process.env['BASE_PATH']}/`,
  build: {
    outDir: 'dist/client',
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    port: 5173,
    proxy: {
      // Local `npm run dev` only: forwards BFF calls to the Node server so the
      // client code path is identical in development and production.
      '/tarot/api': {
        target: 'http://localhost:3000',
        changeOrigin: true,
      },
    },
  },
});
