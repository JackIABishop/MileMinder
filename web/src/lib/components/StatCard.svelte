<script lang="ts">
	export let title: string;
	export let value: string | number;
	export let unit: string = '';
	export let subtitle: string = '';
	export let trend: 'up' | 'down' | 'neutral' | null = null;
	export let trendValue: string = '';
	export let color: 'default' | 'green' | 'amber' | 'red' = 'default';

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
		<h3 class="text-sm font-medium text-carbon-400">{title}</h3>
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
