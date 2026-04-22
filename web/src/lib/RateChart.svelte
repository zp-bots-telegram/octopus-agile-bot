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

	function render() {
		if (!container) return;
		if (plot) {
			plot.destroy();
			plot = null;
		}
		const opts: uPlot.Options = {
			width: container.clientWidth,
			height: 280,
			scales: { x: { time: true } },
			series: [
				{},
				{
					label: 'p/kWh inc VAT',
					stroke: '#2563eb',
					fill: 'rgba(37, 99, 235, 0.1)',
					width: 2
				}
			],
			axes: [
				{
					values: (_, ticks) =>
						ticks.map((t) =>
							new Date(t * 1000).toLocaleTimeString([], {
								hour: '2-digit',
								minute: '2-digit'
							})
						)
				},
				{ label: 'p/kWh' }
			]
		};
		plot = new uPlot(opts, buildData(slots), container);
	}

	onMount(() => {
		render();
		const ro = new ResizeObserver(() => {
			if (plot && container) plot.setSize({ width: container.clientWidth, height: 280 });
		});
		ro.observe(container);
		onDestroy(() => ro.disconnect());
	});

	$effect(() => {
		if (plot) plot.setData(buildData(slots));
	});
</script>

<div bind:this={container} style="width: 100%; height: 280px;"></div>
