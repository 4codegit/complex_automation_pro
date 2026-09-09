/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        base: '#0a0e13', // page background
        panel: '#10151c', // card surface
        panel2: '#151b24', // nested surface / hover
        line: '#1e2630', // borders
        ink: '#e6edf3', // primary text
        mute: '#93a1b0', // secondary text
        dim: '#5c6875', // tertiary text / hints
        accent: '#38bdf8', // interactive accent
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'Segoe UI', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
      },
      animation: {
        'pulse-red': 'pulse-red 1.2s ease-in-out infinite',
        'blink-soft': 'blink-soft 1s steps(2, start) infinite',
      },
      keyframes: {
        'pulse-red': {
          '0%, 100%': { boxShadow: '0 0 0 0 rgba(239, 68, 68, 0.35)' },
          '50%': { boxShadow: '0 0 14px 2px rgba(239, 68, 68, 0.25)' },
        },
        'blink-soft': {
          '0%, 100%': { opacity: '1' },
          '50%': { opacity: '0.35' },
        },
      },
    },
  },
  plugins: [],
};
