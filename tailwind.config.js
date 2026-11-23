/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: 'class',
  content: [
    './ui/**/*.html',
    './ui/**/*.js'
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Pretendard', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'system-ui', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace'],
      },
      colors: {
        chzzk: {
          green: '#00FFA3',
          dark: '#0B0E11',
          card: '#141921',
          border: '#1E2636',
          input: '#0F131A',
          hover: '#1A2233',
          text: '#A8B3C7',
          muted: '#5A6577',
          accent: '#00E59B',
          danger: '#FF4D6A',
          warn: '#FFB84D',
        },
        obsBg: '#0D1117',
        obsCard: '#161B22',
        obsBorder: '#30363D',
        obsBorderSubtle: '#21262D',
        obsAccent: '#00FFA3',
        obsText: '#E6EDF3',
        obsMuted: '#8B949E',
        obsRowHover: '#1C2129',
      },
      animation: {
        'fade-in': 'fadeIn 0.25s ease-out',
        'slide-up': 'slideUp 0.25s cubic-bezier(0.16, 1, 0.3, 1)',
        'toast-in': 'toastIn 0.3s cubic-bezier(0.16, 1, 0.3, 1)',
        'toast-out': 'toastOut 0.25s ease-in forwards',
      },
      keyframes: {
        fadeIn: { '0%': { opacity: '0' }, '100%': { opacity: '1' } },
        slideUp: { '0%': { opacity: '0', transform: 'translateY(6px)' }, '100%': { opacity: '1', transform: 'translateY(0)' } },
        toastIn: { '0%': { opacity: '0', transform: 'translateY(12px) scale(0.96)' }, '100%': { opacity: '1', transform: 'translateY(0) scale(1)' } },
        toastOut: { '0%': { opacity: '1', transform: 'translateY(0) scale(1)' }, '100%': { opacity: '0', transform: 'translateY(0) scale(0.96)' } },
      },
    },
  },
  plugins: [],
};
