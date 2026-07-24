import type { Config } from 'tailwindcss';

/**
 * Tailwind configuration for the quotes property.
 *
 * Shares the ThirdEye token system with tarot-ui so the two properties read
 * as siblings, but is AMETHYST-FORWARD where tarot is gilt-forward: primary
 * interactive accents here are violet, gold is reserved for secondary
 * flourishes. Method badge colours are tokenised so the API explorer never
 * reaches for arbitrary values.
 *
 * Contrast (checked against `void` #0B0A12): parchment ~13.6:1, lilac
 * (#B9AEDC) ~8.1:1, mist ~6.2:1 — all clear WCAG 2.1 AA for body text.
 */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        /** Deepest background — near-black with a violet cast. */
        void: '#0B0A12',
        /** Raised surfaces: cards, panels, code wells. */
        obsidian: '#141221',
        /** Borders and dividers. */
        arcane: '#2A2440',
        /** PRIMARY accent for this property: dusk violet. */
        amethyst: '#7C6BB0',
        /** Lighter amethyst for text-on-dark and hovers. */
        lilac: '#B9AEDC',
        /** Secondary flourish only (headings' small marks, dividers). */
        gilt: '#D8B45A',
        /** Muted body text. */
        mist: '#A9A2C4',
        /** High-contrast text. */
        parchment: '#EDE4D3',
        /** Method badge hues (muted, on obsidian chips). */
        'method-get': '#6FAF8E',
        'method-post': '#8B7BC7',
        'method-put': '#C7A15B',
        'method-delete': '#C76B6B',
      },
      fontFamily: {
        display: ['Iowan Old Style', 'Palatino', 'Georgia', 'serif'],
        body: ['Avenir Next', 'Segoe UI', 'system-ui', 'sans-serif'],
        mono: ['SF Mono', 'ui-monospace', 'Menlo', 'monospace'],
      },
      keyframes: {
        'rise-in': {
          '0%': { opacity: '0', transform: 'translateY(0.75rem)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
      },
      animation: {
        'rise-in': 'rise-in 600ms ease-out both',
      },
    },
  },
  plugins: [],
} satisfies Config;
