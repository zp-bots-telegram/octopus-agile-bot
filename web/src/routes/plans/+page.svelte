<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError, type ChargePlan } from '$lib/api';

	let plans = $state<ChargePlan[]>([]);
	let error = $state<string | null>(null);

	let durationMinutes = $state(240);
	let start = $state('22:00');
	let end = $state('07:00');

	async function load() {
		try {
			plans = (await api.listChargePlans()) ?? [];
		} catch (e) {
			error = e instanceof ApiError ? e.message : String(e);
		}
	}

	async function create() {
		error = null;
		try {
			await api.createChargePlan(durationMinutes, start, end);
			await load();
		} catch (e) {
			error = e instanceof ApiError ? e.message : String(e);
		}
	}

	async function cancel(id: number) {
		try {
			await api.cancelChargePlan(id);
			await load();
		} catch (e) {
			error = e instanceof ApiError ? e.message : String(e);
		}
	}

	function humanDuration(nsOrMin: number): string {
		// Go marshals time.Duration as nanoseconds; accept either.
		const mins = nsOrMin >= 60_000_000 ? Math.round(nsOrMin / 60_000_000_000) * 60 : nsOrMin;
		const h = Math.floor(mins / 60);
		const m = mins % 60;
		if (h === 0) return `${m}m`;
		if (m === 0) return `${h}h`;
		return `${h}h${String(m).padStart(2, '0')}m`;
	}

	onMount(load);
</script>

<h2 class="mb-4 text-xl font-semibold">Charge plans</h2>

{#if error}
	<p class="mb-4 text-sm text-red-600">{error}</p>
{/if}

<section class="mb-6 rounded-lg border border-slate-200 bg-white p-4">
	<h3 class="mb-3 font-semibold">Add a plan</h3>
	<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_1fr_auto]">
		<label class="text-sm">
			<span class="text-slate-600">Duration (minutes)</span>
			<input
				class="mt-1 w-full rounded border border-slate-300 px-2 py-1"
				type="number"
				min="30"
				step="30"
				bind:value={durationMinutes}
			/>
		</label>
		<label class="text-sm">
			<span class="text-slate-600">Earliest start (HH:MM)</span>
			<input
				class="mt-1 w-full rounded border border-slate-300 px-2 py-1"
				bind:value={start}
				placeholder="22:00"
			/>
		</label>
		<label class="text-sm">
			<span class="text-slate-600">Latest end (HH:MM)</span>
			<input
				class="mt-1 w-full rounded border border-slate-300 px-2 py-1"
				bind:value={end}
				placeholder="07:00"
			/>
		</label>
		<button
			class="self-end rounded bg-blue-600 px-4 py-1.5 text-white hover:bg-blue-700"
			onclick={create}
		>
			Add
		</button>
	</div>
	<p class="mt-2 text-xs text-slate-500">
		End earlier than start means overnight (e.g. 22:00–07:00 = 9h window crossing midnight).
	</p>
</section>

<section>
	{#if plans.length === 0}
		<p class="text-slate-600">No charge plans yet.</p>
	{:else}
		<ul class="space-y-2">
			{#each plans as p}
				<li class="flex items-center justify-between rounded border border-slate-200 bg-white p-3">
					<div>
						<p class="font-medium">
							#{p.ID} — {humanDuration(p.Duration)} between {p.WindowStartLocal}–{p.WindowEndLocal}
						</p>
						<p class="text-sm text-slate-500">
							{p.Enabled ? 'Active' : 'Paused'}
						</p>
					</div>
					<button
						class="text-sm text-red-600 hover:underline"
						onclick={() => cancel(p.ID)}
					>
						Cancel
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</section>
