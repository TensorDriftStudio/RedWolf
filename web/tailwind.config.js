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
          500: '#A6192E',
          600: '#8e1426',
          primary: '#A6192E',
          crimson: '#A6192E',
          hover: '#be1f36',
          dark: '#7e1322',
        },
        enterprise: {
          base: '#0d1015',
          header: '#13171f',
          panel: '#181e28',
          card: '#1d2330',
          hover: '#252e3e',
          border: '#2a3344',
          borderSubtle: '#1f2736',
          textMuted: '#8b97a8',
          textDim: '#637083',
        },
        status: {
          activeBg: '#112217',
          activeBorder: '#238636',
          activeText: '#3fb950',
          readyBg: '#122033',
          readyBorder: '#1f6feb',
          readyText: '#58a6ff',
          warnBg: '#2a1d07',
          warnBorder: '#9e6a03',
          warnText: '#d29922',
          errorBg: '#311417',
          errorBorder: '#da3633',
          errorText: '#f85149',
        }
      },
      fontFamily: {
        sans: ['"Inter"', '-apple-system', 'BlinkMacSystemFont', '"Segoe UI"', 'Roboto', 'sans-serif'],
        mono: ['"JetBrains Mono"', '"SFMono-Regular"', 'Menlo', 'Consolas', 'monospace'],
      },
      borderRadius: {
        'sm': '2px',
        'DEFAULT': '4px',
        'md': '4px',
        'lg': '6px',
        'xl': '8px',
      }
    },
  },
  plugins: [],
}
