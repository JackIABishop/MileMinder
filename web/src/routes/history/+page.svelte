<script lang="ts">
	import { onMount } from 'svelte';
	import { getCurrentVehicle, getVehicle, getReadings, deleteReading, getExportURL, formatNumber, formatDate, type VehicleStatus, type Reading } from '$lib/api';

	let status: VehicleStatus | null = null;
	let readings: Reading[] = [];
	let loading = true;
	let error = '';
	let deleteConfirm: string | null = null;

	onMount(async () => {
		await loadData();
	});

	async function loadData() {
		loading = true;
		error = '';
		try {
			const current = await getCurrentVehicle();
			if (current.current) {
				status = await getVehicle(current.current);
				readings = await getReadings(current.current);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load data';
		} finally {
			loading = false;
		}
	}

	async function handleDelete(date: string) {
		if (!status) return;
		
		try {
			await deleteReading(status.id, date);
			deleteConfirm = null;
			await loadData();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to delete reading';
		}
	}

	function getMilesDriven(index: number): number {
		if (index === 0) return 0;
		return readings[index].miles - readings[index - 1].miles;
	}

	function getDaysBetween(index: number): number {
		if (index === 0) return 0;
		const current = new Date(readings[index].date);
		const previous = new Date(readings[index - 1].date);
		return Math.round((current.getTime() - previous.getTime()) / (1000 * 60 * 60 * 24));
	}
</script>

<svelte:head>
	<title>History | MileMinder</title>
</svelte:head>

<div class="p-8">
	<header class="mb-8 animate-fade-in flex items-start justify-between">
		<div>
			<h1 class="text-3xl font-display font-bold text-carbon-100">Reading History</h1>
			<p class="text-carbon-500 mt-2">View and manage your odometer readings</p>
		</div>
		{#if status}
			<a 
				href={getExportURL(status.id)} 
				download="{status.id}_readings.csv"
				class="btn-secondary flex items-center gap-2"
			>
				<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
				</svg>
				Export CSV
			</a>
		{/if}
	</header>

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="animate-pulse text-carbon-400">Loading...</div>
		</div>
	{:else if error}
		<div class="card border-gauge-red/30 bg-gauge-red/5 mb-6">
			<p class="text-gauge-red">{error}</p>
		</div>
	{:else if !status}
		<div class="card">
			<p class="text-carbon-400">No vehicle selected. Please select a vehicle first.</p>
		</div>
	{:else if readings.length === 0}
		<div class="card text-center py-12">
			<svg class="w-16 h-16 mx-auto text-carbon-600 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
			</svg>
			<h2 class="text-xl font-semibold text-carbon-300 mb-2">No Readings Yet</h2>
			<p class="text-carbon-500 mb-6">Start logging your odometer readings</p>
			<a href="/add" class="btn-primary">Add First Reading</a>
		</div>
	{:else}
		<!-- Stats -->
		<div class="grid grid-cols-3 gap-4 mb-6">
			<div class="card animate-slide-up">
				<p class="text-sm text-carbon-400 mb-1">Total Readings</p>
				<p class="text-2xl font-mono font-bold text-carbon-100">{readings.length}</p>
			</div>
			<div class="card animate-slide-up stagger-1">
				<p class="text-sm text-carbon-400 mb-1">First Reading</p>
				<p class="text-lg font-mono font-bold text-carbon-100">{formatDate(readings[0].date)}</p>
			</div>
			<div class="card animate-slide-up stagger-2">
				<p class="text-sm text-carbon-400 mb-1">Latest Reading</p>
				<p class="text-lg font-mono font-bold text-carbon-100">{formatDate(readings[readings.length - 1].date)}</p>
			</div>
		</div>

		<!-- Table -->
		<div class="card animate-slide-up stagger-3 overflow-hidden">
			<div class="overflow-x-auto">
				<table class="w-full">
					<thead>
						<tr class="border-b border-carbon-800">
							<th class="text-left py-3 px-4 text-sm font-medium text-carbon-400">Date</th>
							<th class="text-right py-3 px-4 text-sm font-medium text-carbon-400">Odometer</th>
							<th class="text-right py-3 px-4 text-sm font-medium text-carbon-400">Miles Driven</th>
							<th class="text-right py-3 px-4 text-sm font-medium text-carbon-400">Days</th>
							<th class="text-right py-3 px-4 text-sm font-medium text-carbon-400">Daily Avg</th>
							<th class="py-3 px-4"></th>
						</tr>
					</thead>
					<tbody>
						{#each readings.slice().reverse() as reading, idx}
							{@const originalIndex = readings.length - 1 - idx}
							{@const milesDriven = getMilesDriven(originalIndex)}
							{@const days = getDaysBetween(originalIndex)}
							{@const dailyAvg = days > 0 ? milesDriven / days : 0}
							<tr class="border-b border-carbon-800/50 hover:bg-carbon-800/30 transition-colors">
								<td class="py-3 px-4">
									<span class="font-medium text-carbon-100">{formatDate(reading.date)}</span>
								</td>
								<td class="py-3 px-4 text-right">
									<span class="font-mono text-carbon-100">{formatNumber(reading.miles)}</span>
									<span class="text-carbon-500 text-sm ml-1">mi</span>
								</td>
								<td class="py-3 px-4 text-right">
									{#if originalIndex > 0}
										<span class="font-mono text-carbon-300">+{formatNumber(milesDriven)}</span>
									{:else}
										<span class="text-carbon-600">—</span>
									{/if}
								</td>
								<td class="py-3 px-4 text-right">
									{#if originalIndex > 0}
										<span class="font-mono text-carbon-300">{days}</span>
									{:else}
										<span class="text-carbon-600">—</span>
									{/if}
								</td>
								<td class="py-3 px-4 text-right">
									{#if originalIndex > 0 && days > 0}
										<span class="font-mono text-carbon-300">{formatNumber(dailyAvg, 1)}</span>
									{:else}
										<span class="text-carbon-600">—</span>
									{/if}
								</td>
								<td class="py-3 px-4 text-right">
									{#if originalIndex > 0}
										{#if deleteConfirm === reading.date}
											<div class="flex items-center justify-end gap-2">
												<button
													class="text-xs text-gauge-red hover:underline"
													on:click={() => handleDelete(reading.date)}
												>
													Confirm
												</button>
												<button
													class="text-xs text-carbon-400 hover:underline"
													on:click={() => deleteConfirm = null}
												>
													Cancel
												</button>
											</div>
										{:else}
											<button
												class="text-carbon-500 hover:text-gauge-red transition-colors"
												on:click={() => deleteConfirm = reading.date}
												title="Delete reading"
											>
												<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
													<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
												</svg>
											</button>
										{/if}
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}
</div>
