<script lang="ts">
	import { onMount } from 'svelte';
	import { Alert, Button, Card, CardBody, CardHeader, CardTitle } from '@immich/ui';
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

	// Go's time.Duration marshals as int64 nanoseconds. Convert to whole minutes.
	function humanDuration(ns: number): string {
		const mins = Math.round(ns / 60_000_000_000);
		const h = Math.floor(mins / 60);
		const m = mins % 60;
		if (h === 0) return `${m}m`;
		if (m === 0) return `${h}h`;
		return `${h}h${String(m).padStart(2, '0')}m`;
	}

	onMount(load);
</script>

<h2 class="mb-4 text-xl font-semibold">Charge plans</h2>

<div class="space-y-4">
	{#if error}
		<Alert color="danger">{error}</Alert>
	{/if}

	<Card>
		<CardHeader>
			<CardTitle>Add a plan</CardTitle>
		</CardHeader>
		<CardBody>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_1fr_auto] items-end">
				<label class="text-sm">
					<span class="text-dark/80">Duration (minutes)</span>
					<input
						class="mt-1 w-full rounded border border-light-300 bg-transparent px-2 py-1"
						type="number"
						min="30"
						step="30"
						bind:value={durationMinutes}
					/>
				</label>
				<label class="text-sm">
					<span class="text-dark/80">Earliest start (HH:MM)</span>
					<input
						class="mt-1 w-full rounded border border-light-300 bg-transparent px-2 py-1"
						bind:value={start}
						placeholder="22:00 or 10pm"
					/>
				</label>
				<label class="text-sm">
					<span class="text-dark/80">Latest end (HH:MM)</span>
					<input
						class="mt-1 w-full rounded border border-light-300 bg-transparent px-2 py-1"
						bind:value={end}
						placeholder="07:00 or 7am"
					/>
				</label>
				<Button onclick={create}>Add</Button>
			</div>
			<p class="mt-2 text-xs text-dark/60">
				End earlier than start means overnight (e.g. 22:00–07:00 = 9h window crossing midnight).
			</p>
		</CardBody>
	</Card>

	{#if plans.length === 0}
		<p class="text-dark/80">No charge plans yet.</p>
	{:else}
		<div class="space-y-2">
			{#each plans as p}
				<Card>
					<CardBody>
						<div class="flex items-center justify-between">
							<div>
								<p class="font-medium">
									#{p.ID} — {humanDuration(p.Duration)} between {p.WindowStartLocal}–{p.WindowEndLocal}
								</p>
								<p class="text-sm text-dark/60">
									{p.Enabled ? 'Active' : 'Paused'}
								</p>
							</div>
							<Button size="small" color="danger" variant="outline" onclick={() => cancel(p.ID)}>
								Cancel
							</Button>
						</div>
					</CardBody>
				</Card>
			{/each}
		</div>
	{/if}
</div>
