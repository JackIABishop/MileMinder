<script lang="ts">
	import '../app.css';
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { listVehicles, getCurrentVehicle, setCurrentVehicle, type VehicleListItem } from '$lib/api';

	let vehicles: VehicleListItem[] = [];
	let currentVehicle: string = '';
	let showVehicleMenu = false;

	onMount(async () => {
		await loadVehicles();
	});

	async function loadVehicles() {
		try {
			vehicles = await listVehicles();
			const current = await getCurrentVehicle();
			currentVehicle = current.current || (vehicles.length > 0 ? vehicles[0].id : '');
		} catch (e) {
			console.error('Failed to load vehicles:', e);
		}
	}

	async function switchVehicle(id: string) {
		try {
			await setCurrentVehicle(id);
			currentVehicle = id;
			showVehicleMenu = false;
			// Trigger page reload to refresh data
			window.location.reload();
		} catch (e) {
			console.error('Failed to switch vehicle:', e);
		}
	}

	$: currentVehicleData = vehicles.find(v => v.id === currentVehicle);
</script>

<div class="min-h-screen flex">
	<!-- Sidebar -->
	<aside class="w-64 bg-carbon-900/40 border-r border-carbon-800 flex flex-col">
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
	<main class="flex-1 overflow-auto">
		<slot />
	</main>
</div>
