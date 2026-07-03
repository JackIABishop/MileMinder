<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { login, signup, initAuth, mode, user, authReady } from '$lib/auth';

	let isSignup = false;
	let email = '';
	let password = '';
	let error = '';
	let submitting = false;

	onMount(async () => {
		if (!$authReady) await initAuth();
		// A single-user server has no login; nobody should be here.
		if ($mode === 'single-user' || $user) {
			goto('/');
		}
	});

	async function submit() {
		error = '';
		submitting = true;
		try {
			if (isSignup) {
				await signup(email, password);
			} else {
				await login(email, password);
			}
			goto('/');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Something went wrong';
		} finally {
			submitting = false;
		}
	}

	function toggleMode() {
		isSignup = !isSignup;
		error = '';
	}
</script>

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
			<h2 class="text-lg font-semibold text-carbon-100 mb-1">
				{isSignup ? 'Create your account' : 'Sign in'}
			</h2>
			<p class="text-sm text-carbon-500 mb-6">
				{isSignup ? 'Sync and access your mileage on the web.' : 'Welcome back.'}
			</p>

			<form on:submit|preventDefault={submit} class="space-y-4">
				<div>
					<label for="email" class="block text-sm text-carbon-400 mb-1">Email</label>
					<input
						id="email"
						type="email"
						bind:value={email}
						autocomplete="email"
						required
						class="w-full px-3 py-2 rounded-xl bg-carbon-800/50 border border-carbon-700 text-carbon-100 focus:outline-none focus:border-accent-primary"
					/>
				</div>
				<div>
					<label for="password" class="block text-sm text-carbon-400 mb-1">Password</label>
					<input
						id="password"
						type="password"
						bind:value={password}
						autocomplete={isSignup ? 'new-password' : 'current-password'}
						required
						minlength={isSignup ? 8 : undefined}
						class="w-full px-3 py-2 rounded-xl bg-carbon-800/50 border border-carbon-700 text-carbon-100 focus:outline-none focus:border-accent-primary"
					/>
					{#if isSignup}
						<p class="text-xs text-carbon-600 mt-1">At least 8 characters.</p>
					{/if}
				</div>

				{#if error}
					<p class="text-sm text-red-400">{error}</p>
				{/if}

				<button
					type="submit"
					disabled={submitting}
					class="w-full py-2 rounded-xl bg-accent-primary text-carbon-950 font-medium hover:opacity-90 transition-opacity disabled:opacity-50"
				>
					{submitting ? 'Please wait…' : isSignup ? 'Create account' : 'Sign in'}
				</button>
			</form>

			{#if !isSignup}
				<a href="/forgot" class="block mt-4 text-center text-sm text-carbon-400 hover:text-carbon-200 transition-colors">
					Forgot password?
				</a>
			{/if}

			<button
				type="button"
				on:click={toggleMode}
				class="w-full mt-4 text-sm text-carbon-400 hover:text-carbon-200 transition-colors"
			>
				{isSignup ? 'Already have an account? Sign in' : "Don't have an account? Create one"}
			</button>
		</div>
	</div>
</div>
