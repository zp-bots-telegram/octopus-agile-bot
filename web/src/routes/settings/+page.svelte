<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError, type RegionResp } from '$lib/api';

	let current = $state<RegionResp | null>(null);
	let letter = $state('');
	let postcode = $state('');
	let error = $state<string | null>(null);
	let saved = $state(false);

	async function load() {
		try {
			current = await api.getRegion();
			letter = current?.region ?? '';
		} catch (e) {
			if (!(e instanceof ApiError && e.status === 428)) {
				error = e instanceof ApiError ? e.message : String(e);
			}
		}
	}

	async function saveLetter() {
		error = null;
		saved = false;
		try {
			current = await api.setRegion(letter.trim().toUpperCase());
			saved = true;
		} catch (e) {
			error = e instanceof ApiError ? e.message : String(e);
		}
	}

	async function savePostcode() {
		error = null;
		saved = false;
		try {
			current = await api.setRegionByPostcode(postcode);
			letter = current.region;
			saved = true;
		} catch (e) {
			error = e instanceof ApiError ? e.message : String(e);
		}
	}

	onMount(load);
</script>

<h2 class="mb-4 text-xl font-semibold">Settings</h2>

{#if error}
	<p class="mb-3 text-sm text-red-600">{error}</p>
{/if}
{#if saved}
	<p class="mb-3 text-sm text-green-600">Saved.</p>
{/if}

<section class="rounded-lg border border-slate-200 bg-white p-4">
	<h3 class="mb-2 font-semibold">DNO region</h3>
	{#if current}
		<p class="mb-4 text-sm text-slate-600">
			Currently <strong>{current.region}</strong> — {current.region_name}
		</p>
	{:else}
		<p class="mb-4 text-sm text-slate-600">No region set yet.</p>
	{/if}

	<div class="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-[1fr_auto]">
		<label class="text-sm">
			<span class="text-slate-600">Postcode</span>
			<input
				class="mt-1 w-full rounded border border-slate-300 px-2 py-1 uppercase"
				bind:value={postcode}
				placeholder="SW1A 1AA"
			/>
		</label>
		<button
			class="self-end rounded bg-blue-600 px-4 py-1.5 text-white hover:bg-blue-700"
			onclick={savePostcode}
		>
			Look up
		</button>
	</div>

	<details class="text-sm text-slate-600">
		<summary class="cursor-pointer">…or set the letter directly</summary>
		<div class="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-[1fr_auto]">
			<label>
				<span class="text-slate-600">Letter (A–P)</span>
				<input
					class="mt-1 w-full rounded border border-slate-300 px-2 py-1 uppercase"
					bind:value={letter}
					maxlength="1"
				/>
			</label>
			<button
				class="self-end rounded border border-slate-300 px-4 py-1.5 hover:bg-slate-50"
				onclick={saveLetter}
			>
				Save
			</button>
		</div>
	</details>
</section>
