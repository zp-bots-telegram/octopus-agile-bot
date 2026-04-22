<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError, type RegionResp } from '$lib/api';

	let current = $state<RegionResp | null>(null);
	let letter = $state('');
	let postcode = $state('');
	let error = $state<string | null>(null);
	let saved = $state(false);

	let alertEnabled = $state(false);
	let alertThreshold = $state(0);
	let alertError = $state<string | null>(null);
	let alertSaved = $state(false);

	async function load() {
		try {
			current = await api.getRegion();
			letter = current?.region ?? '';
		} catch (e) {
			if (!(e instanceof ApiError && e.status === 428)) {
				error = e instanceof ApiError ? e.message : String(e);
			}
		}
		try {
			const a = await api.getAlert();
			if (a) {
				alertEnabled = a.enabled;
				alertThreshold = a.threshold_inc_vat;
			} else {
				alertEnabled = false;
				alertThreshold = 0;
			}
		} catch (e) {
			alertError = e instanceof ApiError ? e.message : String(e);
		}
	}

	async function saveAlert() {
		alertError = null;
		alertSaved = false;
		try {
			await api.putAlert(alertThreshold);
			alertEnabled = true;
			alertSaved = true;
		} catch (e) {
			alertError = e instanceof ApiError ? e.message : String(e);
		}
	}

	async function disableAlert() {
		alertError = null;
		try {
			await api.deleteAlert();
			alertEnabled = false;
			alertSaved = true;
		} catch (e) {
			alertError = e instanceof ApiError ? e.message : String(e);
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

	let octopusLinked = $state(false);
	let octopusAccount = $state('');
	let octopusKey = $state('');
	let octopusInfo = $state<{ current_tariff: string; mpan: string; postcode: string } | null>(null);
	let octopusError = $state<string | null>(null);
	let octopusSaved = $state(false);

	async function loadOctopus() {
		try {
			const r = await api.getOctopus();
			octopusLinked = r.linked;
			octopusAccount = r.account_number;
		} catch (e) {
			octopusError = e instanceof ApiError ? e.message : String(e);
		}
	}

	async function linkOctopus() {
		octopusError = null;
		octopusSaved = false;
		try {
			const info = await api.linkOctopus(octopusAccount.trim(), octopusKey.trim());
			octopusInfo = info;
			octopusKey = '';
			octopusLinked = true;
			octopusSaved = true;
		} catch (e) {
			octopusError = e instanceof ApiError ? e.message : String(e);
		}
	}

	async function unlinkOctopus() {
		octopusError = null;
		try {
			await api.unlinkOctopus();
			octopusLinked = false;
			octopusAccount = '';
			octopusInfo = null;
			octopusSaved = true;
		} catch (e) {
			octopusError = e instanceof ApiError ? e.message : String(e);
		}
	}

	onMount(async () => {
		await load();
		await loadOctopus();
	});
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

<section class="mt-6 rounded-lg border border-slate-200 bg-white p-4">
	<h3 class="mb-2 font-semibold">Price alert</h3>
	<p class="mb-3 text-sm text-slate-600">
		I'll message you ~10 minutes before a half-hour slot drops below this threshold
		(inc VAT, p/kWh). Use <strong>0</strong> to alert only on negative prices.
	</p>

	{#if alertError}
		<p class="mb-2 text-sm text-red-600">{alertError}</p>
	{/if}
	{#if alertSaved}
		<p class="mb-2 text-sm text-green-600">Saved.</p>
	{/if}

	<p class="mb-3 text-sm">
		Currently:
		{#if alertEnabled}
			<strong>on</strong> — threshold {alertThreshold.toFixed(2)} p/kWh
		{:else}
			<strong>off</strong>
		{/if}
	</p>

	<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_auto_auto]">
		<label class="text-sm">
			<span class="text-slate-600">Threshold (p/kWh)</span>
			<input
				class="mt-1 w-full rounded border border-slate-300 px-2 py-1"
				type="number"
				step="0.1"
				bind:value={alertThreshold}
			/>
		</label>
		<button
			class="self-end rounded bg-blue-600 px-4 py-1.5 text-white hover:bg-blue-700"
			onclick={saveAlert}
		>
			{alertEnabled ? 'Update' : 'Enable'}
		</button>
		{#if alertEnabled}
			<button
				class="self-end rounded border border-red-300 px-4 py-1.5 text-red-700 hover:bg-red-50"
				onclick={disableAlert}
			>
				Disable
			</button>
		{/if}
	</div>
</section>

<section class="mt-6 rounded-lg border border-slate-200 bg-white p-4">
	<h3 class="mb-2 font-semibold">Octopus account</h3>
	<p class="mb-3 text-sm text-slate-600">
		Link your Octopus account to unlock account-scoped features (current tariff,
		upcoming: consumption history). Find your API key at
		<a
			class="text-blue-600 underline"
			target="_blank"
			rel="noreferrer"
			href="https://octopus.energy/dashboard/new/accounts/personal-details/api-access"
			>octopus.energy → API access</a
		>
		and your account number (A-XXXXXXXX) on the same page.
	</p>

	{#if octopusError}
		<p class="mb-2 text-sm text-red-600">{octopusError}</p>
	{/if}
	{#if octopusSaved}
		<p class="mb-2 text-sm text-green-600">Saved.</p>
	{/if}

	{#if octopusLinked}
		<p class="mb-3 text-sm">
			Linked: <strong>{octopusAccount}</strong>
			{#if octopusInfo}
				— tariff {octopusInfo.current_tariff || 'unknown'}, MPAN {octopusInfo.mpan || '—'}
			{/if}
		</p>
		<button
			class="rounded border border-red-300 px-4 py-1.5 text-red-700 hover:bg-red-50"
			onclick={unlinkOctopus}
		>
			Unlink
		</button>
	{:else}
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_auto]">
			<label class="text-sm">
				<span class="text-slate-600">Account number</span>
				<input
					class="mt-1 w-full rounded border border-slate-300 px-2 py-1 uppercase"
					bind:value={octopusAccount}
					placeholder="A-XXXXXXXX"
				/>
			</label>
			<label class="text-sm">
				<span class="text-slate-600">API key</span>
				<input
					class="mt-1 w-full rounded border border-slate-300 px-2 py-1"
					type="password"
					autocomplete="off"
					bind:value={octopusKey}
					placeholder="sk_live_…"
				/>
			</label>
			<button
				class="self-end rounded bg-blue-600 px-4 py-1.5 text-white hover:bg-blue-700"
				onclick={linkOctopus}
			>
				Link
			</button>
		</div>
	{/if}
</section>
