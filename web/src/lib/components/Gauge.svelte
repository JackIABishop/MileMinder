<script lang="ts">
	import { onMount } from 'svelte';

	export let percent: number = 0;
	export let size: number = 200;
	export let strokeWidth: number = 12;

	$: radius = (size - strokeWidth) / 2;
	$: circumference = 2 * Math.PI * radius;
	$: clampedPercent = Math.min(Math.max(percent, 0), 150);
	$: offset = circumference - (clampedPercent / 100) * circumference;

	$: strokeColor = percent <= 90 ? '#22c55e' : percent <= 100 ? '#f59e0b' : '#ef4444';
	$: glowColor = percent <= 90 ? 'rgba(34, 197, 94, 0.5)' : percent <= 100 ? 'rgba(245, 158, 11, 0.5)' : 'rgba(239, 68, 68, 0.5)';

	let mounted = false;
	onMount(() => {
		setTimeout(() => mounted = true, 100);
	});
</script>

<div class="relative inline-flex items-center justify-center" style="width: {size}px; height: {size}px;">
	<svg class="transform -rotate-90" width={size} height={size}>
		<!-- Background ring -->
		<circle
			class="gauge-ring gauge-bg"
			cx={size / 2}
			cy={size / 2}
			r={radius}
			stroke-width={strokeWidth}
		/>
		<!-- Filled ring -->
		<circle
			class="gauge-ring transition-all duration-1000 ease-out"
			cx={size / 2}
			cy={size / 2}
			r={radius}
			stroke-width={strokeWidth}
			stroke-dasharray={circumference}
			stroke-dashoffset={mounted ? offset : circumference}
			stroke={strokeColor}
			style="filter: drop-shadow(0 0 8px {glowColor});"
		/>
	</svg>
	
	<!-- Center content -->
	<div class="absolute inset-0 flex flex-col items-center justify-center">
		<slot />
	</div>
</div>
