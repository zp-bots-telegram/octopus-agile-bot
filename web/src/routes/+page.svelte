<script lang="ts">
	import { onMount } from 'svelte';
	import {
		Alert,
		Button,
		Card,
		CardBody,
		CardHeader,
		CardTitle,
		Field,
		HStack,
		Input,
		Link,
		Stack,
		Text
	} from '@immich/ui';
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
	<Text>Not signed in. <Link href="/login">Sign in →</Link></Text>
{:else if !region}
	<Card>
		<CardHeader>
			<CardTitle>Welcome!</CardTitle>
		</CardHeader>
		<CardBody>
			<Stack gap={4}>
				<Text>Set your DNO region before we can find cheap slots for you.</Text>
				<div>
					<Button href="/settings">Go to Settings</Button>
				</div>
			</Stack>
		</CardBody>
	</Card>
{:else}
	<Stack gap={6}>
		<Card>
			<CardHeader>
				<div class="flex w-full items-center justify-between">
					<CardTitle>Region {region.region} — {region.region_name}</CardTitle>
					<Link href="/settings" class="text-sm">Change</Link>
				</div>
			</CardHeader>
			<CardBody>
				<Stack gap={4}>
					<HStack gap={3} class="items-end">
						<Field label="Window length">
							<Input bind:value={duration} placeholder="3h" />
						</Field>
						<Button onclick={loadCheapest}>Find cheapest</Button>
					</HStack>

					{#if cheapestError}
						<Alert color="danger" title="Couldn't find a window">{cheapestError}</Alert>
					{:else if cheapest}
						<Alert color="info" icon={false}>
							<Stack gap={1}>
								<Text fontWeight="medium">
									Cheapest {duration} window: {new Date(cheapest.start).toLocaleString()} →
									{new Date(cheapest.end).toLocaleTimeString()}
								</Text>
								<Text color="muted" size="small">
									Mean {cheapest.mean_inc_vat_p_per_kwh.toFixed(2)} p/kWh (inc VAT)
								</Text>
							</Stack>
						</Alert>
					{/if}
				</Stack>
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
					<Text color="muted" size="small">No rates yet.</Text>
				{/if}
			</CardBody>
		</Card>
	</Stack>
{/if}
