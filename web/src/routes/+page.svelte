<script lang="ts">
	import { onMount } from 'svelte';
	import { getCurrentVehicle, getVehicle, addReading, formatNumber, formatDate, getDeltaStatus, type VehicleStatus } from '$lib/api';
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
			const current = await getCurrentVehicle();
			if (current.current) {
				status = await getVehicle(current.current);
			} else {
				error = 'No vehicle selected. Please add a vehicle first.';
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
</script>

<svelte:head>
	<title>Dashboard | MileMinder</title>
</svelte:head>

<div class="p-8">
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
		<!-- Main Gauge -->
		<div class="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-8">
			<div class="lg:col-span-1 card flex flex-col items-center justify-center py-8 animate-slide-up">
				<Gauge percent={status.percent_used} size={220} strokeWidth={14}>
					<span class="text-5xl font-mono font-bold text-carbon-100">
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
			<div class="lg:col-span-2 grid grid-cols-2 gap-4">
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
					color={status.daily_rate > (status.annual_allowance / 365) ? 'amber' : 'green'}
				/>
				
				<StatCard 
					title="Annual Allowance" 
					value={formatNumber(status.annual_allowance)} 
					unit="mi"
					subtitle="{formatNumber(Math.round(status.annual_allowance / 365))} mi/day ideal"
				/>
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
					<div class="text-sm text-carbon-500">
						Plan ends {formatDate(status.plan_end)}
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
			<div class="flex items-center gap-4">
				<div class="flex-1">
					<p class="text-carbon-400 mb-2">At your current rate of {formatNumber(status.daily_rate, 1)} mi/day:</p>
					<p class="text-2xl font-mono {status.projected_over ? 'text-gauge-red' : 'text-gauge-green'}">
						{formatNumber(Math.round(status.projected_end))} mi
						<span class="text-lg">{status.projected_over ? 'over' : 'under'} allowance</span>
					</p>
				</div>
				<div class="w-16 h-16 rounded-full flex items-center justify-center {status.projected_over ? 'bg-gauge-red/20' : 'bg-gauge-green/20'}">
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
		</div>

		<!-- Quick Add Section -->
		<div class="card animate-slide-up stagger-5">
			<div class="flex items-center justify-between mb-4">
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
