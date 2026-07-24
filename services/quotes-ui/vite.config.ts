import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

/**
 * Vite build configuration.
 *
 * Unlike tarot-ui this app is mounted at the ROOT of the domain, so `base`
 * defaults to `/`. Output goes to `dist/client`, resolved by the Node server
 * relative to its own compiled location.
 */
export default defineConfig({
  plugins: [react()],
  base: process.env['BASE_PATH'] === undefined ? '/' : `${process.env['BASE_PATH']}/`,
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
