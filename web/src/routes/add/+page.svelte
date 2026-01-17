<script lang="ts">
	import { onMount } from 'svelte';
	import { getCurrentVehicle, getVehicle, addReading, formatNumber, type VehicleStatus } from '$lib/api';
	import QuickAdd from '$lib/components/QuickAdd.svelte';

	let status: VehicleStatus | null = null;
	let loading = true;
	let submitting = false;
	let error = '';
	let success = '';

	let odometerValue = '';
	let dateValue = new Date().toISOString().split('T')[0];
	let forceOverride = false;

	onMount(async () => {
		try {
			const current = await getCurrentVehicle();
			if (current.current) {
				status = await getVehicle(current.current);
				odometerValue = '';
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load vehicle';
		} finally {
			loading = false;
		}
	});

	async function handleSubmit() {
		if (!status || !odometerValue) return;

		submitting = true;
		error = '';
		success = '';

		try {
			await addReading(status.id, {
				miles: parseInt(odometerValue),
				date: dateValue,
				force: forceOverride
			});
			success = `Recorded ${formatNumber(parseInt(odometerValue))} mi for ${dateValue}`;
			
			// Refresh status
			status = await getVehicle(status.id);
			odometerValue = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to add reading';
		} finally {
			submitting = false;
		}
	}

	async function handleQuickAdd(event: CustomEvent<{ miles: number }>) {
		if (!status) return;
		odometerValue = event.detail.miles.toString();
		await handleSubmit();
	}

	$: isValidInput = odometerValue && parseInt(odometerValue) > 0;
	$: suggestedMiles = status ? status.latest_reading : 0;
</script>

<svelte:head>
	<title>Add Mileage | MileMinder</title>
</svelte:head>

<div class="p-8 max-w-2xl mx-auto">
	<header class="mb-8 animate-fade-in">
		<h1 class="text-3xl font-display font-bold text-carbon-100">Add Mileage</h1>
		<p class="text-carbon-500 mt-2">Record your current odometer reading</p>
	</header>

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="animate-pulse text-carbon-400">Loading...</div>
		</div>
	{:else if !status}
		<div class="card border-gauge-amber/30 bg-gauge-amber/5">
			<p class="text-gauge-amber">No vehicle selected. Please select a vehicle first.</p>
		</div>
	{:else}
		<!-- Current Status -->
		<div class="card mb-6 animate-slide-up">
			<div class="flex items-center justify-between">
				<div>
					<p class="text-sm text-carbon-400">Current odometer</p>
					<p class="text-3xl font-mono font-bold text-carbon-100">{formatNumber(status.latest_reading)} <span class="text-lg text-carbon-500">mi</span></p>
				</div>
				<div class="text-right">
					<p class="text-sm text-carbon-400">Vehicle</p>
					<p class="text-lg font-medium text-carbon-100">{status.vehicle || status.id}</p>
				</div>
			</div>
		</div>

		<!-- Quick Add -->
		<div class="card mb-6 animate-slide-up stagger-1">
			<h2 class="text-lg font-semibold text-carbon-100 mb-4">Quick Add</h2>
			<p class="text-sm text-carbon-500 mb-4">Quickly add common distances to your current reading</p>
			<QuickAdd currentOdometer={status.latest_reading} on:add={handleQuickAdd} />
		</div>

		<!-- Manual Entry Form -->
		<form on:submit|preventDefault={handleSubmit} class="card animate-slide-up stagger-2">
			<h2 class="text-lg font-semibold text-carbon-100 mb-6">Manual Entry</h2>
			
			{#if error}
				<div class="mb-4 p-4 bg-gauge-red/10 border border-gauge-red/30 rounded-lg">
					<p class="text-gauge-red text-sm">{error}</p>
					{#if error.includes('less than')}
						<label class="flex items-center gap-2 mt-2 text-sm text-carbon-400">
							<input type="checkbox" bind:checked={forceOverride} class="rounded bg-carbon-800 border-carbon-700">
							<span>Force override (allow lower reading)</span>
						</label>
					{/if}
				</div>
			{/if}

			{#if success}
				<div class="mb-4 p-4 bg-gauge-green/10 border border-gauge-green/30 rounded-lg">
					<p class="text-gauge-green text-sm flex items-center gap-2">
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
						</svg>
						{success}
					</p>
				</div>
			{/if}

			<div class="space-y-6">
				<div>
					<label for="odometer" class="label">Odometer Reading</label>
					<div class="relative">
						<input
							type="number"
							id="odometer"
							bind:value={odometerValue}
							placeholder={suggestedMiles.toString()}
							class="input font-mono text-xl pr-12"
							min="0"
							required
						/>
						<span class="absolute right-4 top-1/2 -translate-y-1/2 text-carbon-500">mi</span>
					</div>
					<p class="text-xs text-carbon-500 mt-2">
						Must be at least {formatNumber(status.latest_reading)} mi (current reading)
					</p>
				</div>

				<div>
					<label for="date" class="label">Date</label>
					<input
						type="date"
						id="date"
						bind:value={dateValue}
						class="input"
						max={new Date().toISOString().split('T')[0]}
					/>
				</div>

				<button
					type="submit"
					class="btn-primary w-full py-4 text-lg flex items-center justify-center gap-2"
					disabled={!isValidInput || submitting}
				>
					{#if submitting}
						<svg class="animate-spin w-5 h-5" fill="none" viewBox="0 0 24 24">
							<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
							<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
						</svg>
						Saving...
					{:else}
						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
						</svg>
						Record Reading
					{/if}
				</button>
			</div>
		</form>
	{/if}
</div>
