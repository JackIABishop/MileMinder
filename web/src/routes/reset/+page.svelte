<script lang="ts">
	import { page } from '$app/stores';
	import { resetPassword } from '$lib/api';

	$: token = $page.url.searchParams.get('token') ?? '';

	let newPassword = '';
	let confirmPassword = '';
	let error = '';
	let success = false;
	let submitting = false;

	async function submit() {
		error = '';
		if (!token) {
			error = 'Invalid or expired reset link';
			return;
		}
		if (newPassword.length < 8) {
			error = 'Password must be at least 8 characters';
			return;
		}
		if (newPassword !== confirmPassword) {
			error = 'Passwords do not match';
			return;
		}
		submitting = true;
		try {
			await resetPassword(token, newPassword);
			success = true;
			newPassword = '';
			confirmPassword = '';
		} catch {
			error = 'Invalid or expired reset link';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Reset password | MileMinder</title>
</svelte:head>

<div class="min-h-screen flex items-center justify-center p-6">
	<div class="w-full max-w-sm">
		<div class="flex items-center gap-3 mb-8 justify-center">
			<div class="w-10 h-10 rounded-xl bg-gradient-to-br from-accent-primary to-accent-secondary flex items-center justify-center">
				<svg class="w-6 h-6 text-carbon-950" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
				</svg>
			</div>
			<h1 class="font-display font-bold text-2xl text-carbon-100">MileMinder</h1>
		</div>

		<div class="bg-carbon-900/40 border border-carbon-800 rounded-2xl p-6">
			<h2 class="text-lg font-semibold text-carbon-100 mb-1">Choose a new password</h2>
			<p class="text-sm text-carbon-500 mb-6">Use at least 8 characters.</p>

			{#if success}
				<p class="text-sm text-carbon-200 mb-6">Your password has been reset.</p>
				<a href="/login" class="block w-full text-center py-2 rounded-xl bg-accent-primary text-carbon-950 font-medium hover:opacity-90 transition-opacity">
					Sign in
				</a>
			{:else}
				<form on:submit|preventDefault={submit} class="space-y-4">
					<div>
						<label for="newPassword" class="block text-sm text-carbon-400 mb-1">New password</label>
						<input
							id="newPassword"
							type="password"
							bind:value={newPassword}
							autocomplete="new-password"
							required
							minlength="8"
							class="w-full px-3 py-2 rounded-xl bg-carbon-800/50 border border-carbon-700 text-carbon-100 focus:outline-none focus:border-accent-primary"
						/>
					</div>
					<div>
						<label for="confirmPassword" class="block text-sm text-carbon-400 mb-1">Confirm password</label>
						<input
							id="confirmPassword"
							type="password"
							bind:value={confirmPassword}
							autocomplete="new-password"
							required
							minlength="8"
							class="w-full px-3 py-2 rounded-xl bg-carbon-800/50 border border-carbon-700 text-carbon-100 focus:outline-none focus:border-accent-primary"
						/>
					</div>

					{#if error}
						<p class="text-sm text-red-400">
							{error}. <a href="/forgot" class="underline hover:text-red-300">Request a new link</a>
						</p>
					{/if}

					<button
						type="submit"
						disabled={submitting}
						class="w-full py-2 rounded-xl bg-accent-primary text-carbon-950 font-medium hover:opacity-90 transition-opacity disabled:opacity-50"
					>
						{submitting ? 'Please wait...' : 'Reset password'}
					</button>
				</form>
			{/if}
		</div>
	</div>
</div>
