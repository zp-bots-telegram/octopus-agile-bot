<script lang="ts">
	import { onMount } from 'svelte';
	import { Alert, Button, Card, CardBody, CardHeader, CardTitle } from '@immich/ui';
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

<h2 class="mb-4 text-xl font-semibold">Daily cheapest-window notification</h2>

<div class="space-y-4">
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
			<p class="mb-4 text-sm text-dark/80 dark:text-light/80">
				Every day at the chosen local time, the bot will message you with the cheapest window
				of the given length over the next 24 hours.
			</p>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_auto] items-end">
				<label class="text-sm">
					<span class="text-dark/80 dark:text-light/80">Window length (minutes)</span>
					<input
						class="mt-1 w-full rounded border border-light-300 dark:border-dark-300 bg-transparent px-2 py-1"
						type="number"
						min="30"
						step="30"
						bind:value={durationMinutes}
					/>
				</label>
				<label class="text-sm">
					<span class="text-dark/80 dark:text-light/80">Notify at (HH:MM local)</span>
					<input
						class="mt-1 w-full rounded border border-light-300 dark:border-dark-300 bg-transparent px-2 py-1"
						bind:value={notifyAtLocal}
					/>
				</label>
				<div class="flex gap-2">
					<Button onclick={save}>Save</Button>
					{#if sub}
						<Button color="danger" variant="outline" onclick={remove}>Remove</Button>
					{/if}
				</div>
			</div>
		</CardBody>
	</Card>
</div>
