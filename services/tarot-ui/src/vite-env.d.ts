/// <reference types="vite/client" />

/**
 * Typed build-time environment.
 *
 * Declaring these makes `import.meta.env` strongly typed under `strict`, so a
 * typo in a variable name is a compile error rather than a silent `undefined`
 * at runtime.
 */
interface ImportMetaEnv {
  /**
   * Base URL of the card image store.
   *
   * Defaults to the root-relative `/image` in production, where NGINX serves
   * the image store on the same origin as this app. Set to an absolute URL
   * (e.g. `https://api.thirdeye.live/image`) to run the app outside the
   * gateway during development.
   */
  readonly VITE_IMAGE_BASE?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
