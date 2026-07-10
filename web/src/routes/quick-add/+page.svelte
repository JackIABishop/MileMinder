<script lang="ts">
	import { onMount, tick } from 'svelte';
	import {
		listVehicles,
		getCurrentVehicle,
		setCurrentVehicle,
		getVehicle,
		addReading,
		formatNumber,
		type VehicleListItem,
		type VehicleStatus
	} from '$lib/api';

	let vehicles: VehicleListItem[] = [];
	let selectedId = '';
	let status: VehicleStatus | null = null;

	let loading = true;
	let submitting = false;
	let error = '';
	let success = '';
	let forceOverride = false;

	let odometerValue = '';
	let inputEl: HTMLInputElement;

	// Date.toISOString() is UTC, which drifts a day off near local midnight.
	// Quick-add has no date field to correct a wrong default, so it must use
	// the device's local calendar date rather than UTC.
	function todayLocal(): string {
		const d = new Date();
		const y = d.getFullYear();
		const m = String(d.getMonth() + 1).padStart(2, '0');
		const day = String(d.getDate()).padStart(2, '0');
		return `${y}-${m}-${day}`;
	}

	onMount(async () => {
		try {
			vehicles = await listVehicles();
			const current = await getCurrentVehicle();
			const valid = current.current && vehicles.some((v) => v.id === current.current);
			selectedId = valid ? current.current : vehicles[0]?.id ?? '';
			if (selectedId) await loadStatus(selectedId);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load vehicles';
		} finally {
			loading = false;
			await tick();
			inputEl?.focus();
		}
	});

	async function loadStatus(id: string) {
		status = await getVehicle(id);
	}

	async function handleVehicleChange() {
		error = '';
		success = '';
		if (!selectedId) return;
		await loadStatus(selectedId);
		try {
			await setCurrentVehicle(selectedId);
		} catch {
			// Non-fatal: the reading below still targets the selected vehicle even
			// if the "last used" pointer fails to save.
		}
	}

	async function handleSubmit() {
		if (!selectedId || !odometerValue) return;

		submitting = true;
		error = '';
		success = '';

		try {
			const miles = parseInt(odometerValue);
			await addReading(selectedId, {
				miles,
				date: todayLocal(),
				force: forceOverride
			});
			success = `Recorded ${formatNumber(miles)} mi`;
			odometerValue = '';
			forceOverride = false;
			status = await getVehicle(selectedId);
			await tick();
			inputEl?.focus();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to add reading';
		} finally {
			submitting = false;
		}
	}

	$: isValidInput = odometerValue !== '' && parseInt(odometerValue) > 0;
</script>

<svelte:head>
	<title>Quick Add | MileMinder</title>
</svelte:head>

<div class="min-h-screen flex flex-col p-4 sm:p-6 max-w-md mx-auto w-full">
	<header class="flex items-center gap-3 py-4">
		<a
			href="/"
			class="flex h-11 w-11 items-center justify-center rounded-xl border border-carbon-800 bg-carbon-900/70 text-carbon-300"
			aria-label="Back to dashboard"
		>
			<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
			</svg>
		</a>
		<h1 class="text-xl font-display font-bold text-carbon-100">Quick Add</h1>
	</header>

	{#if loading}
		<div class="flex flex-1 items-center justify-center">
			<div class="animate-pulse text-carbon-400">Loading...</div>
		</div>
	{:else if vehicles.length === 0}
		<div class="card border-gauge-amber/30 bg-gauge-amber/5">
			<p class="text-gauge-amber">No vehicle set up yet.</p>
			<a href="/" class="btn-secondary inline-block mt-4">Go to dashboard</a>
		</div>
	{:else}
		<div class="flex-1 flex flex-col justify-center gap-6">
			{#if vehicles.length > 1}
				<div>
					<label for="vehicle" class="label">Vehicle</label>
					<select
						id="vehicle"
						class="input"
						bind:value={selectedId}
						on:change={handleVehicleChange}
					>
						{#each vehicles as vehicle}
							<option value={vehicle.id}>{vehicle.vehicle || vehicle.id}</option>
						{/each}
					</select>
				</div>
			{:else}
				<p class="text-center text-carbon-400">{vehicles[0].vehicle || vehicles[0].id}</p>
			{/if}

			{#if status}
				<p class="text-center text-sm text-carbon-500">
					Current: <span class="font-mono text-carbon-300">{formatNumber(status.latest_reading)} mi</span>
				</p>
			{/if}

			<form on:submit|preventDefault={handleSubmit} class="flex flex-col gap-4">
				{#if error}
					<div class="p-4 bg-gauge-red/10 border border-gauge-red/30 rounded-lg">
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
					<div class="p-4 bg-gauge-green/10 border border-gauge-green/30 rounded-lg">
						<p class="text-gauge-green text-sm flex items-center gap-2">
							<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
							</svg>
							{success}
						</p>
					</div>
				{/if}

				<div class="relative">
					<input
						bind:this={inputEl}
						type="number"
						inputmode="numeric"
						bind:value={odometerValue}
						placeholder="Odometer reading"
						class="input font-mono text-3xl text-center py-6 pr-14"
						min="0"
						required
					/>
					<span class="absolute right-5 top-1/2 -translate-y-1/2 text-carbon-500">mi</span>
				</div>

				<button
					type="submit"
					class="btn-primary w-full py-5 text-lg flex items-center justify-center gap-2"
					disabled={!isValidInput || submitting}
				>
					{#if submitting}
						<svg class="animate-spin w-5 h-5" fill="none" viewBox="0 0 24 24">
							<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
							<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
						</svg>
						Saving...
					{:else}
						Log Reading
					{/if}
				</button>
			</form>
		</div>
	{/if}
</div>
