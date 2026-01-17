<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	
	export let currentOdometer: number = 0;
	
	const dispatch = createEventDispatcher();
	
	const quickAmounts = [10, 25, 50, 100];
	
	function addQuick(amount: number) {
		dispatch('add', { miles: currentOdometer + amount });
	}
</script>

<div class="flex flex-wrap gap-2">
	{#each quickAmounts as amount}
		<button
			class="btn-secondary text-sm flex items-center gap-1"
			on:click={() => addQuick(amount)}
		>
			<span class="text-accent-primary">+{amount}</span>
			<span class="text-carbon-400">mi</span>
		</button>
	{/each}
</div>
