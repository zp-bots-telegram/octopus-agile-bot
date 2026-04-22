<script lang="ts">
	import { onMount } from 'svelte';
	import { Alert, Button, Card, CardBody, CardHeader, CardTitle } from '@immich/ui';
	import { api, ApiError } from '$lib/api';

	type Point = { interval_start: string; interval_end: string; consumption_kwh: number };

	let points = $state<Point[]>([]);
	let error = $state<string | null>(null);
	let loading = $state(false);
	let needLink = $state(false);

	const fmtDay = (d: Date) => d.toISOString().slice(0, 10);
	let from = $state(fmtDay(new Date(Date.now() - 7 * 86400_000)));
	let to = $state(fmtDay(new Date()));
	let groupBy = $state('day');

	async function load() {
		error = null;
		needLink = false;
		loading = true;
		try {
			const fromISO = new Date(from + 'T00:00:00Z').toISOString();
			const toISO = new Date(to + 'T23:59:59Z').toISOString();
			points = await api.consumption(fromISO, toISO, groupBy);
		} catch (e) {
			if (e instanceof ApiError && e.status === 428) {
				needLink = true;
			} else {
				error = e instanceof ApiError ? e.message : String(e);
			}
			points = [];
		} finally {
			loading = false;
		}
	}

	const total = $derived(points.reduce((a, p) => a + p.consumption_kwh, 0));

	onMount(load);
</script>

<h2 class="mb-4 text-xl font-semibold">Consumption</h2>

<div class="space-y-4">
	{#if needLink}
		<Alert color="warning" title="Link your Octopus account first">
			Visit <a class="text-primary-600 dark:text-primary-400 underline" href="/settings">
				Settings
			</a> to link your account — we need your API key and MPAN to query consumption.
		</Alert>
	{:else}
		{#if error}
			<Alert color="danger">{error}</Alert>
		{/if}

		<Card>
			<CardHeader>
				<CardTitle>Range</CardTitle>
			</CardHeader>
			<CardBody>
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_1fr_auto] items-end">
					<label class="text-sm">
						<span class="text-dark/80 dark:text-light/80">From</span>
						<input
							class="mt-1 w-full rounded border border-light-300 dark:border-dark-300 bg-transparent px-2 py-1"
							type="date"
							bind:value={from}
						/>
					</label>
					<label class="text-sm">
						<span class="text-dark/80 dark:text-light/80">To</span>
						<input
							class="mt-1 w-full rounded border border-light-300 dark:border-dark-300 bg-transparent px-2 py-1"
							type="date"
							bind:value={to}
						/>
					</label>
					<label class="text-sm">
						<span class="text-dark/80 dark:text-light/80">Group by</span>
						<select
							class="mt-1 w-full rounded border border-light-300 dark:border-dark-300 bg-transparent px-2 py-1"
							bind:value={groupBy}
						>
							<option value="">Half-hourly</option>
							<option value="hour">Hourly</option>
							<option value="day">Daily</option>
							<option value="week">Weekly</option>
							<option value="month">Monthly</option>
						</select>
					</label>
					<Button onclick={load} loading={loading}>Reload</Button>
				</div>
			</CardBody>
		</Card>

		<Card>
			<CardHeader>
				<div class="flex w-full items-center justify-between">
					<CardTitle>Usage</CardTitle>
					<span class="text-sm text-dark/60 dark:text-light/60">
						Total: {total.toFixed(2)} kWh
					</span>
				</div>
			</CardHeader>
			<CardBody>
				{#if points.length === 0}
					<p class="text-sm text-dark/60 dark:text-light/60">
						No consumption recorded in this range.
					</p>
				{:else}
					<div class="overflow-x-auto">
						<table class="w-full text-sm">
							<thead class="text-left text-dark/60 dark:text-light/60">
								<tr>
									<th class="py-1 pr-4 font-normal">From</th>
									<th class="py-1 pr-4 font-normal">To</th>
									<th class="py-1 text-right font-normal">kWh</th>
								</tr>
							</thead>
							<tbody>
								{#each points as p}
									<tr class="border-t border-light-200 dark:border-dark-200">
										<td class="py-1 pr-4">{new Date(p.interval_start).toLocaleString()}</td>
										<td class="py-1 pr-4">{new Date(p.interval_end).toLocaleString()}</td>
										<td class="py-1 text-right tabular-nums">
											{p.consumption_kwh.toFixed(3)}
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			</CardBody>
		</Card>
	{/if}
</div>
