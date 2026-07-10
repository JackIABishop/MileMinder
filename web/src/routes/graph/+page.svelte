<script lang="ts">
	import { onMount } from 'svelte';
	import { tick } from 'svelte';
	import {
		getCurrentVehicle,
		getFleet,
		getGraphData,
		getScenario,
		formatNumber,
		formatDate,
		type VehicleStatus,
		type GraphData,
		type Scenario
	} from '$lib/api';
	import { formatMoneyMinor } from '$lib/money';
	import { settings } from '$lib/settings';
	import { Chart, registerables } from 'chart.js';
	import 'chartjs-adapter-date-fns';

	Chart.register(...registerables);

	type GraphMode = 'single' | 'compare';

	type CompareSeries = {
		id: string;
		status: VehicleStatus;
		graph: GraphData;
		color: string;
		fill: string;
		origin: Date;
		originKind: 'plan start' | 'first reading';
	};

	let status: VehicleStatus | null = null;
	let graphData: GraphData | null = null;
	let fleet: VehicleStatus[] = [];
	let compareGraphData: Record<string, GraphData> = {};
	let loading = true;
	let comparisonLoading = false;
	let error = '';
	let comparisonError = '';
	let chartCanvas: HTMLCanvasElement;
	let chart: Chart | null = null;
	let showProjection = false;
	let showYears = false;
	let graphMode: GraphMode = 'single';

	// What-if scenario (issue #9): a read-only projection overlay within single
	// mode. All the math is server-side (getScenario); this holds only form state
	// and the last result.
	let whatIfOpen = false;
	let scenarioExtraMiles = 600;
	let scenarioByDate = '';
	let scenario: Scenario | null = null;
	let scenarioLoading = false;
	let scenarioError = '';
	let compareA = '';
	let compareB = '';
	let comparisonLoadToken = 0;

	const DAY_MS = 24 * 60 * 60 * 1000;
	const compareColors = [
		{ line: '#22c55e', fill: 'rgba(34, 197, 94, 0.08)' },
		{ line: '#f59e0b', fill: 'rgba(245, 158, 11, 0.08)' }
	];

	onMount(async () => {
		try {
			const [current, fleetResp] = await Promise.all([getCurrentVehicle(), getFleet()]);
			fleet = fleetResp.vehicles;

			const currentID =
				current.current && fleet.some((v) => v.id === current.current)
					? current.current
					: fleet[0]?.id || '';

			if (currentID) {
				status = fleet.find((v) => v.id === currentID) || null;
				graphData = await getGraphData(currentID);
			}

			setCompareDefaults(currentID);
			scenarioByDate = toISODate(new Date(Date.now() + 30 * DAY_MS));
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load data';
		} finally {
			loading = false;
		}
	});

	// YYYY-MM-DD in the user's local date, for date inputs and API calls.
	function toISODate(d: Date): string {
		const tz = d.getTimezoneOffset() * 60000;
		return new Date(d.getTime() - tz).toISOString().slice(0, 10);
	}

	async function runScenario() {
		if (!status?.has_plan) return;
		scenarioError = '';
		scenarioLoading = true;
		try {
			scenario = await getScenario(status.id, {
				extra_miles: scenarioExtraMiles,
				by_date: scenarioByDate
			});
		} catch (e) {
			scenario = null;
			scenarioError = e instanceof Error ? e.message : 'Failed to run scenario';
		} finally {
			scenarioLoading = false;
			renderSingleChart();
		}
	}

	function clearScenario() {
		scenario = null;
		scenarioError = '';
		renderSingleChart();
	}

	$: compareSeries = buildCompareSeries(compareA, compareB, fleet, compareGraphData);
	$: compareReady = compareSeries.length === 2;
	$: shouldLoadComparison =
		!loading &&
		graphMode === 'compare' &&
		fleet.length >= 2 &&
		(!compareA ||
			!compareB ||
			compareA === compareB ||
			!compareGraphData[compareA] ||
			!compareGraphData[compareB]) &&
		!comparisonLoading;

	// Render the active chart once the canvas and its data are available.
	$: if (chartCanvas && !loading && graphMode === 'single' && status && graphData) {
		renderSingleChart();
	}

	$: if (shouldLoadComparison) {
		loadComparison();
	}

	$: if (chartCanvas && !loading && graphMode === 'compare' && compareReady && !comparisonLoading) {
		renderCompareChart();
	}

	function destroyChart() {
		if (chart) {
			chart.destroy();
			chart = null;
		}
	}

	// Whole days between two dates (positive if b is after a)
	function daysBetween(a: Date, b: Date): number {
		return (b.getTime() - a.getTime()) / DAY_MS;
	}

	// Ideal miles accrued by a given date: straight line from plan start.
	function idealAt(date: Date, start: Date, annual: number): number {
		const days = Math.max(0, daysBetween(start, date));
		return (annual * days) / 365;
	}

	function originFor(status: VehicleStatus, graph: GraphData): { date: Date; kind: 'plan start' | 'first reading' } {
		if (status.has_plan) {
			return { date: new Date(status.plan_start), kind: 'plan start' };
		}
		return { date: graph.dates[0] ? new Date(graph.dates[0]) : new Date(), kind: 'first reading' };
	}

	function displayName(status: VehicleStatus): string {
		const name = status.vehicle || status.id;
		return status.registration ? `${name} (${status.registration})` : name;
	}

	function setCompareDefaults(preferredID = '') {
		if (fleet.length < 2) {
			compareA = fleet[0]?.id || '';
			compareB = '';
			return;
		}

		const first = preferredID && fleet.some((v) => v.id === preferredID) ? preferredID : fleet[0].id;
		const second = fleet.find((v) => v.id !== first)?.id || '';
		compareA = first;
		compareB = second;
	}

	function ensureComparePair() {
		if (fleet.length < 2) return;

		const validA = compareA && fleet.some((v) => v.id === compareA);
		if (!validA) {
			compareA = fleet[0].id;
		}

		const validB = compareB && fleet.some((v) => v.id === compareB) && compareB !== compareA;
		if (!validB) {
			compareB = fleet.find((v) => v.id !== compareA)?.id || '';
		}
	}

	async function setGraphMode(next: GraphMode) {
		if (graphMode === next) return;
		graphMode = next;
		destroyChart();
		if (next === 'compare') {
			scenario = null;
			scenarioError = '';
			await tick();
			await loadComparison();
		}
	}

	async function updateCompareA() {
		if (compareA === compareB) {
			compareB = fleet.find((v) => v.id !== compareA)?.id || '';
		}
		await tick();
		await loadComparison();
	}

	async function updateCompareB() {
		if (compareA === compareB) {
			compareA = fleet.find((v) => v.id !== compareB)?.id || '';
		}
		await tick();
		await loadComparison();
	}

	async function loadComparison() {
		destroyChart();
		comparisonError = '';
		ensureComparePair();
		if (fleet.length < 2 || !compareA || !compareB || compareA === compareB) return;

		const token = ++comparisonLoadToken;
		comparisonLoading = true;
		try {
			const ids = [compareA, compareB];
			const missing = ids.filter((id) => !compareGraphData[id]);
			if (missing.length > 0) {
				const loaded = await Promise.all(missing.map((id) => getGraphData(id)));
				const next = { ...compareGraphData };
				missing.forEach((id, i) => {
					next[id] = loaded[i];
				});
				compareGraphData = next;
			}
		} catch (e) {
			comparisonError = e instanceof Error ? e.message : 'Failed to load comparison data';
		} finally {
			if (token === comparisonLoadToken) {
				comparisonLoading = false;
			}
		}
	}

	function buildCompareSeries(
		firstID: string,
		secondID: string,
		vehicles: VehicleStatus[],
		graphs: Record<string, GraphData>
	): CompareSeries[] {
		const ids = [firstID, secondID];
		if (ids.some((id) => !id) || firstID === secondID) return [];

		return ids.flatMap((id, index) => {
			const vehicle = vehicles.find((v) => v.id === id);
			const graph = graphs[id];
			if (!vehicle || !graph) return [];

			const origin = originFor(vehicle, graph);
			return [
				{
					id,
					status: vehicle,
					graph,
					color: compareColors[index].line,
					fill: compareColors[index].fill,
					origin: origin.date,
					originKind: origin.kind
				}
			];
		});
	}

	// Sample a time line into weekly points so it stays hoverable at any date.
	function sampleLine(fromMs: number, toMs: number, yAtMs: (ms: number) => number) {
		const stepMs = 7 * DAY_MS;
		const points: { x: number; y: number }[] = [];
		if (toMs <= fromMs) {
			return [{ x: fromMs, y: yAtMs(fromMs) }];
		}
		for (let t = fromMs; t < toMs; t += stepMs) {
			points.push({ x: t, y: yAtMs(t) });
		}
		points.push({ x: toMs, y: yAtMs(toMs) });
		return points;
	}

	function sampleElapsedLine(toDays: number, yAtDay: (day: number) => number) {
		const stepDays = 7;
		const end = Math.max(0, toDays);
		const points: { x: number; y: number }[] = [];
		for (let day = 0; day < end; day += stepDays) {
			points.push({ x: day, y: yAtDay(day) });
		}
		points.push({ x: end, y: yAtDay(end) });
		return points;
	}

	function elapsedLabel(days: number): string {
		if (days < 30) return `day ${Math.round(days)}`;
		return `day ${Math.round(days)} · month ${Math.floor(days / 30) + 1}`;
	}

	function renderSingleChart() {
		if (!graphData || !status || !chartCanvas) return;

		destroyChart();

		const ctx = chartCanvas.getContext('2d');
		if (!ctx) return;

		const firstReadingDate = graphData.dates[0] ? new Date(graphData.dates[0]) : new Date();
		const start = status.has_plan ? new Date(status.plan_start) : firstReadingDate;
		const end = status.has_plan ? new Date(status.plan_end) : new Date();
		const today = new Date();
		const annual = status.annual_allowance;

		// Right edge of the time axis: today, or the plan end when projecting.
		// A pending what-if scenario extends the axis to its target date.
		let axisMax = status.has_plan && showProjection ? end : today;
		if (scenario) {
			const byDate = new Date(scenario.by_date);
			if (byDate.getTime() > axisMax.getTime()) axisMax = byDate;
		}

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

		// What-if overlay: a straight dashed line from the latest reading to the
		// hypothetical position at by_date. The y-axis is miles-since-start, so the
		// endpoint subtracts start_miles (matching the graph's actuals baseline).
		if (scenario && graphData.dates.length > 0) {
			const lastDate = new Date(graphData.dates[graphData.dates.length - 1]);
			const lastY = graphData.actuals[graphData.actuals.length - 1];
			const fromMs = lastDate.getTime();
			const byMs = new Date(scenario.by_date).getTime();
			const endY = scenario.hypothetical_miles - status.start_miles;
			datasets.push({
				label: 'What-if scenario',
				data: sampleLine(fromMs, byMs, (ms) =>
					byMs > fromMs ? lastY + ((endY - lastY) * (ms - fromMs)) / (byMs - fromMs) : endY
				),
				borderColor: '#a855f7',
				backgroundColor: 'transparent',
				borderDash: [6, 4],
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
							weight: 600
						},
						bodyFont: {
							family: 'JetBrains Mono'
						},
						callbacks: {
							title: function(items) {
								if (!items.length) return '';
								return new Date(items[0].parsed.x ?? 0).toLocaleDateString('en-GB', {
									day: '2-digit',
									month: 'short',
									year: 'numeric'
								});
							},
							label: function(context) {
								return `${context.dataset.label}: ${formatNumber(Math.round(context.parsed.y ?? 0))} mi`;
							},
							afterBody: function(items) {
								if (!status?.has_plan) return [];
								if (!items.length) return [];
								const item = items[0];
								if (item.dataset.label === 'Allowance Limit') return [];
								const date = new Date(item.parsed.x ?? 0);
								const allowance = idealAt(date, start, annual);
								const diff = (item.parsed.y ?? 0) - allowance;
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

	function renderCompareChart() {
		if (!chartCanvas || compareSeries.length !== 2) return;

		destroyChart();

		const ctx = chartCanvas.getContext('2d');
		if (!ctx) return;

		const actualDatasets = compareSeries.map((series) => {
			const points = series.graph.dates
				.map((d, i) => ({
					x: daysBetween(series.origin, new Date(d)),
					y: series.graph.actuals[i]
				}))
				.filter((point) => point.x >= 0);

			return {
				label: displayName(series.status),
				data: points,
				borderColor: series.color,
				backgroundColor: series.fill,
				fill: false,
				tension: 0.3,
				pointBackgroundColor: series.color,
				pointBorderColor: series.color,
				pointRadius: 4,
				pointHoverRadius: 6,
				originDate: series.origin,
				originKind: series.originKind,
				kind: 'actual'
			};
		});

		const maxActualDay = Math.max(
			30,
			...actualDatasets.flatMap((dataset) => dataset.data.map((point) => point.x))
		);

		const allowanceDatasets = compareSeries
			.filter((series) => series.status.has_plan)
			.map((series) => {
				const planEnd = new Date(series.status.plan_end);
				const planDays = Math.max(0, daysBetween(series.origin, planEnd));
				const lineEnd = Math.min(Math.max(30, maxActualDay), planDays || maxActualDay);

				return {
					label: `${displayName(series.status)} allowance`,
					data: sampleElapsedLine(lineEnd, (day) => (series.status.annual_allowance * day) / 365),
					borderColor: series.color,
					backgroundColor: 'transparent',
					borderDash: [5, 5],
					pointRadius: 0,
					pointHoverRadius: 4,
					originDate: series.origin,
					originKind: series.originKind,
					kind: 'allowance',
					annualAllowance: series.status.annual_allowance
				};
			});

		chart = new Chart(ctx, {
			type: 'line',
			data: { datasets: [...actualDatasets, ...allowanceDatasets] },
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
							weight: 600
						},
						bodyFont: {
							family: 'JetBrains Mono'
						},
						callbacks: {
							title: function(items) {
								if (!items.length) return '';
								return elapsedLabel(items[0].parsed.x ?? 0);
							},
							label: function(context) {
								const dataset: any = context.dataset;
								const value = formatNumber(Math.round(context.parsed.y ?? 0));
								const suffix = dataset.kind === 'allowance' ? ' allowance' : ' normalized miles';
								return `${dataset.label}: ${value} mi${suffix}`;
							},
							afterLabel: function(context) {
								const dataset: any = context.dataset;
								const lines = [
									`Origin: ${dataset.originKind} · ${formatDate(dataset.originDate.toISOString())}`
								];
								if (dataset.kind === 'allowance') {
									lines.push(`${formatNumber(dataset.annualAllowance)} mi/year`);
								}
								return lines;
							}
						}
					}
				},
				scales: {
					x: {
						type: 'linear',
						min: 0,
						max: maxActualDay,
						grid: {
							color: 'rgba(66, 66, 75, 0.3)'
						},
						title: {
							display: true,
							text: 'Elapsed time since each vehicle origin',
							color: '#91919f',
							font: {
								family: 'Outfit'
							}
						},
						ticks: {
							color: '#91919f',
							font: {
								family: 'Outfit'
							},
							callback: function(value) {
								return elapsedLabel(Number(value));
							}
						}
					},
					y: {
						beginAtZero: true,
						grid: {
							color: 'rgba(66, 66, 75, 0.3)'
						},
						title: {
							display: true,
							text: 'Miles driven since origin',
							color: '#91919f',
							font: {
								family: 'Outfit'
							}
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

<div class="p-4 sm:p-6 lg:p-8">
	<header class="mb-8 animate-fade-in">
		<div class="flex flex-wrap items-start justify-between gap-4">
			<div>
				<h1 class="text-3xl font-display font-bold text-carbon-100">Mileage Graph</h1>
				<p class="text-carbon-500 mt-2">Track usage against allowance or compare two vehicles</p>
			</div>
			<div class="inline-flex rounded-lg border border-carbon-700 bg-carbon-800/50 p-1">
				<button
					class="px-3 py-2 rounded text-sm font-medium transition-colors {graphMode === 'single' ? 'bg-accent-primary text-carbon-950' : 'text-carbon-400 hover:text-carbon-100'}"
					on:click={() => setGraphMode('single')}
				>
					Single
				</button>
				<button
					class="px-3 py-2 rounded text-sm font-medium transition-colors {graphMode === 'compare' ? 'bg-accent-primary text-carbon-950' : 'text-carbon-400 hover:text-carbon-100'}"
					on:click={() => setGraphMode('compare')}
				>
					Compare
				</button>
			</div>
		</div>
	</header>

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="animate-pulse text-carbon-400">Loading...</div>
		</div>
	{:else if error}
		<div class="card border-gauge-red/30 bg-gauge-red/5">
			<p class="text-gauge-red">{error}</p>
		</div>
	{:else if graphMode === 'single'}
		{#if status && graphData}
			<!-- Legend Info -->
			<div class="grid {status.has_plan ? 'grid-cols-1 sm:grid-cols-2' : 'grid-cols-1'} gap-4 mb-6">
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
				<div class="flex flex-col gap-4 mb-4 lg:flex-row lg:items-center lg:justify-between">
					<h2 class="text-lg font-semibold text-carbon-100 break-words">{status.vehicle || status.id}</h2>
					<div class="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center sm:gap-4 lg:justify-end">
						{#if status.has_plan}
							<label class="flex items-center gap-2 text-sm text-carbon-400 cursor-pointer select-none">
								<input
									type="checkbox"
									bind:checked={showYears}
									on:change={renderSingleChart}
									class="accent-accent-primary"
								/>
								Highlight plan years
							</label>
							<label class="flex items-center gap-2 text-sm text-carbon-400 cursor-pointer select-none">
								<input
									type="checkbox"
									bind:checked={showProjection}
									on:change={renderSingleChart}
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
				<div class="h-[300px] sm:h-[400px]">
					<canvas bind:this={chartCanvas}></canvas>
				</div>
			</div>

			<!-- What-if scenario (only meaningful against an allowance) -->
			{#if status.has_plan}
				<div class="card animate-slide-up stagger-2 mt-6">
					<button
						class="flex w-full items-center justify-between text-left"
						on:click={() => (whatIfOpen = !whatIfOpen)}
						aria-expanded={whatIfOpen}
					>
						<div>
							<h2 class="text-lg font-semibold text-carbon-100">What if?</h2>
							<p class="text-sm text-carbon-500">
								See where an extra trip puts you against your allowance — nothing is saved.
							</p>
						</div>
						<span class="text-carbon-400 text-xl">{whatIfOpen ? '−' : '+'}</span>
					</button>

					{#if whatIfOpen}
						<div class="mt-4 border-t border-carbon-700 pt-4">
							<div class="flex flex-col gap-4 sm:flex-row sm:items-end">
								<div class="flex-1 min-w-0">
									<label class="label" for="scenario-miles">Extra miles</label>
									<input
										id="scenario-miles"
										type="number"
										min="0"
										step="1"
										class="input"
										bind:value={scenarioExtraMiles}
									/>
								</div>
								<div class="flex-1 min-w-0">
									<label class="label" for="scenario-date">By date</label>
									<input
										id="scenario-date"
										type="date"
										class="input"
										min={toISODate(new Date(Date.now() + DAY_MS))}
										max={toISODate(new Date(status.plan_end))}
										bind:value={scenarioByDate}
									/>
								</div>
								<div class="flex gap-2">
									<button class="btn-primary" on:click={runScenario} disabled={scenarioLoading}>
										{scenarioLoading ? 'Calculating…' : 'Run scenario'}
									</button>
									{#if scenario}
										<button class="btn-secondary" on:click={clearScenario}>Clear</button>
									{/if}
								</div>
							</div>

							{#if scenarioError}
								<p class="mt-4 text-sm text-gauge-red">{scenarioError}</p>
							{/if}

							{#if scenario}
								<div class="mt-5">
									<p class="text-sm text-carbon-500 mb-3">
										As of {formatDate(scenario.by_date)}, driving
										{formatNumber(Math.round(scenario.extra_miles))} extra miles:
									</p>
									<div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
										<div class="text-center">
											<p class="text-sm text-carbon-400 mb-1">Projected odometer</p>
											<p class="text-xl font-mono font-bold text-carbon-100">
												{formatNumber(Math.round(scenario.hypothetical_miles))} mi
											</p>
											<p class="text-xs text-carbon-500 mt-1">
												trajectory alone: {formatNumber(Math.round(scenario.baseline_miles))} mi
											</p>
										</div>
										<div class="text-center">
											<p class="text-sm text-carbon-400 mb-1">Vs allowance</p>
											<p class="text-xl font-mono font-bold {scenario.status.delta <= 0 ? 'text-gauge-green' : 'text-gauge-red'}">
												{scenario.status.delta > 0 ? '+' : ''}{formatNumber(Math.round(scenario.status.delta))} mi
											</p>
											<p class="text-xs text-carbon-500 mt-1">
												{scenario.status.delta <= 0 ? 'under the line' : 'over the line'}
											</p>
										</div>
										<div class="text-center">
											<p class="text-sm text-carbon-400 mb-1">Allowance used</p>
											<p class="text-xl font-mono font-bold text-carbon-100">
												{formatNumber(scenario.status.percent_used, 0)}%
											</p>
										</div>
										<div class="text-center">
											<p class="text-sm text-carbon-400 mb-1">Projected overage</p>
											{#if scenario.status.projected_over && (scenario.status.excess_rate ?? 0) > 0}
												<p class="text-xl font-mono font-bold text-gauge-red">
													{formatMoneyMinor(scenario.status.projected_overage_cost_minor ?? 0, $settings.currency)}
												</p>
												<p class="text-xs text-carbon-500 mt-1">assuming this pace continues</p>
											{:else if scenario.status.projected_over}
												<p class="text-xl font-mono font-bold text-gauge-red">On track to exceed</p>
												<p class="text-xs text-carbon-500 mt-1">assuming this pace continues</p>
											{:else}
												<p class="text-xl font-mono font-bold text-gauge-green">Within allowance</p>
											{/if}
										</div>
									</div>
								</div>
							{/if}
						</div>
					{/if}
				</div>
			{/if}

			<!-- Stats Summary -->
			<div class="grid {status.has_plan ? 'grid-cols-1 sm:grid-cols-3' : 'grid-cols-1 sm:grid-cols-2'} gap-4 mt-6">
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
	{:else}
		{#if fleet.length < 2}
			<div class="card">
				<h2 class="text-lg font-semibold text-carbon-100 mb-2">Need two vehicles to compare</h2>
				<p class="text-carbon-400">
					Add another vehicle to compare normalized mileage across your fleet.
				</p>
			</div>
		{:else}
			<div class="card animate-slide-up mb-6">
				<div class="flex flex-col gap-4 sm:flex-row sm:flex-wrap sm:items-end">
					<div class="flex-1 min-w-0">
						<label class="label" for="compare-a">Vehicle A</label>
						<select
							id="compare-a"
							class="input"
							bind:value={compareA}
							on:change={updateCompareA}
						>
							{#each fleet as vehicle}
								<option value={vehicle.id} disabled={vehicle.id === compareB}>
									{displayName(vehicle)}
								</option>
							{/each}
						</select>
					</div>
					<div class="flex-1 min-w-0">
						<label class="label" for="compare-b">Vehicle B</label>
						<select
							id="compare-b"
							class="input"
							bind:value={compareB}
							on:change={updateCompareB}
						>
							{#each fleet as vehicle}
								<option value={vehicle.id} disabled={vehicle.id === compareA}>
									{displayName(vehicle)}
								</option>
							{/each}
						</select>
					</div>
					<div class="text-sm text-carbon-500 max-w-sm">
						Miles are normalized from each vehicle’s plan start, or first reading when no plan exists.
					</div>
				</div>
			</div>

			{#if comparisonLoading}
				<div class="flex items-center justify-center h-64">
					<div class="animate-pulse text-carbon-400">Loading comparison...</div>
				</div>
			{:else if comparisonError}
				<div class="card border-gauge-red/30 bg-gauge-red/5">
					<p class="text-gauge-red">{comparisonError}</p>
				</div>
			{:else if compareReady}
				<div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
					{#each compareSeries as series}
						<div class="card animate-slide-up">
							<div class="flex items-center gap-3">
								<div class="w-4 h-4 rounded-full" style="background-color: {series.color}"></div>
								<div>
									<p class="text-sm text-carbon-400">{displayName(series.status)}</p>
									<p class="text-lg font-mono font-semibold text-carbon-100">
										{formatNumber(series.graph.actuals[series.graph.actuals.length - 1] || 0)} mi
									</p>
									<p class="text-xs text-carbon-500">
										Since {series.originKind}: {formatDate(series.origin.toISOString())}
									</p>
								</div>
							</div>
						</div>
					{/each}
				</div>

				<div class="card animate-slide-up stagger-2">
					<div class="flex flex-wrap items-start justify-between gap-4 mb-4">
						<div>
							<h2 class="text-lg font-semibold text-carbon-100">Two-car comparison</h2>
							<p class="text-sm text-carbon-500 mt-1">
								Miles driven since each vehicle’s own origin, aligned by elapsed day and month.
							</p>
						</div>
					</div>
					<div class="h-[300px] sm:h-[400px]">
						<canvas bind:this={chartCanvas}></canvas>
					</div>
				</div>
			{/if}
		{/if}
	{/if}
</div>
