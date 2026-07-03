<script lang="ts">
	import { onMount } from 'svelte';
	import { getCurrentVehicle, getVehicle, getGraphData, formatNumber, formatDate, type VehicleStatus, type GraphData } from '$lib/api';
	import { Chart, registerables } from 'chart.js';
	import 'chartjs-adapter-date-fns';

	Chart.register(...registerables);

	let status: VehicleStatus | null = null;
	let graphData: GraphData | null = null;
	let loading = true;
	let error = '';
	let chartCanvas: HTMLCanvasElement;
	let chart: Chart | null = null;
	let showProjection = false;
	let showYears = false;

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

	const DAY_MS = 24 * 60 * 60 * 1000;

	// Whole days between two dates (positive if b is after a)
	function daysBetween(a: Date, b: Date): number {
		return (b.getTime() - a.getTime()) / DAY_MS;
	}

	// Ideal miles accrued by a given date: straight line from plan start.
	function idealAt(date: Date, start: Date, annual: number): number {
		const days = Math.max(0, daysBetween(start, date));
		return (annual * days) / 365;
	}

	// Sample a line into weekly points so it stays hoverable at any date
	// (a two-point line has nothing to snap to between its ends). Weekly
	// spacing is independent of plan length, so hover resolution is consistent.
	function sampleLine(fromMs: number, toMs: number, yAtMs: (ms: number) => number) {
		const stepMs = 7 * DAY_MS;
		const points: { x: number; y: number }[] = [];
		for (let t = fromMs; t < toMs; t += stepMs) {
			points.push({ x: t, y: yAtMs(t) });
		}
		points.push({ x: toMs, y: yAtMs(toMs) });
		return points;
	}

	async function renderChart() {
		if (!graphData || !status || !chartCanvas) return;

		// Destroy existing chart if any
		if (chart) {
			chart.destroy();
			chart = null;
		}

		const ctx = chartCanvas.getContext('2d');
		if (!ctx) return;

		const firstReadingDate = graphData.dates[0] ? new Date(graphData.dates[0]) : new Date();
		const start = status.has_plan ? new Date(status.plan_start) : firstReadingDate;
		const end = status.has_plan ? new Date(status.plan_end) : new Date();
		const today = new Date();
		const annual = status.annual_allowance;

		// Right edge of the time axis: today, or the plan end when projecting.
		const axisMax = status.has_plan && showProjection ? end : today;

		// Allowance-year intervals (start → start+1yr …), capped at the plan end.
		const yearIntervals: { start: number; end: number; index: number }[] = [];
		{
			let b = new Date(start);
			let idx = 0;
			while (b.getTime() < end.getTime()) {
				const next = new Date(b);
				next.setFullYear(next.getFullYear() + 1);
				yearIntervals.push({
					start: b.getTime(),
					end: Math.min(next.getTime(), end.getTime()),
					index: idx
				});
				b = next;
				idx++;
			}
		}

		// Inline plugin: shade alternating allowance years, draw boundary lines
		// and "Year N" labels. Gated on the showYears toggle.
		const yearBandsPlugin = {
			id: 'yearBands',
			beforeDatasetsDraw(c: Chart) {
				if (!showYears || !status?.has_plan) return;
				const { ctx, chartArea, scales } = c;
				const xs = scales.x;
				ctx.save();
				ctx.fillStyle = 'rgba(96, 165, 250, 0.06)';
				for (const iv of yearIntervals) {
					if (iv.index % 2 !== 0) continue;
					const l = Math.max(xs.getPixelForValue(iv.start), chartArea.left);
					const r = Math.min(xs.getPixelForValue(iv.end), chartArea.right);
					if (r > l) ctx.fillRect(l, chartArea.top, r - l, chartArea.bottom - chartArea.top);
				}
				ctx.restore();
			},
			afterDatasetsDraw(c: Chart) {
				if (!showYears || !status?.has_plan) return;
				const { ctx, chartArea, scales } = c;
				const xs = scales.x;
				ctx.save();
				// Boundary lines (skip the first — it sits on the axis edge).
				ctx.strokeStyle = 'rgba(146, 146, 159, 0.35)';
				ctx.lineWidth = 1;
				ctx.setLineDash([2, 4]);
				for (const iv of yearIntervals) {
					if (iv.index === 0) continue;
					const px = xs.getPixelForValue(iv.start);
					if (px < chartArea.left || px > chartArea.right) continue;
					ctx.beginPath();
					ctx.moveTo(px, chartArea.top);
					ctx.lineTo(px, chartArea.bottom);
					ctx.stroke();
				}
				ctx.setLineDash([]);
				// Year labels, centred in the visible portion of each band.
				ctx.fillStyle = '#91919f';
				ctx.font = '11px Outfit, sans-serif';
				ctx.textAlign = 'center';
				ctx.textBaseline = 'top';
				for (const iv of yearIntervals) {
					const l = Math.max(xs.getPixelForValue(iv.start), chartArea.left);
					const r = Math.min(xs.getPixelForValue(iv.end), chartArea.right);
					if (r - l < 36) continue;
					ctx.fillText(`Year ${iv.index + 1}`, (l + r) / 2, chartArea.top + 4);
				}
				ctx.restore();
			}
		};

		// Actual readings plotted at their true dates (miles since plan start).
		const actualPoints = graphData.dates.map((d, i) => ({
			x: new Date(d).getTime(),
			y: graphData!.actuals[i]
		}));

		const datasets: any[] = [
			{
				label: 'Actual Miles',
				data: actualPoints,
				borderColor: '#22c55e',
				backgroundColor: 'rgba(34, 197, 94, 0.1)',
				fill: true,
				tension: 0.3,
				pointBackgroundColor: '#22c55e',
				pointBorderColor: '#22c55e',
				pointRadius: 4,
				pointHoverRadius: 6
			}
		];
		if (status.has_plan) {
			// Ideal allowance line: a straight line from (start, 0) to the axis edge,
			// sampled weekly so it can be hovered at any date.
			const idealPoints = sampleLine(start.getTime(), axisMax.getTime(), (ms) =>
				idealAt(new Date(ms), start, annual)
			);
			datasets.push({
				label: 'Allowance Limit',
				data: idealPoints,
				borderColor: '#60a5fa',
				backgroundColor: 'transparent',
				borderDash: [5, 5],
				pointRadius: 0,
				pointHoverRadius: 4
			});
		}

		// Projection: extend the last reading to plan end at the current daily
		// rate, sampled weekly so each future date is hoverable.
		if (status.has_plan && showProjection && graphData.dates.length > 0) {
			const lastDate = new Date(graphData.dates[graphData.dates.length - 1]);
			const lastY = graphData.actuals[graphData.actuals.length - 1];
			const rate = status.daily_rate;
			datasets.push({
				label: 'Projected',
				data: sampleLine(lastDate.getTime(), end.getTime(), (ms) =>
					lastY + rate * ((ms - lastDate.getTime()) / DAY_MS)
				),
				borderColor: status.projected_over ? '#ef4444' : '#f59e0b',
				backgroundColor: 'transparent',
				borderDash: [3, 3],
				pointRadius: 0,
				pointHoverRadius: 4
			});
		}

		chart = new Chart(ctx, {
			type: 'line',
			data: { datasets },
			plugins: [yearBandsPlugin],
			options: {
				responsive: true,
				maintainAspectRatio: false,
				interaction: {
					intersect: false,
					mode: 'nearest',
					axis: 'x'
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
							title: function(items) {
								if (!items.length) return '';
								return new Date(items[0].parsed.x).toLocaleDateString('en-GB', {
									day: '2-digit',
									month: 'short',
									year: 'numeric'
								});
							},
							label: function(context) {
								return `${context.dataset.label}: ${formatNumber(Math.round(context.parsed.y))} mi`;
							},
							// For an actual/projected point, also show the allowance at
							// that date and how far above/below the line it sits.
							afterBody: function(items) {
								if (!status?.has_plan) return [];
								if (!items.length) return [];
								const item = items[0];
								if (item.dataset.label === 'Allowance Limit') return [];
								const date = new Date(item.parsed.x);
								const allowance = idealAt(date, start, annual);
								const diff = item.parsed.y - allowance;
								const sign = diff >= 0 ? '+' : '';
								return [
									`Allowance: ${formatNumber(Math.round(allowance))} mi`,
									`${sign}${formatNumber(Math.round(diff))} mi vs allowance`
								];
							}
						}
					}
				},
				scales: {
					x: {
						type: 'time',
						min: start.getTime(),
						max: axisMax.getTime(),
						time: {
							tooltipFormat: 'dd MMM yyyy',
							displayFormats: {
								day: 'dd MMM',
								week: 'dd MMM',
								month: 'MMM yyyy'
							}
						},
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
						beginAtZero: true,
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
		<div class="grid {status.has_plan ? 'grid-cols-2' : 'grid-cols-1'} gap-4 mb-6">
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
			{#if status.has_plan}
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
			{/if}
		</div>

		<!-- Chart -->
		<div class="card animate-slide-up stagger-2">
			<div class="flex items-center justify-between mb-4">
				<h2 class="text-lg font-semibold text-carbon-100">{status.vehicle || status.id}</h2>
				<div class="flex items-center gap-4">
					{#if status.has_plan}
						<label class="flex items-center gap-2 text-sm text-carbon-400 cursor-pointer select-none">
							<input
								type="checkbox"
								bind:checked={showYears}
								on:change={renderChart}
								class="accent-accent-primary"
							/>
							Highlight plan years
						</label>
						<label class="flex items-center gap-2 text-sm text-carbon-400 cursor-pointer select-none">
							<input
								type="checkbox"
								bind:checked={showProjection}
								on:change={renderChart}
								class="accent-accent-primary"
							/>
							Project to plan end
						</label>
						<p class="text-sm text-carbon-500">
							{formatDate(status.plan_start)} → {formatDate(status.plan_end)}
						</p>
					{/if}
				</div>
			</div>
			<div class="h-[400px]">
				<canvas bind:this={chartCanvas}></canvas>
			</div>
		</div>

		<!-- Stats Summary -->
		<div class="grid {status.has_plan ? 'grid-cols-3' : 'grid-cols-2'} gap-4 mt-6">
			<div class="card animate-slide-up stagger-3 text-center">
				<p class="text-sm text-carbon-400 mb-1">Total Readings</p>
				<p class="text-2xl font-mono font-bold text-carbon-100">{graphData.dates.length}</p>
			</div>
			{#if status.has_plan}
				<div class="card animate-slide-up stagger-4 text-center">
					<p class="text-sm text-carbon-400 mb-1">Difference</p>
					<p class="text-2xl font-mono font-bold {status.delta <= 0 ? 'text-gauge-green' : 'text-gauge-red'}">
						{status.delta > 0 ? '+' : ''}{formatNumber(Math.round(status.delta))} mi
					</p>
				</div>
			{/if}
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
