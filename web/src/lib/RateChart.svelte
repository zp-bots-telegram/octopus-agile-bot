<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import uPlot from 'uplot';
	import type { Slot } from './api';

	let { slots }: { slots: Slot[] } = $props();

	let container: HTMLDivElement;
	let plot: uPlot | null = null;

	function buildData(sl: Slot[]): uPlot.AlignedData {
		const xs: number[] = [];
		const ys: number[] = [];
		for (const s of sl) {
			xs.push(Math.floor(new Date(s.valid_from).getTime() / 1000));
			ys.push(s.inc_vat_p_per_kwh);
		}
		return [xs, ys];
	}

	// Read a CSS token off <html>. The Immich palette's light/dark tokens already
	// auto-invert when the `dark` class is applied, so we don't need a mode branch.
	function token(name: string): string {
		return (
			getComputedStyle(document.documentElement).getPropertyValue(name).trim() || '#0f172a'
		);
	}

	function render() {
		if (!container) return;
		if (plot) {
			plot.destroy();
			plot = null;
		}

		const axisStroke = token('--color-dark'); // body text colour — dark in light mode, light in dark mode
		const gridStroke = token('--color-light-300'); // subtle grid — also auto-inverts

		const opts: uPlot.Options = {
			width: container.clientWidth,
			height: 280,
			scales: { x: { time: true } },
			series: [
				{},
				{
					label: 'p/kWh inc VAT',
					stroke: '#2563eb',
					fill: 'rgba(37, 99, 235, 0.15)',
					width: 2
				}
			],
			axes: [
				{
					stroke: axisStroke,
					grid: { stroke: gridStroke, width: 1 },
					ticks: { stroke: gridStroke, width: 1 },
					values: (_, ticks) =>
						ticks.map((t) =>
							new Date(t * 1000).toLocaleTimeString([], {
								hour: '2-digit',
								minute: '2-digit'
							})
						)
				},
				{
					label: 'p/kWh',
					stroke: axisStroke,
					grid: { stroke: gridStroke, width: 1 },
					ticks: { stroke: gridStroke, width: 1 }
				}
			]
		};
		plot = new uPlot(opts, buildData(slots), container);
	}

	let ro: ResizeObserver | null = null;
	let mo: MutationObserver | null = null;

	onMount(() => {
		render();

		ro = new ResizeObserver(() => {
			if (plot && container) plot.setSize({ width: container.clientWidth, height: 280 });
		});
		ro.observe(container);

		// Re-render on theme toggle (html gains/loses the `dark` class).
		mo = new MutationObserver(() => render());
		mo.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
	});

	onDestroy(() => {
		ro?.disconnect();
		mo?.disconnect();
		plot?.destroy();
	});

	$effect(() => {
		if (plot) plot.setData(buildData(slots));
	});
</script>

<div bind:this={container} style="width: 100%;"></div>
