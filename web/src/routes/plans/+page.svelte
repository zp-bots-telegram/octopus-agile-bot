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
		Heading,
		Input,
		NumberInput,
		Stack,
		Text
	} from '@immich/ui';
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
		const mins = nsOrMin >= 60_000_000 ? Math.round(nsOrMin / 60_000_000_000) * 60 : nsOrMin;
		const h = Math.floor(mins / 60);
		const m = mins % 60;
		if (h === 0) return `${m}m`;
		if (m === 0) return `${h}h`;
		return `${h}h${String(m).padStart(2, '0')}m`;
	}

	onMount(load);
</script>

<Stack gap={4}>
	<Heading tag="h2" size="medium">Charge plans</Heading>

	{#if error}
		<Alert color="danger">{error}</Alert>
	{/if}

	<Card>
		<CardHeader>
			<CardTitle>Add a plan</CardTitle>
		</CardHeader>
		<CardBody>
			<Stack gap={3}>
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_1fr_auto] items-end">
					<Field label="Duration (minutes)">
						<NumberInput bind:value={durationMinutes} min={30} step={30} />
					</Field>
					<Field label="Earliest start (HH:MM)">
						<Input bind:value={start} placeholder="22:00" />
					</Field>
					<Field label="Latest end (HH:MM)">
						<Input bind:value={end} placeholder="07:00" />
					</Field>
					<Button onclick={create}>Add</Button>
				</div>
				<Text color="muted" size="tiny">
					End earlier than start means overnight (e.g. 22:00–07:00 = 9h window crossing midnight).
				</Text>
			</Stack>
		</CardBody>
	</Card>

	{#if plans.length === 0}
		<Text color="muted">No charge plans yet.</Text>
	{:else}
		<Stack gap={2}>
			{#each plans as p}
				<Card>
					<CardBody>
						<HStack class="justify-between">
							<Stack gap={1}>
								<Text fontWeight="medium">
									#{p.ID} — {humanDuration(p.Duration)} between {p.WindowStartLocal}–{p.WindowEndLocal}
								</Text>
								<Text color="muted" size="small">
									{p.Enabled ? 'Active' : 'Paused'}
								</Text>
							</Stack>
							<Button size="small" color="danger" variant="outline" onclick={() => cancel(p.ID)}>
								Cancel
							</Button>
						</HStack>
					</CardBody>
				</Card>
			{/each}
		</Stack>
	{/if}
</Stack>
