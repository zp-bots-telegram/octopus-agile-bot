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
	<p class="mb-4 text-sm text-red-600">{error}</p>
{/if}
{#if saved}
	<p class="mb-4 text-sm text-green-600">Saved.</p>
{/if}

<section class="rounded-lg border border-slate-200 bg-white p-4">
	<p class="mb-4 text-sm text-slate-600">
		Every day at the chosen local time, the bot will message you with the cheapest
		window of the given length over the next 24 hours.
	</p>
	<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_auto]">
		<label class="text-sm">
			<span class="text-slate-600">Window length (minutes)</span>
			<input
				class="mt-1 w-full rounded border border-slate-300 px-2 py-1"
				type="number"
				min="30"
				step="30"
				bind:value={durationMinutes}
			/>
		</label>
		<label class="text-sm">
			<span class="text-slate-600">Notify at (HH:MM local)</span>
			<input
				class="mt-1 w-full rounded border border-slate-300 px-2 py-1"
				bind:value={notifyAtLocal}
			/>
		</label>
		<div class="flex gap-2 self-end">
			<button class="rounded bg-blue-600 px-4 py-1.5 text-white hover:bg-blue-700" onclick={save}
				>Save</button
			>
			{#if sub}
				<button
					class="rounded border border-red-300 px-4 py-1.5 text-red-700 hover:bg-red-50"
					onclick={remove}>Remove</button
				>
			{/if}
		</div>
	</div>
</section>
