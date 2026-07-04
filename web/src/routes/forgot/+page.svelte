<script lang="ts">
	import { forgotPassword } from '$lib/api';

	let email = '';
	let error = '';
	let submitted = false;
	let submitting = false;

	async function submit() {
		error = '';
		submitting = true;
		try {
			await forgotPassword(email);
			submitted = true;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Something went wrong';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Forgot password | MileMinder</title>
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
			<h2 class="text-lg font-semibold text-carbon-100 mb-1">Reset your password</h2>
			<p class="text-sm text-carbon-500 mb-6">Enter your account email.</p>

			{#if submitted}
				<p class="text-sm text-carbon-200 mb-6">If that email has an account, a reset link is on its way.</p>
				<a href="/login" class="block w-full text-center py-2 rounded-xl bg-accent-primary text-carbon-950 font-medium hover:opacity-90 transition-opacity">
					Back to sign in
				</a>
			{:else}
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

					{#if error}
						<p class="text-sm text-red-400">{error}</p>
					{/if}

					<button
						type="submit"
						disabled={submitting}
						class="w-full py-2 rounded-xl bg-accent-primary text-carbon-950 font-medium hover:opacity-90 transition-opacity disabled:opacity-50"
					>
						{submitting ? 'Please wait...' : 'Send reset link'}
					</button>
				</form>

				<a href="/login" class="block mt-4 text-center text-sm text-carbon-400 hover:text-carbon-200 transition-colors">
					Back to sign in
				</a>
			{/if}
		</div>
	</div>
</div>
