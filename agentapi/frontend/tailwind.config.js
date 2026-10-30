/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // 兼容既有 gray/slate/zinc 类名：统一映射为暖白、香槟与暖黑
        gray: {
          50: '#fbfaf7',
          100: '#f5f2eb',
          200: '#e8e2d6',
          300: '#d3c9b8',
          400: '#a99d8a',
          500: '#817665',
          600: '#635a4c',
          700: '#4a4338',
          800: '#302b24',
          900: '#1e1a15',
          950: '#100e0b'
        },
        slate: {
          50: '#fbfaf7',
          100: '#f5f2eb',
          200: '#e8e2d6',
          300: '#d3c9b8',
          400: '#a99d8a',
          500: '#817665',
          600: '#635a4c',
          700: '#4a4338',
          800: '#302b24',
          900: '#1e1a15',
          950: '#100e0b'
        },
        zinc: {
          50: '#fbfaf7',
          100: '#f5f2eb',
          200: '#e8e2d6',
          300: '#d3c9b8',
          400: '#a99d8a',
          500: '#817665',
          600: '#635a4c',
          700: '#4a4338',
          800: '#302b24',
          900: '#1e1a15',
          950: '#100e0b'
        },
        // 品牌主色 - 白金主题的金色
        primary: {
          50: '#fdfbf3',
          100: '#faf3dd',
          200: '#f3e4b6',
          300: '#ead18a',
          400: '#ddb85c',
          500: '#b38a2b',
          600: '#8c681f',
          700: '#72541f',
          800: '#5f471e',
          900: '#4a3716',
          950: '#2d200b'
        },
        // 中性色 - 暖白、米色与香槟灰
        accent: {
          50: '#fbfaf7',
          100: '#f6f3ec',
          200: '#ebe5d9',
          300: '#dad1bf',
          400: '#b9ad97',
          500: '#938772',
          600: '#746a58',
          700: '#5b5346',
          800: '#3c362e',
          900: '#27231d',
          950: '#151310'
        },
        // 深色模式 - 暖黑而非冷蓝黑
        dark: {
          50: '#fbfaf7',
          100: '#f4f1e9',
          200: '#e5dfd2',
          300: '#c9c0ae',
          400: '#a19684',
          500: '#7d7261',
          600: '#5a5144',
          700: '#3d372e',
          800: '#26221c',
          900: '#191612',
          950: '#0e0c09'
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
        glass: '0 8px 32px rgba(58, 46, 22, 0.08)',
        'glass-sm': '0 4px 16px rgba(58, 46, 22, 0.06)',
        glow: '0 0 20px rgba(201, 159, 55, 0.22)',
        'glow-lg': '0 0 40px rgba(201, 159, 55, 0.32)',
        card: '0 1px 2px rgba(58, 46, 22, 0.04), 0 8px 24px -12px rgba(58, 46, 22, 0.12)',
        'card-hover': '0 16px 40px -16px rgba(58, 46, 22, 0.22)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.6)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #e2c273 0%, #c99f37 52%, #a9822a 100%)',
        'gradient-gold-soft': 'linear-gradient(135deg, rgba(234, 209, 138, 0.28) 0%, rgba(201, 159, 55, 0.12) 100%)',
        'gradient-dark': 'linear-gradient(135deg, #26221c 0%, #0e0c09 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.6) 0%, rgba(255,255,255,0.2) 100%)',
        'mesh-gradient':
          'radial-gradient(at 18% 0%, rgba(226, 194, 115, 0.18) 0px, transparent 55%), radial-gradient(at 82% 8%, rgba(201, 159, 55, 0.12) 0px, transparent 50%), radial-gradient(at 50% 100%, rgba(234, 209, 138, 0.1) 0px, transparent 55%)'
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
          '0%': { boxShadow: '0 0 20px rgba(201, 159, 55, 0.22)' },
          '100%': { boxShadow: '0 0 32px rgba(201, 159, 55, 0.4)' }
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
