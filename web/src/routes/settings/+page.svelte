<script lang="ts">
	import { onMount } from 'svelte';
	import { listVehicles, createVehicle, formatDate, type VehicleListItem } from '$lib/api';

	let vehicles: VehicleListItem[] = [];
	let loading = true;
	let error = '';
	let success = '';
	let showAddForm = false;
	let submitting = false;

	// New vehicle form
	let newVehicle = {
		id: '',
		vehicle: '',
		start_date: '',
		end_date: '',
		annual_allowance: 10000,
		start_miles: 0
	};

	onMount(async () => {
		await loadVehicles();
	});

	async function loadVehicles() {
		loading = true;
		try {
			vehicles = await listVehicles();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load vehicles';
		} finally {
			loading = false;
		}
	}

	async function handleCreateVehicle() {
		if (!newVehicle.id || !newVehicle.start_date || !newVehicle.end_date) {
			error = 'Please fill in all required fields';
			return;
		}

		submitting = true;
		error = '';

		try {
			await createVehicle({
				id: newVehicle.id.toLowerCase().replace(/\s+/g, '_'),
				vehicle: newVehicle.vehicle || newVehicle.id,
				start_date: newVehicle.start_date,
				end_date: newVehicle.end_date,
				annual_allowance: newVehicle.annual_allowance,
				start_miles: newVehicle.start_miles
			});
			
			success = `Vehicle "${newVehicle.vehicle || newVehicle.id}" created successfully!`;
			showAddForm = false;
			newVehicle = {
				id: '',
				vehicle: '',
				start_date: '',
				end_date: '',
				annual_allowance: 10000,
				start_miles: 0
			};
			await loadVehicles();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to create vehicle';
		} finally {
			submitting = false;
		}
	}

	function resetForm() {
		showAddForm = false;
		error = '';
		newVehicle = {
			id: '',
			vehicle: '',
			start_date: '',
			end_date: '',
			annual_allowance: 10000,
			start_miles: 0
		};
	}
</script>

<svelte:head>
	<title>Settings | MileMinder</title>
</svelte:head>

<div class="p-8 max-w-3xl mx-auto">
	<header class="mb-8 animate-fade-in">
		<h1 class="text-3xl font-display font-bold text-carbon-100">Settings</h1>
		<p class="text-carbon-500 mt-2">Manage your vehicles and preferences</p>
	</header>

	{#if success}
		<div class="mb-6 p-4 bg-gauge-green/10 border border-gauge-green/30 rounded-lg animate-fade-in">
			<p class="text-gauge-green text-sm flex items-center gap-2">
				<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
				</svg>
				{success}
			</p>
		</div>
	{/if}

	<!-- Vehicles Section -->
	<section class="mb-8">
		<div class="flex items-center justify-between mb-4">
			<h2 class="text-xl font-semibold text-carbon-100">Vehicles</h2>
			{#if !showAddForm}
				<button class="btn-primary" on:click={() => showAddForm = true}>
					<svg class="w-4 h-4 inline mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
					</svg>
					Add Vehicle
				</button>
			{/if}
		</div>

		{#if loading}
			<div class="card animate-pulse">
				<p class="text-carbon-400">Loading vehicles...</p>
			</div>
		{:else}
			<!-- Existing Vehicles -->
			{#if vehicles.length > 0}
				<div class="space-y-3 mb-6">
					{#each vehicles as vehicle}
						<div class="card flex items-center justify-between animate-slide-up">
							<div class="flex items-center gap-4">
								<div class="w-10 h-10 rounded-lg bg-accent-primary/20 flex items-center justify-center">
									<svg class="w-5 h-5 text-accent-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
									</svg>
								</div>
								<div>
									<p class="font-medium text-carbon-100">{vehicle.vehicle || vehicle.id}</p>
									<p class="text-sm text-carbon-500">{vehicle.id}</p>
								</div>
							</div>
							{#if vehicle.is_default}
								<span class="px-3 py-1 text-xs font-medium bg-accent-primary/20 text-accent-primary rounded-full">
									Default
								</span>
							{/if}
						</div>
					{/each}
				</div>
			{:else if !showAddForm}
				<div class="card text-center py-8 mb-6">
					<p class="text-carbon-400 mb-4">No vehicles configured yet.</p>
					<button class="btn-primary" on:click={() => showAddForm = true}>
						Add Your First Vehicle
					</button>
				</div>
			{/if}

			<!-- Add Vehicle Form -->
			{#if showAddForm}
				<div class="card animate-slide-up">
					<div class="flex items-center justify-between mb-6">
						<h3 class="text-lg font-semibold text-carbon-100">Add New Vehicle</h3>
						<button class="btn-ghost text-sm" on:click={resetForm}>Cancel</button>
					</div>

					{#if error}
						<div class="mb-4 p-4 bg-gauge-red/10 border border-gauge-red/30 rounded-lg">
							<p class="text-gauge-red text-sm">{error}</p>
						</div>
					{/if}

					<form on:submit|preventDefault={handleCreateVehicle} class="space-y-6">
						<div class="grid grid-cols-2 gap-4">
							<div>
								<label for="vehicleId" class="label">Vehicle ID *</label>
								<input
									type="text"
									id="vehicleId"
									bind:value={newVehicle.id}
									placeholder="e.g., my_tesla"
									class="input"
									required
								/>
								<p class="text-xs text-carbon-500 mt-1">Used for CLI commands</p>
							</div>
							<div>
								<label for="vehicleName" class="label">Display Name</label>
								<input
									type="text"
									id="vehicleName"
									bind:value={newVehicle.vehicle}
									placeholder="e.g., Tesla Model 3"
									class="input"
								/>
							</div>
						</div>

						<div class="grid grid-cols-2 gap-4">
							<div>
								<label for="startDate" class="label">Plan Start Date *</label>
								<input
									type="date"
									id="startDate"
									bind:value={newVehicle.start_date}
									class="input"
									required
								/>
							</div>
							<div>
								<label for="endDate" class="label">Plan End Date *</label>
								<input
									type="date"
									id="endDate"
									bind:value={newVehicle.end_date}
									class="input"
									required
								/>
							</div>
						</div>

						<div class="grid grid-cols-2 gap-4">
							<div>
								<label for="annualAllowance" class="label">Annual Allowance (miles) *</label>
								<input
									type="number"
									id="annualAllowance"
									bind:value={newVehicle.annual_allowance}
									class="input font-mono"
									min="1"
									required
								/>
							</div>
							<div>
								<label for="startMiles" class="label">Starting Odometer *</label>
								<input
									type="number"
									id="startMiles"
									bind:value={newVehicle.start_miles}
									class="input font-mono"
									min="0"
									required
								/>
							</div>
						</div>

						<div class="flex gap-3 pt-4">
							<button type="submit" class="btn-primary flex-1" disabled={submitting}>
								{#if submitting}
									Creating...
								{:else}
									Create Vehicle
								{/if}
							</button>
							<button type="button" class="btn-secondary" on:click={resetForm}>
								Cancel
							</button>
						</div>
					</form>
				</div>
			{/if}
		{/if}
	</section>

	<!-- About Section -->
	<section class="card animate-slide-up stagger-3">
		<h2 class="text-lg font-semibold text-carbon-100 mb-4">About MileMinder</h2>
		<div class="space-y-3 text-sm text-carbon-400">
			<p>
				MileMinder helps you track your vehicle mileage against PCP or insurance allowances.
				Both CLI and Web UI read/write to the same data files.
			</p>
			<div class="flex items-center gap-4 pt-2">
				<span class="flex items-center gap-2">
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
					</svg>
					<code class="text-xs bg-carbon-800 px-2 py-1 rounded">~/.mileminder/</code>
				</span>
			</div>
		</div>
		<div class="mt-6 pt-4 border-t border-carbon-800 flex items-center justify-between text-sm">
			<span class="text-carbon-500">Built with Go + Svelte</span>
			<a href="https://github.com/JackIABishop/MileMinder" target="_blank" rel="noopener" class="text-accent-primary hover:underline">
				GitHub →
			</a>
		</div>
	</section>
</div>
