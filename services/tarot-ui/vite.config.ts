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
/**
 * Public base path, normalised the same way as quotes-ui: a BASE_PATH of `/`
 * must yield `/`, not the protocol-relative `//` (see quotes-ui, where that
 * produced //assets/... URLs and a blank page). This app's default is a
 * sub-path, but the guard costs nothing and protects future re-mounting.
 */
const RAW_BASE = process.env['BASE_PATH'] ?? '/tarot';
const BASE = RAW_BASE === '/' ? '/' : `${RAW_BASE.replace(/\/+$/, '')}/`;

export default defineConfig({
  plugins: [react()],
  base: BASE,
  build: {
    // Conservative baseline (matches quotes-ui): older Safari parsed the
    // default-target bundle to a blank page; es2019/safari13 downlevels the
    // modern syntax involved.
    target: ['es2019', 'safari13'],
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
