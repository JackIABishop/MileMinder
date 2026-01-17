<script lang="ts">
	import { onMount } from 'svelte';
	import { getCurrentVehicle, getVehicle, getGraphData, formatNumber, formatDate, type VehicleStatus, type GraphData } from '$lib/api';
	import { Chart, registerables } from 'chart.js';

	Chart.register(...registerables);

	let status: VehicleStatus | null = null;
	let graphData: GraphData | null = null;
	let loading = true;
	let error = '';
	let chartCanvas: HTMLCanvasElement;
	let chart: Chart | null = null;

	onMount(async () => {
		try {
			const current = await getCurrentVehicle();
			if (current.current) {
				status = await getVehicle(current.current);
				graphData = await getGraphData(current.current);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load data';
		} finally {
			loading = false;
		}
	});

	// Render chart when canvas becomes available and we have data
	$: if (chartCanvas && graphData && !chart) {
		renderChart();
	}

	async function renderChart() {
		if (!graphData || !chartCanvas) return;

		// Destroy existing chart if any
		if (chart) {
			chart.destroy();
		}

		const ctx = chartCanvas.getContext('2d');
		if (!ctx) return;

		// Format dates for display
		const labels = graphData.dates.map(d => {
			const date = new Date(d);
			return date.toLocaleDateString('en-GB', { day: '2-digit', month: 'short' });
		});

		chart = new Chart(ctx, {
			type: 'line',
			data: {
				labels,
				datasets: [
					{
						label: 'Actual Miles',
						data: graphData.actuals,
						borderColor: '#22c55e',
						backgroundColor: 'rgba(34, 197, 94, 0.1)',
						fill: true,
						tension: 0.3,
						pointBackgroundColor: '#22c55e',
						pointBorderColor: '#22c55e',
						pointRadius: 4,
						pointHoverRadius: 6
					},
					{
						label: 'Allowance Limit',
						data: graphData.ideals,
						borderColor: '#60a5fa',
						backgroundColor: 'transparent',
						borderDash: [5, 5],
						tension: 0.1,
						pointRadius: 0,
						pointHoverRadius: 4
					}
				]
			},
			options: {
				responsive: true,
				maintainAspectRatio: false,
				interaction: {
					intersect: false,
					mode: 'index'
				},
				plugins: {
					legend: {
						position: 'top',
						labels: {
							color: '#b8b8c1',
							font: {
								family: 'Outfit'
							},
							padding: 20,
							usePointStyle: true
						}
					},
					tooltip: {
						backgroundColor: '#1a1a1f',
						titleColor: '#f7f7f8',
						bodyColor: '#b8b8c1',
						borderColor: '#42424b',
						borderWidth: 1,
						padding: 12,
						titleFont: {
							family: 'Outfit',
							weight: '600'
						},
						bodyFont: {
							family: 'JetBrains Mono'
						},
						callbacks: {
							label: function(context) {
								return `${context.dataset.label}: ${formatNumber(Math.round(context.parsed.y))} mi`;
							}
						}
					}
				},
				scales: {
					x: {
						grid: {
							color: 'rgba(66, 66, 75, 0.3)'
						},
						ticks: {
							color: '#91919f',
							font: {
								family: 'Outfit'
							}
						}
					},
					y: {
						grid: {
							color: 'rgba(66, 66, 75, 0.3)'
						},
						ticks: {
							color: '#91919f',
							font: {
								family: 'JetBrains Mono'
							},
							callback: function(value) {
								return formatNumber(value as number);
							}
						}
					}
				}
			}
		});
	}
</script>

<svelte:head>
	<title>Graph | MileMinder</title>
</svelte:head>

<div class="p-8">
	<header class="mb-8 animate-fade-in">
		<h1 class="text-3xl font-display font-bold text-carbon-100">Mileage Graph</h1>
		<p class="text-carbon-500 mt-2">Track your actual usage against the ideal allowance</p>
	</header>

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="animate-pulse text-carbon-400">Loading...</div>
		</div>
	{:else if error}
		<div class="card border-gauge-red/30 bg-gauge-red/5">
			<p class="text-gauge-red">{error}</p>
		</div>
	{:else if status && graphData}
		<!-- Legend Info -->
		<div class="grid grid-cols-2 gap-4 mb-6">
			<div class="card animate-slide-up">
				<div class="flex items-center gap-3">
					<div class="w-4 h-4 rounded-full bg-gauge-green"></div>
					<div>
						<p class="text-sm text-carbon-400">Actual Usage</p>
						<p class="text-lg font-mono font-semibold text-carbon-100">
							{formatNumber(graphData.actuals[graphData.actuals.length - 1] || 0)} mi
						</p>
					</div>
				</div>
			</div>
			<div class="card animate-slide-up stagger-1">
				<div class="flex items-center gap-3">
					<div class="w-4 h-4 rounded-full bg-accent-primary"></div>
					<div>
						<p class="text-sm text-carbon-400">Allowance Limit</p>
						<p class="text-lg font-mono font-semibold text-carbon-100">
							{formatNumber(graphData.ideals[graphData.ideals.length - 1] || 0)} mi
						</p>
					</div>
				</div>
			</div>
		</div>

		<!-- Chart -->
		<div class="card animate-slide-up stagger-2">
			<div class="flex items-center justify-between mb-4">
				<h2 class="text-lg font-semibold text-carbon-100">{status.vehicle || status.id}</h2>
				<p class="text-sm text-carbon-500">
					{formatDate(status.plan_start)} → {formatDate(status.plan_end)}
				</p>
			</div>
			<div class="h-[400px]">
				<canvas bind:this={chartCanvas}></canvas>
			</div>
		</div>

		<!-- Stats Summary -->
		<div class="grid grid-cols-3 gap-4 mt-6">
			<div class="card animate-slide-up stagger-3 text-center">
				<p class="text-sm text-carbon-400 mb-1">Total Readings</p>
				<p class="text-2xl font-mono font-bold text-carbon-100">{graphData.dates.length}</p>
			</div>
			<div class="card animate-slide-up stagger-4 text-center">
				<p class="text-sm text-carbon-400 mb-1">Difference</p>
				<p class="text-2xl font-mono font-bold {status.delta <= 0 ? 'text-gauge-green' : 'text-gauge-red'}">
					{status.delta > 0 ? '+' : ''}{formatNumber(Math.round(status.delta))} mi
				</p>
			</div>
			<div class="card animate-slide-up stagger-5 text-center">
				<p class="text-sm text-carbon-400 mb-1">Daily Rate</p>
				<p class="text-2xl font-mono font-bold text-carbon-100">{formatNumber(status.daily_rate, 1)} mi</p>
			</div>
		</div>
	{:else}
		<div class="card">
			<p class="text-carbon-400">No vehicle selected. Please select a vehicle to view the graph.</p>
		</div>
	{/if}
</div>
