import type { Config } from 'tailwindcss';

const config: Config = {
  content: ['./src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        bg: {
          0: '#050a1c',
          1: '#0a1530',
          2: '#0f1d3d',
          3: '#1a2752',
          4: '#1d2d5a',
          5: '#26356a',
        },
        border: {
          1: 'rgba(125, 146, 255, 0.18)',
          2: 'rgba(125, 146, 255, 0.34)',
          3: 'rgba(125, 146, 255, 0.50)',
        },
        text: {
          0: '#f0f4ff',
          1: '#c5cee0',
          2: '#94a3b8',
          3: '#64748b',
        },
        acc: {
          1: '#3d5af1',
          2: '#00d4ff',
          3: '#b88aff',
          4: '#ffd166',
          5: '#06d6a0',
          warn: '#f59e0b',
          err: '#ef4444',
        },
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
        display: ['Space Grotesk', 'Inter', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'monospace'],
      },
      borderRadius: {
        DEFAULT: '0.75rem',
      },
      boxShadow: {
        glow: '0 0 60px rgba(61, 90, 241, 0.35)',
        soft: '0 8px 40px rgba(0, 0, 0, 0.45)',
      },
      animation: {
        'pulse-slow': 'pulse 3.5s ease-in-out infinite',
        'spin-slow': 'spin 8s linear infinite',
        shimmer: 'shimmer 2s linear infinite',
        'fade-in': 'fadeIn 0.3s ease-out',
      },
      keyframes: {
        shimmer: {
          from: { backgroundPosition: '-200% 0' },
          to: { backgroundPosition: '200% 0' },
        },
        fadeIn: {
          from: { opacity: '0', transform: 'translateY(4px)' },
          to: { opacity: '1', transform: 'translateY(0)' },
        },
      },
    },
  },
  plugins: [],
};
export default config;
