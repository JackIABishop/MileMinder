/** @type {import('tailwindcss').Config} */
export default {
	content: ['./src/**/*.{html,js,svelte,ts}'],
	theme: {
		extend: {
			colors: {
				// Automotive-inspired dark theme
				carbon: {
					50: '#f7f7f8',
					100: '#eeeef0',
					200: '#d9d9de',
					300: '#b8b8c1',
					400: '#91919f',
					500: '#747484',
					600: '#5e5e6c',
					700: '#4d4d58',
					800: '#42424b',
					900: '#1a1a1f',
					950: '#0d0d10'
				},
				gauge: {
					green: '#22c55e',
					amber: '#f59e0b',
					red: '#ef4444',
					blue: '#3b82f6'
				},
				accent: {
					primary: '#60a5fa',
					secondary: '#a78bfa',
					glow: '#818cf8'
				}
			},
			fontFamily: {
				sans: ['Outfit', 'system-ui', 'sans-serif'],
				mono: ['JetBrains Mono', 'Menlo', 'monospace'],
				display: ['Sora', 'system-ui', 'sans-serif']
			},
			animation: {
				'fade-in': 'fadeIn 0.5s ease-out',
				'slide-up': 'slideUp 0.5s ease-out',
				'count-up': 'countUp 1s ease-out',
				'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
				'gauge-fill': 'gaugeFill 1.5s ease-out forwards'
			},
			keyframes: {
				fadeIn: {
					'0%': { opacity: '0' },
					'100%': { opacity: '1' }
				},
				slideUp: {
					'0%': { opacity: '0', transform: 'translateY(20px)' },
					'100%': { opacity: '1', transform: 'translateY(0)' }
				},
				countUp: {
					'0%': { opacity: '0', transform: 'translateY(10px)' },
					'100%': { opacity: '1', transform: 'translateY(0)' }
				},
				gaugeFill: {
					'0%': { strokeDashoffset: '283' },
					'100%': { strokeDashoffset: 'var(--gauge-offset)' }
				}
			},
			boxShadow: {
				'glow': '0 0 20px rgba(96, 165, 250, 0.3)',
				'glow-green': '0 0 20px rgba(34, 197, 94, 0.4)',
				'glow-amber': '0 0 20px rgba(245, 158, 11, 0.4)',
				'glow-red': '0 0 20px rgba(239, 68, 68, 0.4)'
			}
		}
	},
	plugins: []
};
