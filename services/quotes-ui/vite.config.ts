import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

/**
 * Vite build configuration.
 *
 * Unlike tarot-ui this app is mounted at the ROOT of the domain, so `base`
 * defaults to `/`. Output goes to `dist/client`, resolved by the Node server
 * relative to its own compiled location.
 */
/**
 * Public base path, normalised. The naive `${BASE_PATH}/` produced `//` when
 * BASE_PATH itself was `/` (the Docker build arg for this root-mounted app) —
 * and `//assets/...` is a PROTOCOL-RELATIVE URL, so browsers resolved the
 * bundle against the host "assets" and the live page rendered blank with CSP
 * refusals for https://assets/... . Root stays exactly '/'; sub-paths gain
 * exactly one trailing slash.
 */
const RAW_BASE = process.env['BASE_PATH'] ?? '/';
const BASE = RAW_BASE === '/' ? '/' : `${RAW_BASE.replace(/\/+$/, '')}/`;

export default defineConfig({
  plugins: [react()],
  base: BASE,
  build: {
    // Conservative baseline: a blank page in an older Safari (module parses,
    // React never commits) traced to modern syntax in the default 'modules'
    // target. es2019/safari13 makes esbuild downlevel class fields, optional
    // chaining, nullish coalescing and friends at a few KB cost.
    target: ['es2019', 'safari13'],
    outDir: 'dist/client',
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    port: 5174,
    proxy: {
      '/api': {
        target: 'http://localhost:3200',
        changeOrigin: true,
      },
    },
  },
});
