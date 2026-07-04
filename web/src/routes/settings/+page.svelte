<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { listVehicles, createVehicle, getVehicle, updatePlan, getReadings, getAlertPrefs, updateAlertPrefs, changePassword, type VehicleListItem, type VehicleStatus } from '$lib/api';
	import { mode, user, logout } from '$lib/auth';

	async function handleLogout() {
		await logout();
		goto('/login');
	}

	let vehicles: VehicleListItem[] = [];
	let loading = true;
	let error = '';
	let success = '';
	let showAddForm = false;
	let submitting = false;
	let alertPrefs = { enabled: true, threshold: 100 };
	let loadingAlerts = false;
	let savingAlerts = false;
	let currentPassword = '';
	let newPassword = '';
	let confirmPassword = '';
	let changingPassword = false;

	// Per-vehicle excess rate (pence/excess mile), loaded from each vehicle's status.
	let excessRates: Record<string, number> = {};
	let savingRate: Record<string, boolean> = {};
	let vehicleStatuses: Record<string, VehicleStatus> = {};
	let showPlanForm: Record<string, boolean> = {};
	let savingPlan: Record<string, boolean> = {};
	let planForms: Record<string, {
		start_date: string;
		end_date: string;
		annual_allowance: number;
		start_miles: number;
		excess_rate: number;
	}> = {};

	// New vehicle form
	let newVehicle = {
		id: '',
		vehicle: '',
		has_plan: true,
		start_date: '',
		end_date: '',
		annual_allowance: 10000,
		start_miles: 0,
		excess_rate: 0
	};

	onMount(async () => {
		await loadVehicles();
		if ($mode === 'hosted') {
			await loadAlertPrefs();
		}
	});

	async function loadAlertPrefs() {
		loadingAlerts = true;
		try {
			const prefs = await getAlertPrefs();
			alertPrefs = { enabled: prefs.enabled, threshold: prefs.threshold };
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load alert preferences';
		} finally {
			loadingAlerts = false;
		}
	}

	async function handleSaveAlertPrefs() {
		if (alertPrefs.threshold <= 0) {
			error = 'Alert threshold must be greater than 0';
			return;
		}
		savingAlerts = true;
		error = '';
		try {
			const prefs = await updateAlertPrefs(alertPrefs);
			alertPrefs = { enabled: prefs.enabled, threshold: prefs.threshold };
			success = 'Alert preferences updated.';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to update alert preferences';
		} finally {
			savingAlerts = false;
		}
	}

	async function handleChangePassword() {
		error = '';
		success = '';
		if (newPassword.length < 8) {
			error = 'New password must be at least 8 characters';
			return;
		}
		if (newPassword !== confirmPassword) {
			error = 'New passwords do not match';
			return;
		}
		changingPassword = true;
		try {
			await changePassword(currentPassword, newPassword);
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';
			success = 'Password changed.';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to change password';
		} finally {
			changingPassword = false;
		}
	}

	async function loadVehicles() {
		loading = true;
		try {
			vehicles = await listVehicles();
			// Pull each vehicle's current excess rate so the inline editor prefills.
			const rates: Record<string, number> = {};
			const statuses: Record<string, VehicleStatus> = {};
			await Promise.all(
				vehicles.map(async (v) => {
					try {
						const s = await getVehicle(v.id);
						statuses[v.id] = s;
						rates[v.id] = s.excess_rate ?? 0;
					} catch {
						rates[v.id] = 0;
					}
				})
			);
			excessRates = rates;
			vehicleStatuses = statuses;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load vehicles';
		} finally {
			loading = false;
		}
	}

	async function handleSaveRate(id: string) {
		savingRate = { ...savingRate, [id]: true };
		error = '';
		try {
			await updatePlan(id, { excess_rate: excessRates[id] });
			success = `Excess rate updated for ${id}.`;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to update excess rate';
		} finally {
			savingRate = { ...savingRate, [id]: false };
		}
	}

	async function togglePlanForm(id: string) {
		showPlanForm = { ...showPlanForm, [id]: !showPlanForm[id] };
		if (!showPlanForm[id] || planForms[id]) return;
		try {
			const readings = await getReadings(id);
			const first = readings[0];
			planForms = {
				...planForms,
				[id]: {
					start_date: first?.date ?? new Date().toISOString().slice(0, 10),
					end_date: '',
					annual_allowance: 10000,
					start_miles: first?.miles ?? 0,
					excess_rate: 0
				}
			};
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load readings';
		}
	}

	async function handleAddPlan(id: string) {
		const form = planForms[id];
		if (!form?.start_date || !form.end_date || !form.annual_allowance) {
			error = 'Please fill in all allowance plan fields';
			return;
		}
		savingPlan = { ...savingPlan, [id]: true };
		error = '';
		try {
			await updatePlan(id, form);
			success = `Allowance plan added for ${id}.`;
			showPlanForm = { ...showPlanForm, [id]: false };
			await loadVehicles();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to add allowance plan';
		} finally {
			savingPlan = { ...savingPlan, [id]: false };
		}
	}

	async function handleCreateVehicle() {
		if (!newVehicle.id || (newVehicle.has_plan && (!newVehicle.start_date || !newVehicle.end_date))) {
			error = 'Please fill in all required fields';
			return;
		}

		submitting = true;
		error = '';

		try {
			const payload = {
				id: newVehicle.id.toLowerCase().replace(/\s+/g, '_'),
				vehicle: newVehicle.vehicle || newVehicle.id,
				start_miles: newVehicle.start_miles,
				start_date: newVehicle.start_date || undefined
			};
			await createVehicle(newVehicle.has_plan ? {
				...payload,
				end_date: newVehicle.end_date,
				annual_allowance: newVehicle.annual_allowance,
				excess_rate: newVehicle.excess_rate
			} : payload);
			
			success = `Vehicle "${newVehicle.vehicle || newVehicle.id}" created successfully!`;
			showAddForm = false;
			newVehicle = {
				id: '',
				vehicle: '',
				has_plan: true,
				start_date: '',
				end_date: '',
				annual_allowance: 10000,
				start_miles: 0,
				excess_rate: 0
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
			has_plan: true,
			start_date: '',
			end_date: '',
			annual_allowance: 10000,
			start_miles: 0,
			excess_rate: 0
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

	{#if $mode === 'hosted'}
		<!-- Account Section (hosted mode only) -->
		<section class="mb-8">
			<h2 class="text-xl font-semibold text-carbon-100 mb-4">Account</h2>
			<div class="flex items-center justify-between p-4 bg-carbon-900/40 border border-carbon-800 rounded-xl">
				<div>
					<p class="text-sm text-carbon-400">Signed in as</p>
					<p class="text-carbon-100 font-medium">{$user?.email ?? '—'}</p>
				</div>
				<button class="btn-secondary" on:click={handleLogout}>Sign out</button>
			</div>
			<form on:submit|preventDefault={handleChangePassword} class="mt-4 p-4 bg-carbon-900/40 border border-carbon-800 rounded-xl">
				<h3 class="text-base font-semibold text-carbon-100 mb-4">Change password</h3>
				<div class="grid gap-4 md:grid-cols-3">
					<div>
						<label for="currentPassword" class="label">Current password</label>
						<input
							id="currentPassword"
							type="password"
							bind:value={currentPassword}
							autocomplete="current-password"
							class="input"
							required
						/>
					</div>
					<div>
						<label for="newPassword" class="label">New password</label>
						<input
							id="newPassword"
							type="password"
							bind:value={newPassword}
							autocomplete="new-password"
							class="input"
							minlength="8"
							required
						/>
					</div>
					<div>
						<label for="confirmPassword" class="label">Confirm password</label>
						<input
							id="confirmPassword"
							type="password"
							bind:value={confirmPassword}
							autocomplete="new-password"
							class="input"
							minlength="8"
							required
						/>
					</div>
				</div>
				<div class="mt-4 flex justify-end">
					<button class="btn-secondary" type="submit" disabled={changingPassword}>
						{changingPassword ? 'Changing...' : 'Change password'}
					</button>
				</div>
			</form>
		</section>

		<section class="mb-8">
			<h2 class="text-xl font-semibold text-carbon-100 mb-4">Alerts</h2>
			<div class="p-4 bg-carbon-900/40 border border-carbon-800 rounded-xl">
				{#if loadingAlerts}
					<p class="text-carbon-400 text-sm">Loading alert preferences...</p>
				{:else}
					<div class="flex items-center justify-between gap-4">
						<label class="flex items-center gap-3">
							<input type="checkbox" bind:checked={alertPrefs.enabled} class="w-4 h-4 accent-accent-primary" />
							<span class="text-carbon-100 font-medium">Email allowance alerts</span>
						</label>
						<button class="btn-secondary" on:click={handleSaveAlertPrefs} disabled={savingAlerts}>
							{savingAlerts ? 'Saving...' : 'Save'}
						</button>
					</div>
					<div class="mt-4 max-w-xs">
						<label for="alertThreshold" class="label">Threshold (% used)</label>
						<input
							id="alertThreshold"
							type="number"
							bind:value={alertPrefs.threshold}
							class="input font-mono"
							min="1"
							step="1"
							disabled={!alertPrefs.enabled}
						/>
					</div>
				{/if}
			</div>
		</section>
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
						<div class="card animate-slide-up">
							<div class="flex items-center justify-between">
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

							{#if vehicleStatuses[vehicle.id]?.has_plan}
								<!-- Excess-rate editor (#5): settable on existing policy vehicles -->
								<div class="mt-4 pt-4 border-t border-carbon-800 flex items-end gap-3">
									<div class="flex-1">
										<label for="rate-{vehicle.id}" class="label">Excess Rate (pence per mile over)</label>
										<input
											type="number"
											id="rate-{vehicle.id}"
											bind:value={excessRates[vehicle.id]}
											class="input font-mono"
											min="0"
											step="1"
											placeholder="0"
										/>
									</div>
									<button
										class="btn-secondary"
										on:click={() => handleSaveRate(vehicle.id)}
										disabled={savingRate[vehicle.id]}
									>
										{savingRate[vehicle.id] ? 'Saving...' : 'Save'}
									</button>
								</div>
							{:else}
								<div class="mt-4 pt-4 border-t border-carbon-800">
									<div class="flex items-center justify-between gap-3">
										<p class="text-sm text-carbon-400">No allowance policy</p>
										<button class="btn-secondary text-sm" on:click={() => togglePlanForm(vehicle.id)}>
											{showPlanForm[vehicle.id] ? 'Cancel' : 'Add allowance plan'}
										</button>
									</div>
									{#if showPlanForm[vehicle.id] && planForms[vehicle.id]}
										<form on:submit|preventDefault={() => handleAddPlan(vehicle.id)} class="mt-4 space-y-4">
											<div class="grid grid-cols-2 gap-4">
												<div>
													<label for="planStart-{vehicle.id}" class="label">Plan Start Date *</label>
													<input id="planStart-{vehicle.id}" type="date" bind:value={planForms[vehicle.id].start_date} class="input" required />
												</div>
												<div>
													<label for="planEnd-{vehicle.id}" class="label">Plan End Date *</label>
													<input id="planEnd-{vehicle.id}" type="date" bind:value={planForms[vehicle.id].end_date} class="input" required />
												</div>
											</div>
											<div class="grid grid-cols-2 gap-4">
												<div>
													<label for="planAllowance-{vehicle.id}" class="label">Annual Allowance (miles) *</label>
													<input id="planAllowance-{vehicle.id}" type="number" bind:value={planForms[vehicle.id].annual_allowance} class="input font-mono" min="1" required />
												</div>
												<div>
													<label for="planMiles-{vehicle.id}" class="label">Starting Odometer *</label>
													<input id="planMiles-{vehicle.id}" type="number" bind:value={planForms[vehicle.id].start_miles} class="input font-mono" min="0" required />
												</div>
											</div>
											<div>
												<label for="planRate-{vehicle.id}" class="label">Excess Rate (pence per mile over)</label>
												<input id="planRate-{vehicle.id}" type="number" bind:value={planForms[vehicle.id].excess_rate} class="input font-mono" min="0" step="1" />
											</div>
											<button type="submit" class="btn-primary" disabled={savingPlan[vehicle.id]}>
												{savingPlan[vehicle.id] ? 'Saving...' : 'Save allowance plan'}
											</button>
										</form>
									{/if}
								</div>
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

						<label class="flex items-center gap-3 p-4 bg-carbon-900/40 border border-carbon-800 rounded-lg">
							<input type="checkbox" bind:checked={newVehicle.has_plan} class="w-4 h-4 accent-accent-primary" />
							<span class="text-sm text-carbon-200">Has a mileage allowance (PCP/lease/insurance)</span>
						</label>

						<div class="grid grid-cols-2 gap-4">
							<div>
								<label for="startDate" class="label">{newVehicle.has_plan ? 'Plan Start Date *' : 'Reading Date'}</label>
								<input
									type="date"
									id="startDate"
									bind:value={newVehicle.start_date}
									class="input"
									required={newVehicle.has_plan}
								/>
							</div>
							{#if newVehicle.has_plan}
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
							{/if}
						</div>

						<div class="grid grid-cols-2 gap-4">
							{#if newVehicle.has_plan}
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
							{/if}
							<div>
								<label for="startMiles" class="label">{newVehicle.has_plan ? 'Starting Odometer *' : 'Current Odometer *'}</label>
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

						{#if newVehicle.has_plan}
							<div>
								<label for="excessRate" class="label">Excess Rate (pence per mile over)</label>
								<input
									type="number"
									id="excessRate"
									bind:value={newVehicle.excess_rate}
									class="input font-mono"
									min="0"
									step="1"
									placeholder="e.g., 10"
								/>
								<p class="text-xs text-carbon-500 mt-1">Optional — used to estimate the overage penalty if you exceed the allowance</p>
							</div>
						{/if}

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
