<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { listVehicles, createVehicle, getVehicle, updatePlan, getReadings, getAlertPrefs, updateAlertPrefs, getReminderSettings, updateReminderSettings, updateSettings, importCSV, changePassword, getProfileExportURL, type VehicleListItem, type VehicleStatus, type VehicleProfile, type ReminderSettings } from '$lib/api';
	import { mode, user, logout } from '$lib/auth';
	import { settings } from '$lib/settings';
	import { SUPPORTED_CURRENCIES, minorUnitLabel } from '$lib/money';

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

	// Per-vehicle reading reminders (hosted mode only).
	let reminderSettings: Record<string, ReminderSettings> = {};
	let savingReminder: Record<string, boolean> = {};
	let showPlanForm: Record<string, boolean> = {};
	let savingPlan: Record<string, boolean> = {};
	let planForms: Record<string, {
		start_date: string;
		end_date: string;
		annual_allowance: number;
		start_miles: number;
		excess_rate: number;
	}> = {};

	// New vehicle form. historyFiles is the optional readings CSV imported
	// right after creation (create-with-history composes the two API calls).
	let historyFiles: FileList | null = null;
	let importedProfileName = '';
	let newVehicle = {
		id: '',
		vehicle: '',
		registration: '',
		has_plan: true,
		start_date: '',
		end_date: '',
		annual_allowance: 10000,
		start_miles: 0,
		excess_rate: 0
	};

	$: normalizedNewVehicleId = newVehicle.id.toLowerCase().replace(/\s+/g, '_');
	$: duplicateVehicleId = normalizedNewVehicleId
		? vehicles.some((vehicle) => vehicle.id === normalizedNewVehicleId)
		: false;

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

	// Currency preference. The select binds to a local copy so an in-flight
	// save can't fight the global store; the store updates on success. The
	// local copy re-syncs whenever the store value changes (the boot fetch can
	// land after this page mounts) without clobbering an unsaved user edit.
	let currencyChoice = $settings.currency;
	let lastStoreCurrency = $settings.currency;
	let savingCurrency = false;
	$: if ($settings.currency !== lastStoreCurrency) {
		lastStoreCurrency = $settings.currency;
		currencyChoice = $settings.currency;
	}

	async function handleSaveCurrency() {
		savingCurrency = true;
		error = '';
		try {
			const saved = await updateSettings({ currency: currencyChoice });
			settings.set(saved);
			success = 'Currency updated.';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to update currency';
		} finally {
			savingCurrency = false;
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
			// Pull each vehicle's current excess rate so the inline editor prefills,
			// plus reminder settings in hosted mode.
			const rates: Record<string, number> = {};
			const statuses: Record<string, VehicleStatus> = {};
			const reminders: Record<string, ReminderSettings> = {};
			const hosted = $mode === 'hosted';
			await Promise.all(
				vehicles.map(async (v) => {
					try {
						const s = await getVehicle(v.id);
						statuses[v.id] = s;
						rates[v.id] = s.excess_rate ?? 0;
					} catch {
						rates[v.id] = 0;
					}
					if (hosted) {
						try {
							reminders[v.id] = await getReminderSettings(v.id);
						} catch {
							reminders[v.id] = { user_id: '', vehicle_id: v.id, enabled: false, frequency: 'weekly' };
						}
					}
				})
			);
			excessRates = rates;
			vehicleStatuses = statuses;
			reminderSettings = reminders;
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

	async function handleSaveReminder(id: string) {
		const r = reminderSettings[id];
		if (!r) return;
		if (r.frequency === 'custom' && (!r.custom_interval || r.custom_interval < 1)) {
			error = 'Custom reminders need an interval of at least 1';
			return;
		}
		savingReminder = { ...savingReminder, [id]: true };
		error = '';
		try {
			const saved = await updateReminderSettings(id, {
				enabled: r.enabled,
				frequency: r.frequency,
				custom_interval: r.frequency === 'custom' ? r.custom_interval : undefined,
				custom_unit: r.frequency === 'custom' ? (r.custom_unit ?? 'days') : undefined
			});
			reminderSettings = { ...reminderSettings, [id]: saved };
			success = `Reminders updated for ${id}.`;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to update reminders';
		} finally {
			savingReminder = { ...savingReminder, [id]: false };
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

	function isObject(value: unknown): value is Record<string, unknown> {
		return typeof value === 'object' && value !== null && !Array.isArray(value);
	}

	function requireString(value: unknown, field: string): string {
		if (typeof value !== 'string' || value.trim() === '') {
			throw new Error(`${field} must be a non-empty string`);
		}
		return value;
	}

	function requireDate(value: unknown, field: string): string {
		const date = requireString(value, field);
		if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) {
			throw new Error(`${field} must be a YYYY-MM-DD date`);
		}
		return date;
	}

	function requireNonNegativeInteger(value: unknown, field: string): number {
		if (typeof value !== 'number' || !Number.isInteger(value) || value < 0) {
			throw new Error(`${field} must be a non-negative integer`);
		}
		return value;
	}

	function requirePositiveInteger(value: unknown, field: string): number {
		const valueNumber = requireNonNegativeInteger(value, field);
		if (valueNumber <= 0) {
			throw new Error(`${field} must be greater than 0`);
		}
		return valueNumber;
	}

	function parseVehicleProfile(value: unknown): VehicleProfile {
		if (!isObject(value)) {
			throw new Error('Profile JSON must be an object');
		}
		const profile: VehicleProfile = {
			id: requireString(value.id, 'id'),
			vehicle: typeof value.vehicle === 'string' ? value.vehicle : ''
		};
		if (value.registration !== undefined) {
			profile.registration = requireString(value.registration, 'registration');
		}

		if (value.plan !== undefined) {
			if (!isObject(value.plan)) {
				throw new Error('plan must be an object');
			}
			profile.plan = {
				start: requireDate(value.plan.start, 'plan.start'),
				end: requireDate(value.plan.end, 'plan.end'),
				annual_allowance: requirePositiveInteger(value.plan.annual_allowance, 'plan.annual_allowance'),
				start_miles: requireNonNegativeInteger(value.plan.start_miles, 'plan.start_miles'),
				excess_rate: value.plan.excess_rate === undefined
					? 0
					: requireNonNegativeInteger(value.plan.excess_rate, 'plan.excess_rate')
			};
		}

		return profile;
	}

	async function handleProfileImport(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;

		error = '';
		success = '';
		try {
			const profile = parseVehicleProfile(JSON.parse(await file.text()));
			newVehicle = {
				id: profile.id,
				vehicle: profile.vehicle || profile.id,
				registration: profile.registration ?? '',
				has_plan: !!profile.plan,
				start_date: profile.plan?.start ?? '',
				end_date: profile.plan?.end ?? '',
				annual_allowance: profile.plan?.annual_allowance ?? 10000,
				start_miles: profile.plan?.start_miles ?? 0,
				excess_rate: profile.plan?.excess_rate ?? 0
			};
			importedProfileName = file.name;
			success = `Profile "${file.name}" loaded. Review and edit the fields before creating the vehicle.`;
		} catch (e) {
			importedProfileName = '';
			error = e instanceof Error ? `Invalid profile JSON: ${e.message}` : 'Invalid profile JSON';
		} finally {
			input.value = '';
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
				registration: newVehicle.registration.trim() || undefined,
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

			// Optional history import, composed after create. The vehicle exists
			// even if this fails, so the error message must say so.
			const historyFile = historyFiles?.[0];
			if (historyFile) {
				try {
					const result = await importCSV(payload.id, await historyFile.text());
					success = `Vehicle "${newVehicle.vehicle || newVehicle.id}" created and ${result.added} historical reading${result.added === 1 ? '' : 's'} imported.`;
				} catch (e) {
					success = '';
					error = `Vehicle created, but history import failed: ${e instanceof Error ? e.message : 'unknown error'}. You can retry from the History page.`;
				}
			}

			showAddForm = false;
			historyFiles = null;
			importedProfileName = '';
			newVehicle = {
				id: '',
				vehicle: '',
				registration: '',
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

	// Edit details (#40): display name + registration on an existing vehicle.
	let showEditForm: Record<string, boolean> = {};
	let editForms: Record<string, { vehicle: string; registration: string }> = {};
	let savingEdit: Record<string, boolean> = {};

	function toggleEditForm(id: string) {
		showEditForm = { ...showEditForm, [id]: !showEditForm[id] };
		if (showEditForm[id]) {
			editForms = {
				...editForms,
				[id]: {
					vehicle: vehicles.find((v) => v.id === id)?.vehicle ?? '',
					registration: vehicleStatuses[id]?.registration ?? ''
				}
			};
		}
	}

	async function handleSaveDetails(id: string) {
		const form = editForms[id];
		if (!form) return;
		savingEdit = { ...savingEdit, [id]: true };
		error = '';
		try {
			await updatePlan(id, {
				vehicle: form.vehicle.trim() || id,
				registration: form.registration.trim()
			});
			success = `Details updated for ${id}.`;
			showEditForm = { ...showEditForm, [id]: false };
			await loadVehicles();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to update details';
		} finally {
			savingEdit = { ...savingEdit, [id]: false };
		}
	}

	function resetForm() {
		showAddForm = false;
		error = '';
		historyFiles = null;
		importedProfileName = '';
		newVehicle = {
			id: '',
			vehicle: '',
			registration: '',
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

<div class="p-4 sm:p-6 lg:p-8 max-w-3xl mx-auto">
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
			<div class="flex flex-col gap-4 p-4 bg-carbon-900/40 border border-carbon-800 rounded-xl sm:flex-row sm:items-center sm:justify-between">
				<div class="min-w-0">
					<p class="text-sm text-carbon-400">Signed in as</p>
					<p class="truncate text-carbon-100 font-medium">{$user?.email ?? '—'}</p>
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
				<div class="mt-4 flex justify-stretch sm:justify-end">
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
					<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
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

	<!-- Preferences Section -->
	<section class="mb-8">
		<h2 class="text-xl font-semibold text-carbon-100 mb-4">Preferences</h2>
		<div class="p-4 bg-carbon-900/40 border border-carbon-800 rounded-xl">
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
				<div>
					<label for="currency" class="label">Currency</label>
					<select id="currency" bind:value={currencyChoice} class="input">
						{#each SUPPORTED_CURRENCIES as c}
							<option value={c.code}>{c.label}</option>
						{/each}
					</select>
					<p class="text-xs text-carbon-500 mt-1">
						Excess rates are entered in {minorUnitLabel(currencyChoice)}; overage costs display in this currency.
					</p>
				</div>
				<div>
					<span class="label">Distance unit</span>
					<p class="input bg-carbon-900/60 text-carbon-400 cursor-default select-none">Miles</p>
					<p class="text-xs text-carbon-500 mt-1">Kilometre support is planned.</p>
				</div>
			</div>
			<div class="mt-4 flex justify-stretch sm:justify-end">
				<button class="btn-secondary" on:click={handleSaveCurrency} disabled={savingCurrency || currencyChoice === $settings.currency}>
					{savingCurrency ? 'Saving...' : 'Save'}
				</button>
			</div>
		</div>
	</section>

	<!-- Vehicles Section -->
	<section class="mb-8">
		<div class="flex flex-col gap-3 mb-4 sm:flex-row sm:items-center sm:justify-between">
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
							<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
								<div class="flex min-w-0 items-center gap-4">
									<div class="w-10 h-10 shrink-0 rounded-lg bg-accent-primary/20 flex items-center justify-center">
										<svg class="w-5 h-5 text-accent-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
										</svg>
									</div>
									<div class="min-w-0">
										<p class="truncate font-medium text-carbon-100">{vehicle.vehicle || vehicle.id}</p>
										<p class="truncate text-sm text-carbon-500">{vehicleStatuses[vehicle.id]?.registration ? `${vehicleStatuses[vehicle.id].registration} · ${vehicle.id}` : vehicle.id}</p>
									</div>
								</div>
								<div class="flex items-center gap-2">
									<button class="btn-secondary text-sm" on:click={() => toggleEditForm(vehicle.id)}>
										{showEditForm[vehicle.id] ? 'Cancel' : 'Edit details'}
									</button>
									<a
										href={getProfileExportURL(vehicle.id)}
										download="{vehicle.id}_profile.json"
										class="btn-secondary text-sm"
									>
										Export profile
									</a>
									{#if vehicle.is_default}
										<span class="px-3 py-1 text-xs font-medium bg-accent-primary/20 text-accent-primary rounded-full">
											Default
										</span>
									{/if}
								</div>
							</div>

							{#if showEditForm[vehicle.id] && editForms[vehicle.id]}
								<form on:submit|preventDefault={() => handleSaveDetails(vehicle.id)} class="mt-4 pt-4 border-t border-carbon-800 space-y-4">
									<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
										<div>
											<label for="editName-{vehicle.id}" class="label">Display Name</label>
											<input id="editName-{vehicle.id}" type="text" bind:value={editForms[vehicle.id].vehicle} class="input" placeholder={vehicle.id} />
										</div>
										<div>
											<label for="editReg-{vehicle.id}" class="label">Registration</label>
											<input id="editReg-{vehicle.id}" type="text" bind:value={editForms[vehicle.id].registration} class="input" placeholder="e.g., AB12 CDE" />
										</div>
									</div>
									<button type="submit" class="btn-primary" disabled={savingEdit[vehicle.id]}>
										{savingEdit[vehicle.id] ? 'Saving...' : 'Save details'}
									</button>
								</form>
							{/if}

							{#if vehicleStatuses[vehicle.id]?.has_plan}
								<!-- Excess-rate editor (#5): settable on existing policy vehicles -->
								<div class="mt-4 pt-4 border-t border-carbon-800 flex flex-col gap-3 sm:flex-row sm:items-end">
									<div class="flex-1">
										<label for="rate-{vehicle.id}" class="label">Excess Rate ({minorUnitLabel($settings.currency)} per mile over)</label>
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
									<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
										<p class="text-sm text-carbon-400">No allowance policy</p>
										<button class="btn-secondary text-sm" on:click={() => togglePlanForm(vehicle.id)}>
											{showPlanForm[vehicle.id] ? 'Cancel' : 'Add allowance plan'}
										</button>
									</div>
									{#if showPlanForm[vehicle.id] && planForms[vehicle.id]}
										<form on:submit|preventDefault={() => handleAddPlan(vehicle.id)} class="mt-4 space-y-4">
											<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
												<div>
													<label for="planStart-{vehicle.id}" class="label">Plan Start Date *</label>
													<input id="planStart-{vehicle.id}" type="date" bind:value={planForms[vehicle.id].start_date} class="input" required />
												</div>
												<div>
													<label for="planEnd-{vehicle.id}" class="label">Plan End Date *</label>
													<input id="planEnd-{vehicle.id}" type="date" bind:value={planForms[vehicle.id].end_date} class="input" required />
												</div>
											</div>
											<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
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
												<label for="planRate-{vehicle.id}" class="label">Excess Rate ({minorUnitLabel($settings.currency)} per mile over)</label>
												<input id="planRate-{vehicle.id}" type="number" bind:value={planForms[vehicle.id].excess_rate} class="input font-mono" min="0" step="1" />
											</div>
											<button type="submit" class="btn-primary" disabled={savingPlan[vehicle.id]}>
												{savingPlan[vehicle.id] ? 'Saving...' : 'Save allowance plan'}
											</button>
										</form>
									{/if}
								</div>
							{/if}

							{#if $mode === 'hosted' && reminderSettings[vehicle.id]}
								<!-- Reading reminders (#52): time-based nudge to log a reading -->
								<div class="mt-4 pt-4 border-t border-carbon-800">
									<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
										<label class="flex items-center gap-3">
											<input type="checkbox" bind:checked={reminderSettings[vehicle.id].enabled} class="w-4 h-4 accent-accent-primary" />
											<span class="text-carbon-100 font-medium">Email me to log a reading</span>
										</label>
										<button class="btn-secondary text-sm" on:click={() => handleSaveReminder(vehicle.id)} disabled={savingReminder[vehicle.id]}>
											{savingReminder[vehicle.id] ? 'Saving...' : 'Save reminders'}
										</button>
									</div>
									<div class="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
										<div>
											<label for="reminderFreq-{vehicle.id}" class="label">Frequency</label>
											<select id="reminderFreq-{vehicle.id}" bind:value={reminderSettings[vehicle.id].frequency} class="input" disabled={!reminderSettings[vehicle.id].enabled}>
												<option value="daily">Daily</option>
												<option value="weekly">Weekly</option>
												<option value="quarterly">Quarterly</option>
												<option value="custom">Custom</option>
											</select>
										</div>
										{#if reminderSettings[vehicle.id].frequency === 'custom'}
											<div class="grid grid-cols-2 gap-2 items-end">
												<div>
													<label for="reminderInterval-{vehicle.id}" class="label">Every</label>
													<input id="reminderInterval-{vehicle.id}" type="number" min="1" step="1" bind:value={reminderSettings[vehicle.id].custom_interval} class="input font-mono" disabled={!reminderSettings[vehicle.id].enabled} />
												</div>
												<div>
													<label for="reminderUnit-{vehicle.id}" class="label">Unit</label>
													<select id="reminderUnit-{vehicle.id}" bind:value={reminderSettings[vehicle.id].custom_unit} class="input" disabled={!reminderSettings[vehicle.id].enabled}>
														<option value="days">Days</option>
														<option value="weeks">Weeks</option>
														<option value="months">Months</option>
													</select>
												</div>
											</div>
										{/if}
									</div>
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
					<div class="flex items-center justify-between gap-3 mb-6">
						<h3 class="text-lg font-semibold text-carbon-100">Add New Vehicle</h3>
						<button class="btn-ghost text-sm" on:click={resetForm}>Cancel</button>
					</div>

					{#if error}
						<div class="mb-4 p-4 bg-gauge-red/10 border border-gauge-red/30 rounded-lg">
							<p class="text-gauge-red text-sm">{error}</p>
						</div>
					{/if}

					<form on:submit|preventDefault={handleCreateVehicle} class="space-y-6">
						<div class="p-4 bg-carbon-900/40 border border-carbon-800 rounded-lg">
							<label for="profileJson" class="label">Import profile (JSON)</label>
							<input
								type="file"
								id="profileJson"
								accept=".json,application/json"
								on:change={handleProfileImport}
								class="input"
							/>
							{#if importedProfileName}
								<p class="text-xs text-gauge-green mt-2">{importedProfileName} loaded into the form</p>
							{:else}
								<p class="text-xs text-carbon-500 mt-2">Loads vehicle details into this form only. Nothing is created until you click Create Vehicle.</p>
							{/if}
						</div>

						<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
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
								{#if duplicateVehicleId}
									<p class="text-xs text-gauge-amber mt-1">A vehicle with this ID already exists. Edit it before creating to avoid replacing the existing vehicle.</p>
								{:else}
									<p class="text-xs text-carbon-500 mt-1">Used for CLI commands</p>
								{/if}
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
							<div>
								<label for="vehicleReg" class="label">Registration</label>
								<input
									type="text"
									id="vehicleReg"
									bind:value={newVehicle.registration}
									placeholder="e.g., AB12 CDE"
									class="input"
								/>
								<p class="text-xs text-carbon-500 mt-1">Optional — shown alongside the name to tell similar cars apart</p>
							</div>
						</div>

						<label class="flex items-center gap-3 p-4 bg-carbon-900/40 border border-carbon-800 rounded-lg">
							<input type="checkbox" bind:checked={newVehicle.has_plan} class="w-4 h-4 accent-accent-primary" />
							<span class="text-sm text-carbon-200">Has a mileage allowance (PCP/lease/insurance)</span>
						</label>

						<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
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

						<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
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

						<div>
							<label for="historyCsv" class="label">Import history (CSV, optional)</label>
							<input
								type="file"
								id="historyCsv"
								accept=".csv,text/csv"
								bind:files={historyFiles}
								class="input"
							/>
							<p class="text-xs text-carbon-500 mt-1">Readings in the export format (date,miles) are imported right after the vehicle is created</p>
						</div>

						{#if newVehicle.has_plan}
							<div>
								<label for="excessRate" class="label">Excess Rate ({minorUnitLabel($settings.currency)} per mile over)</label>
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

						<div class="flex flex-col gap-3 pt-4 sm:flex-row">
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
		<div class="mt-6 pt-4 border-t border-carbon-800 flex flex-col gap-3 text-sm sm:flex-row sm:items-center sm:justify-between">
			<span class="text-carbon-500">Built with Go + Svelte</span>
			<a href="https://github.com/JackIABishop/MileMinder" target="_blank" rel="noopener" class="text-accent-primary hover:underline">
				GitHub →
			</a>
		</div>
	</section>
</div>
