<script lang="ts">
	import { onMount } from 'svelte';
	import { getFleet, setCurrentVehicle, formatNumber, formatDate, getStatusColor, type VehicleStatus } from '$lib/api';
	import Gauge from '$lib/components/Gauge.svelte';

	let fleet: VehicleStatus[] = [];
	let loading = true;
	let error = '';

	onMount(async () => {
		await loadFleet();
	});

	async function loadFleet() {
		loading = true;
		error = '';
		try {
			fleet = await getFleet();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load fleet';
		} finally {
			loading = false;
		}
	}

	async function selectVehicle(id: string) {
		try {
			await setCurrentVehicle(id);
			window.location.href = '/';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to switch vehicle';
		}
	}

	function getTermString(status: VehicleStatus): string {
		return `${status.years_left_term}y ${status.days_left_term}d`;
	}
</script>

<svelte:head>
	<title>Fleet | MileMinder</title>
</svelte:head>

<div class="p-8">
	<header class="mb-8 animate-fade-in">
		<h1 class="text-3xl font-display font-bold text-carbon-100">Fleet Overview</h1>
		<p class="text-carbon-500 mt-2">Monitor all your vehicles at a glance</p>
	</header>

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="animate-pulse text-carbon-400">Loading...</div>
		</div>
	{:else if error}
		<div class="card border-gauge-red/30 bg-gauge-red/5">
			<p class="text-gauge-red">{error}</p>
		</div>
	{:else if fleet.length === 0}
		<div class="card text-center py-12">
			<svg class="w-16 h-16 mx-auto text-carbon-600 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
			</svg>
			<h2 class="text-xl font-semibold text-carbon-300 mb-2">No Vehicles Yet</h2>
			<p class="text-carbon-500 mb-6">Add your first vehicle to start tracking mileage</p>
			<a href="/settings" class="btn-primary">Add Vehicle</a>
		</div>
	{:else}
		<!-- Summary Stats -->
		<div class="grid grid-cols-3 gap-4 mb-8">
			<div class="card animate-slide-up">
				<p class="text-sm text-carbon-400 mb-1">Total Vehicles</p>
				<p class="text-3xl font-mono font-bold text-carbon-100">{fleet.length}</p>
			</div>
			<div class="card animate-slide-up stagger-1">
				<p class="text-sm text-carbon-400 mb-1">Under Budget</p>
				<p class="text-3xl font-mono font-bold text-gauge-green">
					{fleet.filter(v => v.delta <= 0).length}
				</p>
			</div>
			<div class="card animate-slide-up stagger-2">
				<p class="text-sm text-carbon-400 mb-1">Over Budget</p>
				<p class="text-3xl font-mono font-bold text-gauge-red">
					{fleet.filter(v => v.delta > 0).length}
				</p>
			</div>
		</div>

		<!-- Vehicle Cards -->
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
			{#each fleet as vehicle, i}
				<button
					class="card-hover text-left animate-slide-up relative"
					style="animation-delay: {0.1 * (i + 3)}s"
					on:click={() => selectVehicle(vehicle.id)}
				>
					{#if vehicle.is_default}
						<div class="absolute top-4 right-4">
							<span class="px-2 py-1 text-xs font-medium bg-accent-primary/20 text-accent-primary rounded-full">
								Active
							</span>
						</div>
					{/if}

					<div class="flex items-start gap-4">
						<Gauge percent={vehicle.percent_used} size={80} strokeWidth={6}>
							<span class="text-sm font-mono font-bold">{Math.round(vehicle.percent_used)}%</span>
						</Gauge>
						
						<div class="flex-1 min-w-0">
							<h3 class="font-semibold text-lg text-carbon-100 truncate">
								{vehicle.vehicle || vehicle.id}
							</h3>
							<p class="text-sm text-carbon-500">{vehicle.id}</p>
							
							<div class="mt-3 space-y-1">
								<div class="flex justify-between text-sm">
									<span class="text-carbon-400">Odometer</span>
									<span class="font-mono text-carbon-100">{formatNumber(vehicle.latest_reading)} mi</span>
								</div>
								<div class="flex justify-between text-sm">
									<span class="text-carbon-400">Delta</span>
									<span class="font-mono {vehicle.delta <= 0 ? 'text-gauge-green' : 'text-gauge-red'}">
										{vehicle.delta > 0 ? '+' : ''}{formatNumber(Math.round(vehicle.delta))} mi
									</span>
								</div>
								<div class="flex justify-between text-sm">
									<span class="text-carbon-400">Term left</span>
									<span class="font-mono text-carbon-100">{getTermString(vehicle)}</span>
								</div>
							</div>
						</div>
					</div>

					<div class="mt-4 pt-4 border-t border-carbon-800">
						<div class="flex justify-between text-xs text-carbon-500">
							<span>Plan: {formatDate(vehicle.plan_start)} → {formatDate(vehicle.plan_end)}</span>
							<span>{formatNumber(vehicle.annual_allowance)} mi/yr</span>
						</div>
					</div>
				</button>
			{/each}
		</div>
	{/if}
</div>
