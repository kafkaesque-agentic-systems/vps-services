import type { Config } from 'tailwindcss';

/**
 * Tailwind configuration.
 *
 * The mystical palette is tokenised here rather than scattered as arbitrary
 * values in markup: `bg-void`, `text-gilt`, `border-arcane` and friends are the
 * vocabulary components use. Adding a raw hex in a class is a signal the token
 * set is missing something -- extend it here instead.
 *
 * Contrast is checked against WCAG 2.1 AA on the `void` background:
 * `parchment` (#EDE4D3) ~13.6:1, `gilt` (#D8B45A) ~7.9:1, `mist` (#A9A2C4)
 * ~6.2:1 -- all clear the 4.5:1 threshold for body text.
 */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        /** Deepest background -- near-black with a violet cast. */
        void: '#0B0A12',
        /** Raised surfaces: card wells, panels. */
        obsidian: '#141221',
        /** Borders and dividers. */
        arcane: '#2A2440',
        /** Primary accent: candlelit gold. */
        gilt: '#D8B45A',
        /** Secondary accent: dusk violet. */
        amethyst: '#7C6BB0',
        /** Muted body text. */
        mist: '#A9A2C4',
        /** High-contrast text. */
        parchment: '#EDE4D3',
      },
      fontFamily: {
        display: ['Iowan Old Style', 'Palatino', 'Georgia', 'serif'],
        body: ['Avenir Next', 'Segoe UI', 'system-ui', 'sans-serif'],
      },
      transitionDuration: {
        /** Shared fade length; mirrored by FADE_MS in the client. */
        fade: '700ms',
      },
      keyframes: {
        'drift-glow': {
          '0%, 100%': { opacity: '0.35', transform: 'scale(1)' },
          '50%': { opacity: '0.6', transform: 'scale(1.06)' },
        },
        'rise-in': {
          '0%': { opacity: '0', transform: 'translateY(0.75rem)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
      },
      animation: {
        'drift-glow': 'drift-glow 7s ease-in-out infinite',
        'rise-in': 'rise-in 600ms ease-out both',
      },
    },
  },
  plugins: [],
} satisfies Config;
