<script lang="ts">
	import { onMount } from 'svelte';
	import { getFleet, setCurrentVehicle, formatNumber, formatDate, type VehicleStatus, type FleetInsights } from '$lib/api';
	import Gauge from '$lib/components/Gauge.svelte';

	let fleet: VehicleStatus[] = [];
	let insights: FleetInsights | null = null;
	let loading = true;
	let error = '';
	let settingDefault = '';

	onMount(async () => {
		await loadFleet();
	});

	async function loadFleet() {
		loading = true;
		error = '';
		try {
			const resp = await getFleet();
			// Policy cars are ranked by allowance use; plain cars follow by recent pace.
			fleet = [...resp.vehicles].sort((a, b) => {
				if (a.has_plan && b.has_plan) return b.percent_used - a.percent_used;
				if (a.has_plan !== b.has_plan) return a.has_plan ? -1 : 1;
				return b.recent_annual_mileage - a.recent_annual_mileage;
			});
			insights = resp.insights;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load fleet';
		} finally {
			loading = false;
		}
	}

	// "View" — make this the active vehicle and open the dashboard.
	async function viewVehicle(id: string) {
		try {
			await setCurrentVehicle(id);
			window.location.href = '/';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to switch vehicle';
		}
	}

	// "Set as default" — set the default in place without leaving the fleet view.
	async function setDefault(id: string) {
		settingDefault = id;
		try {
			await setCurrentVehicle(id);
			await loadFleet();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to set default';
		} finally {
			settingDefault = '';
		}
	}

	function getTermString(status: VehicleStatus): string {
		return `${status.years_left_term}y ${status.days_left_term}d`;
	}

	// Resolve the worst-offending vehicle (highest percent_used) for the callout.
	$: worstVehicle = (() => {
		const ins = insights;
		if (!ins?.worst_offender_id) return undefined;
		return fleet.find((v) => v.id === ins.worst_offender_id);
	})();
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
		<!-- Household roll-up -->
		{#if insights}
			<div class="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
				<div class="card animate-slide-up">
					<p class="text-sm text-carbon-400 mb-1">Household over/under</p>
					<p class="text-3xl font-mono font-bold {insights.net_delta > 0 ? 'text-gauge-red' : 'text-gauge-green'}">
						{insights.net_delta > 0 ? '+' : ''}{formatNumber(Math.round(insights.net_delta))}<span class="text-base text-carbon-500"> mi</span>
					</p>
					<p class="text-xs text-carbon-500 mt-1">
						{insights.count_over} over · {insights.count_under} under · policy cars only
					</p>
				</div>
				<div class="card animate-slide-up stagger-1">
					<p class="text-sm text-carbon-400 mb-1">Total annual mileage</p>
					<p class="text-3xl font-mono font-bold text-carbon-100">
						{formatNumber(Math.round(insights.total_avg_annual_mileage))}<span class="text-base text-carbon-500"> mi/yr</span>
					</p>
					<p class="text-xs text-carbon-500 mt-1">Lifetime average, all cars</p>
					<p class="text-xs text-carbon-600 mt-1">{insights.policy_vehicles} policy · {insights.plain_vehicles} tracking only</p>
				</div>
				<div class="card animate-slide-up stagger-2">
					<p class="text-sm text-carbon-400 mb-1">Fleet avg pace</p>
					<p class="text-3xl font-mono font-bold text-carbon-100">
						{Math.round(insights.avg_percent_used)}<span class="text-base text-carbon-500">%</span>
					</p>
					<p class="text-xs text-carbon-500 mt-1">Mean allowance used, policy cars only</p>
				</div>
				<div class="card animate-slide-up stagger-3 {insights.net_delta > 0 ? 'border-gauge-red/30' : ''}">
					<p class="text-sm text-carbon-400 mb-1">Worst offender</p>
					{#if insights.worst_offender_id}
						<p class="text-xl font-semibold text-carbon-100 truncate">
							{insights.worst_offender_vehicle || insights.worst_offender_id}
						</p>
						{#if worstVehicle}
							<p class="text-xs {worstVehicle.percent_used > 100 ? 'text-gauge-red' : 'text-carbon-500'} mt-1">
								{Math.round(worstVehicle.percent_used)}% of allowance used
							</p>
						{/if}
					{:else}
						<p class="text-xl font-semibold text-carbon-300">—</p>
					{/if}
				</div>
			</div>
		{/if}

		<!-- Vehicle Cards (ordered worst-pace first) -->
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
			{#each fleet as vehicle, i (vehicle.id)}
				<div
					class="card-hover relative animate-slide-up {insights && vehicle.id === insights.worst_offender_id ? 'border-gauge-red/40' : ''}"
					style="animation-delay: {0.05 * (i + 1)}s"
				>
					{#if vehicle.is_default}
						<div class="absolute top-4 right-4">
							<span class="px-2 py-1 text-xs font-medium bg-accent-primary/20 text-accent-primary rounded-full">
								Active
							</span>
						</div>
					{/if}

					<div class="flex items-start gap-4">
						{#if vehicle.has_plan}
							<Gauge percent={vehicle.percent_used} size={80} strokeWidth={6}>
								<span class="text-sm font-mono font-bold">{Math.round(vehicle.percent_used)}%</span>
							</Gauge>
						{:else}
							<div class="w-20 h-20 rounded-lg bg-carbon-800/70 flex items-center justify-center text-center px-2">
								<span class="text-xs font-medium text-carbon-300">Tracking only</span>
							</div>
						{/if}

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
								{#if vehicle.has_plan}
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
								{:else}
									<div class="flex justify-between text-sm">
										<span class="text-carbon-400">Avg pace</span>
										<span class="font-mono text-carbon-100">{formatNumber(Math.round(vehicle.avg_annual_mileage))} mi/yr</span>
									</div>
									<div class="flex justify-between text-sm">
										<span class="text-carbon-400">Recent pace</span>
										<span class="font-mono text-carbon-100">{formatNumber(Math.round(vehicle.recent_annual_mileage))} mi/yr</span>
									</div>
								{/if}
							</div>
						</div>
					</div>

					<!-- Comparative pace vs the fleet average -->
					{#if insights && vehicle.has_plan}
						<div class="mt-4">
							<div class="flex justify-between text-xs text-carbon-500 mb-1">
								<span>Pace vs fleet avg</span>
								<span class="font-mono {vehicle.percent_used > insights.avg_percent_used ? 'text-gauge-amber' : 'text-gauge-green'}">
									{vehicle.percent_used >= insights.avg_percent_used ? '+' : ''}{Math.round(vehicle.percent_used - insights.avg_percent_used)}%
								</span>
							</div>
							<div class="relative h-2 bg-carbon-800 rounded-full overflow-hidden">
								<div
									class="h-full rounded-full {vehicle.percent_used > 100 ? 'bg-gauge-red' : vehicle.percent_used > insights.avg_percent_used ? 'bg-gauge-amber' : 'bg-gauge-green'}"
									style="width: {Math.min(100, vehicle.percent_used)}%"
								></div>
								<!-- fleet-average marker -->
								<div
									class="absolute top-0 h-2 w-0.5 bg-carbon-300"
									style="left: {Math.min(100, insights.avg_percent_used)}%"
									title="Fleet average"
								></div>
							</div>
						</div>
					{/if}

					<div class="mt-4 pt-4 border-t border-carbon-800 flex items-center justify-between gap-3">
						<button class="btn-primary text-sm" on:click={() => viewVehicle(vehicle.id)}>
							View
						</button>
						{#if vehicle.is_default}
							<span class="text-xs text-carbon-500">Default vehicle</span>
						{:else}
							<button
								class="text-sm text-accent-primary hover:underline disabled:opacity-50"
								on:click={() => setDefault(vehicle.id)}
								disabled={settingDefault === vehicle.id}
							>
								{settingDefault === vehicle.id ? 'Setting…' : 'Set as default'}
							</button>
						{/if}
					</div>

					<div class="mt-3 pt-3 border-t border-carbon-800">
						<div class="flex justify-between text-xs text-carbon-500">
							{#if vehicle.has_plan}
								<span>Plan: {formatDate(vehicle.plan_start)} → {formatDate(vehicle.plan_end)}</span>
								<span>{formatNumber(vehicle.annual_allowance)} mi/yr</span>
							{:else}
								<span>No allowance policy</span>
								<span>{vehicle.pace_trend}</span>
							{/if}
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
