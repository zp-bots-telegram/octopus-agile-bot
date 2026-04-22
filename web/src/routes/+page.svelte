<script lang="ts">
	import { onMount } from 'svelte';
	import { Alert, Button, Card, CardBody, CardHeader, CardTitle } from '@immich/ui';
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
	<p>
		Not signed in.
		<a class="text-primary-600 dark:text-primary-400 underline" href="/login">Sign in →</a>
	</p>
{:else if !region}
	<Card>
		<CardHeader>
			<CardTitle>Welcome!</CardTitle>
		</CardHeader>
		<CardBody>
			<p class="mb-4 text-dark/80">
				Set your DNO region before we can find cheap slots for you.
			</p>
			<Button href="/settings">Go to Settings</Button>
		</CardBody>
	</Card>
{:else}
	<div class="space-y-6">
		<Card>
			<CardHeader>
				<div class="flex w-full items-center justify-between">
					<CardTitle>Region {region.region} — {region.region_name}</CardTitle>
					<a href="/settings" class="text-sm text-dark/60 hover:underline">
						Change
					</a>
				</div>
			</CardHeader>
			<CardBody>
				<div class="flex items-end gap-3">
					<label class="flex flex-col text-sm">
						<span class="text-dark/80">Window length</span>
						<input
							class="mt-1 rounded border border-light-300 bg-transparent px-2 py-1"
							bind:value={duration}
							placeholder="3h"
						/>
					</label>
					<Button onclick={loadCheapest}>Find cheapest</Button>
				</div>

				{#if cheapestError}
					<Alert class="mt-4" color="danger">{cheapestError}</Alert>
				{:else if cheapest}
					<div class="mt-4 rounded bg-light-100 p-4 text-sm">
						<p class="font-medium">
							Cheapest {duration} window: {new Date(cheapest.start).toLocaleString()} →
							{new Date(cheapest.end).toLocaleTimeString()}
						</p>
						<p class="text-dark/80">
							Mean {cheapest.mean_inc_vat_p_per_kwh.toFixed(2)} p/kWh (inc VAT)
						</p>
					</div>
				{/if}
			</CardBody>
		</Card>

		<Card>
			<CardHeader>
				<CardTitle>Published rates</CardTitle>
			</CardHeader>
			<CardBody>
				{#if chartError}
					<Alert color="danger">{chartError}</Alert>
				{:else if slots.length > 0}
					<RateChart {slots} />
				{:else}
					<p class="text-sm text-dark/60">No rates yet.</p>
				{/if}
			</CardBody>
		</Card>
	</div>
{/if}
