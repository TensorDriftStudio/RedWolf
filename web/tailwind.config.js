/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        redwolf: {
          50: '#fdf2f4',
          100: '#fce7eb',
          200: '#fad1d9',
          500: '#e11d48',
          600: '#be123c',
          primary: '#A6192E',
          crimson: '#A6192E',
          hover: '#c0213b',
          dark: '#7e1322',
        },
        surface: {
          ground: '#07090e',
          card: '#0d131f',
          panel: '#121a2b',
          border: '#1e293b',
          borderMuted: '#162032',
        }
      },
      fontFamily: {
        sans: ['"Plus Jakarta Sans"', 'Inter', 'system-ui', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'monospace'],
      },
      boxShadow: {
        'glow-red': '0 0 20px -3px rgba(166, 25, 46, 0.45)',
        'glow-green': '0 0 15px -3px rgba(16, 185, 129, 0.4)',
        'card': '0 4px 20px -2px rgba(0, 0, 0, 0.5)',
      }
    },
  },
  plugins: [],
}
