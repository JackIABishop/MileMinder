import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import { SvelteKitPWA } from '@vite-pwa/sveltekit';

export default defineConfig({
	plugins: [
		sveltekit(),
		SvelteKitPWA({
			// generateSW precaches the built app shell (JS/CSS/HTML/icons) so repeat
			// loads are instant. No runtimeCaching is configured: API responses are
			// never cached, since mileage data must always be fetched fresh.
			strategies: 'generateSW',
			registerType: 'autoUpdate',
			injectRegister: 'auto',
			includeAssets: ['favicon.svg', 'apple-touch-icon.png'],
			kit: {
				// Matches svelte.config.js's adapter-static SPA fallback.
				adapterFallback: 'index.html',
				spa: true
			},
			manifest: {
				name: 'MileMinder',
				short_name: 'MileMinder',
				description: 'Track vehicle mileage against your PCP/lease/insurance allowance',
				start_url: '/',
				scope: '/',
				display: 'standalone',
				background_color: '#0d0d10',
				theme_color: '#0d0d10',
				icons: [
					{ src: 'icon-192.png', sizes: '192x192', type: 'image/png' },
					{ src: 'icon-512.png', sizes: '512x512', type: 'image/png' },
					{ src: 'icon-maskable-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' }
				],
				// Android/Chrome long-press shortcut straight to the quick-add form.
				// iOS has no equivalent — manifest shortcuts aren't supported there.
				shortcuts: [
					{
						name: 'Add reading',
						short_name: 'Add reading',
						url: '/quick-add',
						icons: [{ src: 'icon-192.png', sizes: '192x192', type: 'image/png' }]
					}
				]
			},
			workbox: {
				// Precache the app shell only; API calls under /api/v1 are always
				// network-fresh (no offline reading queue, no background sync).
				navigateFallbackDenylist: [/^\/api\//]
			},
			devOptions: {
				enabled: false
			}
		})
	],
	server: {
		proxy: {
			'/api': {
				target: 'http://localhost:8080',
				changeOrigin: true
			}
		}
	}
});
