<script lang="ts">
	import { onMount } from 'svelte';
	import { Alert, Button, Card, CardBody, CardHeader, CardTitle } from '@immich/ui';
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

	let octopusLinked = $state(false);
	let octopusAccount = $state('');
	let octopusKey = $state('');
	let octopusInfo = $state<{ current_tariff: string; mpan: string; postcode: string } | null>(null);
	let octopusError = $state<string | null>(null);
	let octopusSaved = $state(false);

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

<div class="space-y-4">
	{#if error}
		<Alert color="danger">{error}</Alert>
	{/if}
	{#if saved}
		<Alert color="success">Saved.</Alert>
	{/if}

	<Card>
		<CardHeader>
			<CardTitle>DNO region</CardTitle>
		</CardHeader>
		<CardBody>
			{#if current}
				<p class="mb-4 text-sm text-dark/80">
					Currently <strong>{current.region}</strong> — {current.region_name}
				</p>
			{:else}
				<p class="mb-4 text-sm text-dark/80">No region set yet.</p>
			{/if}

			<div class="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-[1fr_auto] items-end">
				<label class="text-sm">
					<span class="text-dark/80">Postcode</span>
					<input
						class="mt-1 w-full rounded border border-light-300 bg-transparent px-2 py-1 uppercase"
						bind:value={postcode}
						placeholder="SW1A 1AA"
					/>
				</label>
				<Button onclick={savePostcode}>Look up</Button>
			</div>

			<details class="text-sm text-dark/80">
				<summary class="cursor-pointer">…or set the letter directly</summary>
				<div class="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-[1fr_auto] items-end">
					<label>
						<span class="text-dark/80">Letter (A–P)</span>
						<input
							class="mt-1 w-full rounded border border-light-300 bg-transparent px-2 py-1 uppercase"
							bind:value={letter}
							maxlength="1"
						/>
					</label>
					<Button variant="outline" color="secondary" onclick={saveLetter}>Save</Button>
				</div>
			</details>
		</CardBody>
	</Card>

	<Card>
		<CardHeader>
			<CardTitle>Price alert</CardTitle>
		</CardHeader>
		<CardBody>
			<p class="mb-3 text-sm text-dark/80">
				I'll message you ~10 minutes before a half-hour slot drops below this threshold
				(inc VAT, p/kWh). Use <strong>0</strong> to alert only on negative prices.
			</p>

			{#if alertError}
				<Alert class="mb-3" color="danger">{alertError}</Alert>
			{/if}
			{#if alertSaved}
				<Alert class="mb-3" color="success">Saved.</Alert>
			{/if}

			<p class="mb-3 text-sm">
				Currently:
				{#if alertEnabled}
					<strong>on</strong> — threshold {alertThreshold.toFixed(2)} p/kWh
				{:else}
					<strong>off</strong>
				{/if}
			</p>

			<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_auto_auto] items-end">
				<label class="text-sm">
					<span class="text-dark/80">Threshold (p/kWh)</span>
					<input
						class="mt-1 w-full rounded border border-light-300 bg-transparent px-2 py-1"
						type="number"
						step="0.1"
						bind:value={alertThreshold}
					/>
				</label>
				<Button onclick={saveAlert}>
					{alertEnabled ? 'Update' : 'Enable'}
				</Button>
				{#if alertEnabled}
					<Button color="danger" variant="outline" onclick={disableAlert}>Disable</Button>
				{/if}
			</div>
		</CardBody>
	</Card>

	<Card>
		<CardHeader>
			<CardTitle>Octopus account</CardTitle>
		</CardHeader>
		<CardBody>
			<p class="mb-3 text-sm text-dark/80">
				Link your Octopus account to unlock account-scoped features (current tariff,
				consumption history). Find your API key and account number at
				<a
					class="text-primary-600 dark:text-primary-400 underline"
					target="_blank"
					rel="noreferrer"
					href="https://octopus.energy/dashboard/new/accounts/personal-details/api-access"
				>
					octopus.energy → API access
				</a>.
			</p>

			{#if octopusError}
				<Alert class="mb-3" color="danger">{octopusError}</Alert>
			{/if}
			{#if octopusSaved}
				<Alert class="mb-3" color="success">Saved.</Alert>
			{/if}

			{#if octopusLinked}
				<p class="mb-3 text-sm">
					Linked: <strong>{octopusAccount}</strong>
					{#if octopusInfo}
						— tariff {octopusInfo.current_tariff || 'unknown'}, MPAN
						{octopusInfo.mpan || '—'}
					{/if}
				</p>
				<Button color="danger" variant="outline" onclick={unlinkOctopus}>Unlink</Button>
			{:else}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_auto] items-end">
					<label class="text-sm">
						<span class="text-dark/80">Account number</span>
						<input
							class="mt-1 w-full rounded border border-light-300 bg-transparent px-2 py-1"
							bind:value={octopusAccount}
							placeholder="A-XXXXXXXX"
						/>
					</label>
					<label class="text-sm">
						<span class="text-dark/80">API key</span>
						<input
							class="mt-1 w-full rounded border border-light-300 bg-transparent px-2 py-1"
							type="password"
							autocomplete="off"
							bind:value={octopusKey}
							placeholder="sk_live_…"
						/>
					</label>
					<Button onclick={linkOctopus}>Link</Button>
				</div>
			{/if}
		</CardBody>
	</Card>
</div>
