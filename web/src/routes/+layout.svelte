<script lang="ts">
	import '../app.css';
	import { page } from '$app/stores';
	import { afterNavigate, goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { listVehicles, getCurrentVehicle, setCurrentVehicle, type VehicleListItem } from '$lib/api';
	import { initAuth, mode, user, authReady } from '$lib/auth';
	import { loadSettings } from '$lib/settings';

	let vehicles: VehicleListItem[] = [];
	let currentVehicle: string = '';
	let showVehicleMenu = false;
	let mobileDrawerOpen = false;
	const authRoutes = new Set(['/login', '/forgot', '/reset']);

	// Auth pages render bare (no sidebar). In hosted mode an unauthenticated
	// visitor is bounced to login; single-user mode never shows any of this.
	$: isAuthRoute = authRoutes.has($page.url.pathname);
	$: needsLogin = $mode === 'hosted' && !$user;
	$: showChrome = $authReady && !isAuthRoute && !needsLogin;

	onMount(async () => {
		await initAuth();
		if (needsLogin && !isAuthRoute) {
			goto('/login');
			return;
		}
		if (!needsLogin) {
			loadSettings();
			await loadVehicles();
		}
	});

	async function loadVehicles() {
		try {
			vehicles = await listVehicles();
			const current = await getCurrentVehicle();
			// Ignore a stale current pointer (car since deleted) so the selector
			// never shows a vehicle that no longer exists.
			const valid = current.current && vehicles.some((v) => v.id === current.current);
			currentVehicle = valid ? current.current : (vehicles.length > 0 ? vehicles[0].id : '');
		} catch (e) {
			console.error('Failed to load vehicles:', e);
		}
	}

	async function switchVehicle(id: string) {
		try {
			await setCurrentVehicle(id);
			currentVehicle = id;
			showVehicleMenu = false;
			mobileDrawerOpen = false;
			// Trigger page reload to refresh data
			window.location.reload();
		} catch (e) {
			console.error('Failed to switch vehicle:', e);
		}
	}

	function closeMobileDrawer() {
		mobileDrawerOpen = false;
		showVehicleMenu = false;
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && (mobileDrawerOpen || showVehicleMenu)) closeMobileDrawer();
	}

	afterNavigate(() => {
		closeMobileDrawer();
	});

	$: currentVehicleData = vehicles.find(v => v.id === currentVehicle);
</script>

<svelte:window on:keydown={handleKeydown} />

{#if isAuthRoute}
	<slot />
{:else if !$authReady}
	<div class="min-h-screen flex items-center justify-center text-carbon-500">Loading…</div>
{:else if needsLogin}
	<div class="min-h-screen flex items-center justify-center text-carbon-500">Redirecting to sign in…</div>
{:else}
<div class="min-h-screen overflow-x-hidden md:flex">
	<!-- Mobile Header -->
	<header class="sticky top-0 z-30 flex h-16 items-center justify-between border-b border-carbon-800 bg-carbon-950/95 px-4 backdrop-blur md:hidden">
		<div class="flex min-w-0 items-center gap-3">
			<div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-accent-primary to-accent-secondary">
				<svg class="h-5 w-5 text-carbon-950" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
				</svg>
			</div>
			<div class="min-w-0">
				<p class="font-display text-base font-bold text-carbon-100">MileMinder</p>
				<p class="truncate text-xs text-carbon-500">{currentVehicleData?.vehicle || currentVehicle || 'Select vehicle'}</p>
			</div>
		</div>
		<button
			type="button"
			class="flex h-11 w-11 items-center justify-center rounded-xl border border-carbon-800 bg-carbon-900/70 text-carbon-200"
			aria-label="Open navigation"
			aria-expanded={mobileDrawerOpen}
			on:click={() => mobileDrawerOpen = true}
		>
			<svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 7h16M4 12h16M4 17h16" />
			</svg>
		</button>
	</header>

	{#if mobileDrawerOpen}
		<div class="fixed inset-0 z-40 md:hidden" role="dialog" aria-modal="true" aria-label="Navigation">
			<button
				type="button"
				class="absolute inset-0 h-full w-full bg-carbon-950/70"
				aria-label="Close navigation"
				on:click={closeMobileDrawer}
			></button>
			<aside class="absolute inset-y-0 left-0 flex w-[min(20rem,calc(100vw-2rem))] flex-col overflow-y-auto border-r border-carbon-800 bg-carbon-950 shadow-2xl">
				<div class="flex items-center justify-between border-b border-carbon-800 p-4">
					<div class="flex items-center gap-3">
						<div class="flex h-10 w-10 items-center justify-center rounded-xl bg-gradient-to-br from-accent-primary to-accent-secondary">
							<svg class="h-6 w-6 text-carbon-950" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
							</svg>
						</div>
						<div>
							<h1 class="font-display text-lg font-bold text-carbon-100">MileMinder</h1>
							<p class="text-xs text-carbon-500">Track your journey</p>
						</div>
					</div>
					<button
						type="button"
						class="flex h-11 w-11 items-center justify-center rounded-xl text-carbon-400 hover:bg-carbon-800/50 hover:text-carbon-100"
						aria-label="Close navigation"
						on:click={closeMobileDrawer}
					>
						<svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
						</svg>
					</button>
				</div>

				<div class="border-b border-carbon-800 p-4">
					<button
						class="flex w-full items-center justify-between rounded-xl bg-carbon-800/50 p-3 transition-colors hover:bg-carbon-800"
						on:click={() => showVehicleMenu = !showVehicleMenu}
					>
						<div class="flex min-w-0 items-center gap-3">
							<div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-accent-primary/20">
								<svg class="h-4 w-4 text-accent-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
								</svg>
							</div>
							<div class="min-w-0 text-left">
								<p class="truncate text-sm font-medium text-carbon-100">{currentVehicleData?.vehicle || currentVehicle || 'Select vehicle'}</p>
								<p class="truncate text-xs text-carbon-500">{currentVehicle}</p>
							</div>
						</div>
						<svg class="h-4 w-4 shrink-0 text-carbon-400 transition-transform" class:rotate-180={showVehicleMenu} fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
						</svg>
					</button>

					{#if showVehicleMenu && vehicles.length > 0}
						<div class="mt-2 rounded-xl border border-carbon-700 bg-carbon-800 py-2 animate-fade-in">
							{#each vehicles as vehicle}
								<button
									class="flex w-full items-center justify-between px-4 py-3 text-left text-sm transition-colors hover:bg-carbon-700/50"
									class:text-accent-primary={vehicle.id === currentVehicle}
									on:click|stopPropagation={() => switchVehicle(vehicle.id)}
								>
									<span class="min-w-0 truncate">{vehicle.vehicle || vehicle.id}</span>
									{#if vehicle.id === currentVehicle}
										<svg class="h-4 w-4 shrink-0" fill="currentColor" viewBox="0 0 24 24">
											<path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/>
										</svg>
									{/if}
								</button>
							{/each}
						</div>
					{/if}
				</div>

				<nav class="flex-1 space-y-1 p-4">
					<a href="/" class="nav-link" class:active={$page.url.pathname === '/'} on:click={closeMobileDrawer}>
						<svg class="h-5 w-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
						</svg>
						<span>Dashboard</span>
					</a>
					<a href="/add" class="nav-link" class:active={$page.url.pathname === '/add'} on:click={closeMobileDrawer}>
						<svg class="h-5 w-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
						</svg>
						<span>Add Mileage</span>
					</a>
					<a href="/graph" class="nav-link" class:active={$page.url.pathname === '/graph'} on:click={closeMobileDrawer}>
						<svg class="h-5 w-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 12l3-3 3 3 4-4M8 21l4-4 4 4M3 4h18M4 4h16v12a1 1 0 01-1 1H5a1 1 0 01-1-1V4z" />
						</svg>
						<span>Graph</span>
					</a>
					<a href="/fleet" class="nav-link" class:active={$page.url.pathname === '/fleet'} on:click={closeMobileDrawer}>
						<svg class="h-5 w-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
						</svg>
						<span>Fleet</span>
					</a>
					<a href="/history" class="nav-link" class:active={$page.url.pathname === '/history'} on:click={closeMobileDrawer}>
						<svg class="h-5 w-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
						</svg>
						<span>History</span>
					</a>
				</nav>

				<div class="border-t border-carbon-800 p-4">
					<a href="/settings" class="nav-link" class:active={$page.url.pathname === '/settings'} on:click={closeMobileDrawer}>
						<svg class="h-5 w-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
						</svg>
						<span>Settings</span>
					</a>
				</div>
			</aside>
		</div>
	{/if}

	<!-- Sidebar -->
	<aside class="hidden w-64 shrink-0 bg-carbon-900/40 border-r border-carbon-800 md:flex md:flex-col">
		<!-- Logo -->
		<div class="p-6 border-b border-carbon-800">
			<div class="flex items-center gap-3">
				<div class="w-10 h-10 rounded-xl bg-gradient-to-br from-accent-primary to-accent-secondary flex items-center justify-center">
					<svg class="w-6 h-6 text-carbon-950" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
					</svg>
				</div>
				<div>
					<h1 class="font-display font-bold text-lg text-carbon-100">MileMinder</h1>
					<p class="text-xs text-carbon-500">Track your journey</p>
				</div>
			</div>
		</div>

		<!-- Vehicle Selector -->
		<div class="p-4 border-b border-carbon-800">
			<button 
				class="w-full flex items-center justify-between p-3 rounded-xl bg-carbon-800/50 hover:bg-carbon-800 transition-colors"
				on:click={() => showVehicleMenu = !showVehicleMenu}
			>
				<div class="flex items-center gap-3">
					<div class="w-8 h-8 rounded-lg bg-accent-primary/20 flex items-center justify-center">
						<svg class="w-4 h-4 text-accent-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
						</svg>
					</div>
					<div class="text-left">
						<p class="text-sm font-medium text-carbon-100">{currentVehicleData?.vehicle || currentVehicle || 'Select vehicle'}</p>
						<p class="text-xs text-carbon-500">{currentVehicle}</p>
					</div>
				</div>
				<svg class="w-4 h-4 text-carbon-400 transition-transform" class:rotate-180={showVehicleMenu} fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
				</svg>
			</button>

			{#if showVehicleMenu && vehicles.length > 0}
				<div class="mt-2 py-2 bg-carbon-800 rounded-xl border border-carbon-700 animate-fade-in">
					{#each vehicles as vehicle}
						<button
							class="w-full px-4 py-2 text-left text-sm hover:bg-carbon-700/50 transition-colors flex items-center justify-between"
							class:text-accent-primary={vehicle.id === currentVehicle}
							on:click|stopPropagation={() => switchVehicle(vehicle.id)}
						>
							<span>{vehicle.vehicle || vehicle.id}</span>
							{#if vehicle.id === currentVehicle}
								<svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
									<path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/>
								</svg>
							{/if}
						</button>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Navigation -->
		<nav class="flex-1 p-4 space-y-1">
			<a href="/" class="nav-link" class:active={$page.url.pathname === '/'}>
				<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
				</svg>
				<span>Dashboard</span>
			</a>

			<a href="/add" class="nav-link" class:active={$page.url.pathname === '/add'}>
				<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
				</svg>
				<span>Add Mileage</span>
			</a>

			<a href="/graph" class="nav-link" class:active={$page.url.pathname === '/graph'}>
				<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 12l3-3 3 3 4-4M8 21l4-4 4 4M3 4h18M4 4h16v12a1 1 0 01-1 1H5a1 1 0 01-1-1V4z" />
				</svg>
				<span>Graph</span>
			</a>

			<a href="/fleet" class="nav-link" class:active={$page.url.pathname === '/fleet'}>
				<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
				</svg>
				<span>Fleet</span>
			</a>

			<a href="/history" class="nav-link" class:active={$page.url.pathname === '/history'}>
				<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
				<span>History</span>
			</a>
		</nav>

		<!-- Footer -->
		<div class="p-4 border-t border-carbon-800">
			<a href="/settings" class="nav-link" class:active={$page.url.pathname === '/settings'}>
				<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
				</svg>
				<span>Settings</span>
			</a>
		</div>
	</aside>

	<!-- Main Content -->
	<main class="min-w-0 flex-1 overflow-x-hidden md:overflow-auto">
		<slot />
	</main>
</div>
{/if}
