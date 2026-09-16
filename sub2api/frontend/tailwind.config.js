/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // 主色调 - 冰青
        primary: {
          50: '#f0fbfc',
          100: '#d9f5f7',
          200: '#b8eaef',
          300: '#7cdce4',
          400: '#31c8d8',
          500: '#22afc2',
          600: '#137f90',
          700: '#116775',
          800: '#124f59',
          900: '#123e46',
          950: '#071f25'
        },
        // 辅助色 - 低饱和科技紫
        accent: {
          50: '#f6f3ff',
          100: '#ede8ff',
          200: '#ded4ff',
          300: '#c7b5ff',
          400: '#a78bfa',
          500: '#8b6fe8',
          600: '#7254c8',
          700: '#5b42a2',
          800: '#47347d',
          900: '#35285c',
          950: '#211838'
        },
        // 深色模式 - 深空石墨
        dark: {
          50: '#f5f7fa',
          100: '#e7ecf4',
          200: '#d2d9e4',
          300: '#b7c1cf',
          400: '#9aa6b5',
          500: '#6e7a8a',
          600: '#475363',
          700: '#2a3240',
          800: '#181d27',
          900: '#11151d',
          950: '#080a0f'
        }
      },
      fontFamily: {
        sans: [
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 8px 32px rgba(0, 0, 0, 0.08)',
        'glass-sm': '0 4px 16px rgba(0, 0, 0, 0.06)',
        glow: '0 0 20px rgba(49, 200, 216, 0.25)',
        'glow-lg': '0 0 40px rgba(49, 200, 216, 0.35)',
        card: '0 1px 3px rgba(0, 0, 0, 0.04), 0 1px 2px rgba(0, 0, 0, 0.06)',
        'card-hover': '0 10px 40px rgba(0, 0, 0, 0.08)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.1)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #31c8d8 0%, #137f90 100%)',
        'gradient-dark': 'linear-gradient(135deg, #181d27 0%, #080a0f 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.1) 0%, rgba(255,255,255,0.05) 100%)',
        'mesh-gradient':
          'radial-gradient(at 40% 20%, rgba(49, 200, 216, 0.12) 0px, transparent 50%), radial-gradient(at 80% 0%, rgba(167, 139, 250, 0.08) 0px, transparent 50%), radial-gradient(at 0% 50%, rgba(49, 200, 216, 0.08) 0px, transparent 50%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 0 20px rgba(49, 200, 216, 0.25)' },
          '100%': { boxShadow: '0 0 30px rgba(49, 200, 216, 0.4)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
