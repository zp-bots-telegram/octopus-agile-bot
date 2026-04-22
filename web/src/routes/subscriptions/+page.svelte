<script lang="ts">
	import { onMount } from 'svelte';
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

{#if error}
	<p class="mb-4 text-sm text-danger-700 dark:text-danger-400">{error}</p>
{/if}
{#if saved}
	<p class="mb-4 text-sm text-success-700 dark:text-success-400">Saved.</p>
{/if}

<section class="rounded-lg border border-light-200 dark:border-dark-200 bg-light-50 dark:bg-dark-100 p-4">
	<p class="mb-4 text-sm text-dark/80 dark:text-light/80">
		Every day at the chosen local time, the bot will message you with the cheapest
		window of the given length over the next 24 hours.
	</p>
	<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_auto]">
		<label class="text-sm">
			<span class="text-dark/80 dark:text-light/80">Window length (minutes)</span>
			<input
				class="mt-1 w-full rounded border border-light-300 dark:border-dark-300 px-2 py-1"
				type="number"
				min="30"
				step="30"
				bind:value={durationMinutes}
			/>
		</label>
		<label class="text-sm">
			<span class="text-dark/80 dark:text-light/80">Notify at (HH:MM local)</span>
			<input
				class="mt-1 w-full rounded border border-light-300 dark:border-dark-300 px-2 py-1"
				bind:value={notifyAtLocal}
			/>
		</label>
		<div class="flex gap-2 self-end">
			<button class="rounded bg-primary-600 px-4 py-1.5 text-white hover:bg-primary-700" onclick={save}
				>Save</button
			>
			{#if sub}
				<button
					class="rounded border border-danger-300 dark:border-danger-700 px-4 py-1.5 text-danger-700 dark:text-danger-300 hover:bg-danger-50 dark:hover:bg-danger-900"
					onclick={remove}>Remove</button
				>
			{/if}
		</div>
	</div>
</section>
