<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { getCurrentVehicle, getVehicle, listVehicles, addReading, formatNumber, formatDate, getDeltaStatus, type VehicleStatus } from '$lib/api';
	import { formatMoneyMinor } from '$lib/money';
	import { settings } from '$lib/settings';
	import Gauge from '$lib/components/Gauge.svelte';
	import StatCard from '$lib/components/StatCard.svelte';
	import QuickAdd from '$lib/components/QuickAdd.svelte';

	let status: VehicleStatus | null = null;
	let loading = true;
	let error = '';
	let quickAddLoading = false;

	onMount(async () => {
		await loadStatus();
	});

	async function loadStatus() {
		loading = true;
		error = '';
		try {
			const [current, vehicles] = await Promise.all([getCurrentVehicle(), listVehicles()]);
			// A current pointer is only valid if it still resolves to a vehicle; a
			// stale pointer (car since deleted) is treated as no default.
			const hasDefault = !!current.current && vehicles.some((v) => v.id === current.current);

			if (hasDefault) {
				status = await getVehicle(current.current);
			} else if (vehicles.length === 0) {
				// No vehicles at all → onboarding/empty state.
				error = 'No vehicle selected. Please add a vehicle first.';
			} else if (vehicles.length === 1) {
				// Exactly one car → just open it (no point routing to a one-row fleet).
				status = await getVehicle(vehicles[0].id);
			} else {
				// Multiple cars and no default → land on the fleet overview.
				await goto('/fleet');
				return;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load status';
		} finally {
			loading = false;
		}
	}

	async function handleQuickAdd(event: CustomEvent<{ miles: number }>) {
		if (!status) return;
		quickAddLoading = true;
		try {
			await addReading(status.id, { miles: event.detail.miles });
			await loadStatus();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to add reading';
		} finally {
			quickAddLoading = false;
		}
	}

	$: deltaStatus = status ? getDeltaStatus(status.delta) : null;
	$: today = new Date().toLocaleDateString('en-GB', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' });

	// Trend signal (#7): recent 90-day pace vs lifetime average.
	const trendBadges: Record<string, { icon: string; label: string; color: string }> = {
		accelerating: { icon: '↑', label: 'Accelerating', color: 'text-gauge-amber' },
		easing: { icon: '↓', label: 'Easing off', color: 'text-gauge-green' },
		steady: { icon: '→', label: 'Steady', color: 'text-carbon-400' }
	};
	$: trendBadge = status ? (trendBadges[status.pace_trend] ?? trendBadges.steady) : null;
</script>

<svelte:head>
	<title>Dashboard | MileMinder</title>
</svelte:head>

<div class="p-4 sm:p-6 lg:p-8">
	<!-- Header -->
	<header class="mb-8 animate-fade-in">
		<p class="text-carbon-500 text-sm mb-1">{today}</p>
		<h1 class="text-3xl font-display font-bold text-carbon-100">
			{#if status}
				{status.vehicle || status.id}
			{:else}
				Dashboard
			{/if}
		</h1>
		{#if status?.registration}
			<p class="text-carbon-500 text-sm mt-1">{status.registration}</p>
		{/if}
	</header>

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="animate-pulse text-carbon-400">Loading...</div>
		</div>
	{:else if error}
		<div class="card border-gauge-red/30 bg-gauge-red/5">
			<p class="text-gauge-red">{error}</p>
			<a href="/settings" class="btn-primary mt-4 inline-block">Add a Vehicle</a>
		</div>
	{:else if status}
		{#if status.has_plan}
		<!-- Main Gauge -->
		<div class="grid grid-cols-1 lg:grid-cols-3 gap-4 sm:gap-6 mb-8">
			<div class="lg:col-span-1 card flex flex-col items-center justify-center py-8 animate-slide-up">
				<Gauge percent={status.percent_used} size={220} strokeWidth={14}>
					<span class="text-4xl sm:text-5xl font-mono font-bold text-carbon-100">
						{Math.round(status.percent_used)}
					</span>
					<span class="text-lg text-carbon-400">% used</span>
				</Gauge>
				
				<div class="mt-6 text-center">
					<div class="flex items-center justify-center gap-2 {deltaStatus?.color}">
						<span class="text-2xl">{deltaStatus?.icon}</span>
						<span class="font-mono text-xl font-semibold">
							{status.delta > 0 ? '+' : ''}{formatNumber(Math.round(status.delta))} mi
						</span>
					</div>
					<p class="text-sm text-carbon-500 mt-1">{deltaStatus?.label}</p>
				</div>
			</div>

			<!-- Stats Grid -->
			<div class="lg:col-span-2 grid grid-cols-1 sm:grid-cols-2 gap-4">
				<StatCard 
					title="Current Odometer" 
					value={formatNumber(status.latest_reading)} 
					unit="mi"
					subtitle="Last updated {formatDate(status.latest_date)}"
				/>
				
				<StatCard 
					title="Allowance Used" 
					value={formatNumber(Math.round(status.target_today - status.start_miles))} 
					unit="mi"
					subtitle="Budget to date"
				/>
				
				<StatCard
					title="Daily Rate"
					value={formatNumber(status.daily_rate, 1)}
					unit="mi/day"
					subtitle="Current pace"
					tooltip="Average mi/day since this allowance year started. Used to project your year-end total below."
					color={status.daily_rate > (status.annual_allowance / 365) ? 'amber' : 'green'}
				/>
				
				<StatCard
					title="Annual Allowance"
					value={formatNumber(status.annual_allowance)}
					unit="mi"
					subtitle="{formatNumber(Math.round(status.annual_allowance / 365))} mi/day ideal"
				/>

				<!-- Drivable-rate budget (#4): safe mi/day for the rest of the plan -->
				<StatCard
					title="Safe Daily Rate"
					value={formatNumber(status.drivable_daily_rate, 1)}
					unit="mi/day"
					subtitle="To stay legal for the rest of the plan"
					color={status.daily_rate > status.drivable_daily_rate ? 'amber' : 'green'}
				/>

				<!-- Average annual mileage: lifetime average (for insurance) + recent pace -->
				<div class="sm:col-span-2 card animate-slide-up">
					<h3 class="text-sm font-medium text-carbon-400 mb-3">Average Annual Mileage</h3>
					<div class="flex flex-col items-start justify-between gap-4 sm:flex-row sm:items-end">
						<div>
							<div class="flex items-baseline gap-1">
								<span class="number-display text-carbon-100">{formatNumber(Math.round(status.avg_annual_mileage))}</span>
								<span class="number-unit">mi/yr</span>
							</div>
							<p class="mt-1 text-xs text-carbon-500">Lifetime average — quote this for insurance</p>
						</div>
						<div class="text-left sm:text-right">
							<div class="flex items-baseline gap-1 sm:justify-end">
								<span class="font-mono text-xl font-semibold text-carbon-300">{formatNumber(Math.round(status.recent_annual_mileage))}</span>
								<span class="text-sm text-carbon-500">mi/yr</span>
							</div>
							<div class="mt-1 flex flex-wrap items-center gap-2 sm:justify-end">
								<p class="text-xs text-carbon-500">Last 90 days</p>
								{#if trendBadge}
									<span class="flex items-center gap-1 text-xs font-medium {trendBadge.color}">
										<span>{trendBadge.icon}</span>
										<span>{trendBadge.label}</span>
									</span>
								{/if}
							</div>
						</div>
					</div>
				</div>
			</div>
		</div>

		<!-- Time Left Section -->
		<div class="grid grid-cols-1 md:grid-cols-2 gap-6 mb-8">
			<div class="card animate-slide-up stagger-2">
				<h3 class="text-lg font-semibold text-carbon-100 mb-4 flex items-center gap-2">
					<svg class="w-5 h-5 text-accent-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
					</svg>
					Year Remaining
				</h3>
				<div class="space-y-4">
					<div class="flex justify-between items-baseline">
						<span class="text-carbon-400">Days left</span>
						<span class="font-mono text-2xl text-carbon-100">{status.days_left_year}</span>
					</div>
					<div class="flex justify-between items-baseline">
						<span class="text-carbon-400">Miles available</span>
						<span class="font-mono text-2xl text-carbon-100">{formatNumber(Math.round(status.miles_left_year))}</span>
					</div>
					<div class="h-2 bg-carbon-800 rounded-full overflow-hidden">
						<div 
							class="h-full bg-gradient-to-r from-accent-primary to-accent-secondary rounded-full transition-all duration-1000"
							style="width: {Math.min(100, (365 - status.days_left_year) / 365 * 100)}%"
						></div>
					</div>
				</div>
			</div>

			<div class="card animate-slide-up stagger-3">
				<h3 class="text-lg font-semibold text-carbon-100 mb-4 flex items-center gap-2">
					<svg class="w-5 h-5 text-accent-secondary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
					</svg>
					Term Remaining
				</h3>
				<div class="space-y-4">
					<div class="flex justify-between items-baseline">
						<span class="text-carbon-400">Time left</span>
						<span class="font-mono text-2xl text-carbon-100">{status.years_left_term}y {status.days_left_term}d</span>
					</div>
					<div class="flex justify-between items-baseline">
						<span class="text-carbon-400">Miles available</span>
						<span class="font-mono text-2xl text-carbon-100">{formatNumber(Math.round(status.miles_left_term))}</span>
					</div>
					<!-- Final-mileage estimate (#3): projected odometer at plan end -->
					<div class="flex justify-between items-baseline">
						<span class="text-carbon-400">Est. final odometer</span>
						<span class="font-mono text-2xl text-carbon-100">{formatNumber(Math.round(status.estimated_final_mileage))} <span class="text-base text-carbon-500">mi</span></span>
					</div>
					<!-- Renewal countdown (#3) -->
					<div class="text-sm text-carbon-500">
						Plan ends {formatDate(status.plan_end)} — {formatNumber(status.days_to_end)} days away
					</div>
				</div>
			</div>
		</div>

		<!-- Projection -->
		<div class="card animate-slide-up stagger-4 mb-8">
			<h3 class="text-lg font-semibold text-carbon-100 mb-4 flex items-center gap-2">
				<svg class="w-5 h-5 text-gauge-blue" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6" />
				</svg>
				Year-End Projection
			</h3>
			<div class="flex flex-col gap-4 sm:flex-row sm:items-center">
				<div class="flex-1">
					<p class="text-carbon-400 mb-2">At your current rate of {formatNumber(status.daily_rate, 1)} mi/day:</p>
					<p class="text-2xl font-mono {status.projected_over ? 'text-gauge-red' : 'text-gauge-green'}">
						{formatNumber(Math.round(status.projected_end))} mi
						<span class="text-lg">{status.projected_over ? 'over' : 'under'} allowance</span>
					</p>
				</div>
				<div class="w-16 h-16 shrink-0 rounded-full flex items-center justify-center {status.projected_over ? 'bg-gauge-red/20' : 'bg-gauge-green/20'}">
					{#if status.projected_over}
						<svg class="w-8 h-8 text-gauge-red" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
						</svg>
					{:else}
						<svg class="w-8 h-8 text-gauge-green" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
						</svg>
					{/if}
				</div>
			</div>

			<!-- Overage cost estimate (#5): only when an excess rate is configured -->
			{#if status.excess_rate}
				<div class="mt-4 pt-4 border-t border-carbon-800 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
					<div>
						<p class="text-sm text-carbon-400">Projected penalty over the full term</p>
						<p class="text-xs text-carbon-500 mt-1">
							{#if status.projected_excess_miles > 0}
								{formatNumber(Math.round(status.projected_excess_miles))} mi over · {formatMoneyMinor(status.excess_rate ?? 0, $settings.currency)}/mile
							{:else}
								Within allowance at your current pace
							{/if}
						</p>
					</div>
					<p class="text-2xl font-mono {status.projected_excess_miles > 0 ? 'text-gauge-red' : 'text-gauge-green'}">
						{formatMoneyMinor(status.projected_overage_cost_minor ?? 0, $settings.currency)}
					</p>
				</div>
			{/if}
		</div>
		{:else}
			<div class="grid grid-cols-1 lg:grid-cols-3 gap-4 sm:gap-6 mb-8">
				<div class="lg:col-span-1 card flex flex-col justify-center py-8 animate-slide-up">
					<p class="text-sm text-carbon-500 mb-2">No allowance policy</p>
					<p class="text-4xl sm:text-5xl font-mono font-bold text-carbon-100">{formatNumber(status.latest_reading)}</p>
					<p class="text-lg text-carbon-400 mt-1">mi current odometer</p>
				</div>

				<div class="lg:col-span-2 grid grid-cols-1 sm:grid-cols-2 gap-4">
					<StatCard 
						title="Current Odometer" 
						value={formatNumber(status.latest_reading)} 
						unit="mi"
						subtitle="Last updated {formatDate(status.latest_date)}"
					/>
					<StatCard
						title="Daily Rate"
						value={formatNumber(status.daily_rate, 1)}
						unit="mi/day"
						subtitle="Lifetime pace"
						tooltip="Average mi/day since your first recorded reading."
					/>
					<div class="sm:col-span-2 card animate-slide-up">
						<h3 class="text-sm font-medium text-carbon-400 mb-3">Average Annual Mileage</h3>
						<div class="flex flex-col items-start justify-between gap-4 sm:flex-row sm:items-end">
							<div>
								<div class="flex items-baseline gap-1">
									<span class="number-display text-carbon-100">{formatNumber(Math.round(status.avg_annual_mileage))}</span>
									<span class="number-unit">mi/yr</span>
								</div>
								<p class="mt-1 text-xs text-carbon-500">Lifetime average</p>
							</div>
							<div class="text-left sm:text-right">
								<div class="flex items-baseline gap-1 sm:justify-end">
									<span class="font-mono text-xl font-semibold text-carbon-300">{formatNumber(Math.round(status.recent_annual_mileage))}</span>
									<span class="text-sm text-carbon-500">mi/yr</span>
								</div>
								<div class="mt-1 flex flex-wrap items-center gap-2 sm:justify-end">
									<p class="text-xs text-carbon-500">Last 90 days</p>
									{#if trendBadge}
										<span class="flex items-center gap-1 text-xs font-medium {trendBadge.color}">
											<span>{trendBadge.icon}</span>
											<span>{trendBadge.label}</span>
										</span>
									{/if}
								</div>
							</div>
						</div>
					</div>
				</div>
			</div>
		{/if}

		<!-- Quick Add Section -->
		<div class="card animate-slide-up stagger-5">
			<div class="flex flex-wrap items-center justify-between gap-3 mb-4">
				<h3 class="text-lg font-semibold text-carbon-100">Quick Add</h3>
				<a href="/add" class="text-sm text-accent-primary hover:underline">Full form →</a>
			</div>
			<p class="text-carbon-500 text-sm mb-4">Add miles to your current odometer ({formatNumber(status.latest_reading)} mi)</p>
			<QuickAdd currentOdometer={status.latest_reading} on:add={handleQuickAdd} />
			{#if quickAddLoading}
				<p class="text-sm text-carbon-400 mt-2 animate-pulse">Saving...</p>
			{/if}
		</div>
	{/if}
</div>
