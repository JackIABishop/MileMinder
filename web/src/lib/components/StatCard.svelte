<script lang="ts">
	import { onMount } from 'svelte';

	export let title: string;
	export let value: string | number;
	export let unit: string = '';
	export let subtitle: string = '';
	export let tooltip: string = '';
	export let trend: 'up' | 'down' | 'neutral' | null = null;
	export let trendValue: string = '';
	export let color: 'default' | 'green' | 'amber' | 'red' = 'default';

	let tooltipOpen = false;

	onMount(() => {
		const closeOnOutsideClick = () => (tooltipOpen = false);
		window.addEventListener('click', closeOnOutsideClick);
		return () => window.removeEventListener('click', closeOnOutsideClick);
	});

	const colorClasses = {
		default: 'text-carbon-100',
		green: 'text-gauge-green',
		amber: 'text-gauge-amber',
		red: 'text-gauge-red'
	};

	const trendIcons = {
		up: '↑',
		down: '↓',
		neutral: '→'
	};
</script>

<div class="card animate-slide-up">
	<div class="flex items-start justify-between mb-3">
		<h3 class="flex items-center gap-1.5 text-sm font-medium text-carbon-400">
			{title}
			{#if tooltip}
				<span class="relative inline-flex">
					<button
						type="button"
						aria-label={tooltip}
						aria-expanded={tooltipOpen}
						class="inline-flex h-3.5 w-3.5 cursor-help items-center justify-center rounded-full border border-carbon-600 text-[10px] leading-none text-carbon-500 hover:border-carbon-400 hover:text-carbon-300"
						on:click|stopPropagation={() => (tooltipOpen = !tooltipOpen)}
						on:mouseenter={() => (tooltipOpen = true)}
						on:mouseleave={() => (tooltipOpen = false)}
					>i</button>
					{#if tooltipOpen}
						<span
							role="tooltip"
							class="absolute left-1/2 top-full z-10 mt-1.5 w-48 -translate-x-1/2 rounded-md border border-carbon-700 bg-carbon-800 px-2.5 py-1.5 text-xs font-normal normal-case text-carbon-200 shadow-lg"
						>{tooltip}</span>
					{/if}
				</span>
			{/if}
		</h3>
		{#if trend}
			<span class="flex items-center gap-1 text-xs {trend === 'up' ? 'text-gauge-red' : trend === 'down' ? 'text-gauge-green' : 'text-carbon-400'}">
				<span>{trendIcons[trend]}</span>
				<span>{trendValue}</span>
			</span>
		{/if}
	</div>
	
	<div class="flex items-baseline gap-1">
		<span class="number-display {colorClasses[color]}">{value}</span>
		{#if unit}
			<span class="number-unit">{unit}</span>
		{/if}
	</div>
	
	{#if subtitle}
		<p class="mt-2 text-sm text-carbon-500">{subtitle}</p>
	{/if}

	{#if $$slots.default}
		<div class="mt-4">
			<slot />
		</div>
	{/if}
</div>
