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
	import { api, ApiError, type Subscription } from '$lib/api';

	let sub = $state<Subscription>(null);
	let durationMinutes = $state(180);
	let notifyAtLocal = $state('08:00');
	let error = $state<string | null>(null);
	let saved = $state(false);

	async function load() {
		try {
			sub = await api.getSubscription();
			if (sub) {
				durationMinutes = sub.duration_minutes;
				notifyAtLocal = sub.notify_at_local;
			}
		} catch (e) {
			error = e instanceof ApiError ? e.message : String(e);
		}
	}

	async function save() {
		error = null;
		saved = false;
		try {
			await api.putSubscription(durationMinutes, notifyAtLocal);
			saved = true;
			await load();
		} catch (e) {
			error = e instanceof ApiError ? e.message : String(e);
		}
	}

	async function remove() {
		error = null;
		try {
			await api.deleteSubscription();
			await load();
		} catch (e) {
			error = e instanceof ApiError ? e.message : String(e);
		}
	}

	onMount(load);
</script>

<Stack gap={4}>
	<Heading tag="h2" size="medium">Daily cheapest-window notification</Heading>

	{#if error}
		<Alert color="danger">{error}</Alert>
	{/if}
	{#if saved}
		<Alert color="success">Saved.</Alert>
	{/if}

	<Card>
		<CardHeader>
			<CardTitle>Subscription</CardTitle>
		</CardHeader>
		<CardBody>
			<Stack gap={4}>
				<Text color="muted">
					Every day at the chosen local time, the bot will message you with the cheapest window
					of the given length over the next 24 hours.
				</Text>
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_auto] items-end">
					<Field label="Window length (minutes)">
						<NumberInput bind:value={durationMinutes} min={30} step={30} />
					</Field>
					<Field label="Notify at (HH:MM local)">
						<Input bind:value={notifyAtLocal} />
					</Field>
					<HStack gap={2}>
						<Button onclick={save}>Save</Button>
						{#if sub}
							<Button color="danger" variant="outline" onclick={remove}>Remove</Button>
						{/if}
					</HStack>
				</div>
			</Stack>
		</CardBody>
	</Card>
</Stack>
