<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError, type RegionResp, type Slot, type Window } from '$lib/api';
	import { session } from '$lib/session.svelte';
	import RateChart from '$lib/RateChart.svelte';

	let region = $state<RegionResp | null>(null);
	let duration = $state('3h');
	let cheapest = $state<Window | null>(null);
	let cheapestError = $state<string | null>(null);

	let slots = $state<Slot[]>([]);
	let chartError = $state<string | null>(null);

	async function loadRegion() {
		try {
			region = await api.getRegion();
		} catch {
			region = null;
		}
	}

	async function loadCheapest() {
		cheapestError = null;
		cheapest = null;
		try {
			cheapest = await api.cheapest(duration);
		} catch (e) {
			cheapestError = e instanceof ApiError ? e.message : String(e);
		}
	}

	async function loadRates() {
		chartError = null;
		try {
			slots = await api.rates();
		} catch (e) {
			chartError = e instanceof ApiError ? e.message : String(e);
		}
	}

	onMount(async () => {
		if (!session.me) return;
		await loadRegion();
		if (region) {
			await Promise.all([loadCheapest(), loadRates()]);
		}
	});
</script>

{#if !session.me}
	<p>Not signed in. <a class="text-blue-600 underline" href="/login">Sign in →</a></p>
{:else if !region}
	<section class="rounded-lg border border-slate-200 bg-white p-6">
		<h2 class="mb-2 text-lg font-semibold">Welcome!</h2>
		<p class="mb-4 text-slate-600">
			Set your DNO region before we can find cheap slots for you.
		</p>
		<a
			href="/settings"
			class="inline-block rounded bg-blue-600 px-4 py-2 text-white hover:bg-blue-700"
			>Go to Settings</a
		>
	</section>
{:else}
	<section class="mb-6 rounded-lg border border-slate-200 bg-white p-6">
		<div class="mb-4 flex items-baseline justify-between">
			<h2 class="text-lg font-semibold">
				Region {region.region} — {region.region_name}
			</h2>
			<a href="/settings" class="text-sm text-slate-500 hover:underline">Change</a>
		</div>

		<div class="flex items-end gap-3">
			<label class="flex flex-col text-sm">
				<span class="text-slate-600">Window length</span>
				<input
					class="mt-1 rounded border border-slate-300 px-2 py-1"
					bind:value={duration}
					placeholder="3h"
				/>
			</label>
			<button
				class="rounded bg-blue-600 px-4 py-1.5 text-white hover:bg-blue-700"
				onclick={loadCheapest}
			>
				Find cheapest
			</button>
		</div>

		{#if cheapestError}
			<p class="mt-3 text-sm text-red-600">{cheapestError}</p>
		{:else if cheapest}
			<div class="mt-4 rounded bg-slate-50 p-4 text-sm">
				<p class="font-medium">
					Cheapest {duration} window: {new Date(cheapest.start).toLocaleString()} →
					{new Date(cheapest.end).toLocaleTimeString()}
				</p>
				<p class="text-slate-600">
					Mean {cheapest.mean_inc_vat_p_per_kwh.toFixed(2)} p/kWh (inc VAT)
				</p>
			</div>
		{/if}
	</section>

	<section class="rounded-lg border border-slate-200 bg-white p-6">
		<h2 class="mb-4 text-lg font-semibold">Published rates</h2>
		{#if chartError}
			<p class="text-sm text-red-600">{chartError}</p>
		{:else if slots.length > 0}
			<RateChart {slots} />
		{:else}
			<p class="text-sm text-slate-500">No rates yet.</p>
		{/if}
	</section>
{/if}
