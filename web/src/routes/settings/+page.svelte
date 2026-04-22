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
		Link,
		NumberInput,
		PasswordInput,
		Stack,
		Text
	} from '@immich/ui';
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

<Stack gap={4}>
	<Heading tag="h2" size="medium">Settings</Heading>

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
			<Stack gap={4}>
				{#if current}
					<Text>
						Currently <strong>{current.region}</strong> — {current.region_name}
					</Text>
				{:else}
					<Text color="muted">No region set yet.</Text>
				{/if}

				<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_auto] items-end">
					<Field label="Postcode">
						<Input bind:value={postcode} placeholder="SW1A 1AA" />
					</Field>
					<Button onclick={savePostcode}>Look up</Button>
				</div>

				<details class="text-sm">
					<summary class="cursor-pointer text-dark/80 dark:text-light/80">
						…or set the letter directly
					</summary>
					<div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-[1fr_auto] items-end">
						<Field label="Letter (A–P)">
							<Input bind:value={letter} maxlength={1} />
						</Field>
						<Button variant="outline" color="secondary" onclick={saveLetter}>Save</Button>
					</div>
				</details>
			</Stack>
		</CardBody>
	</Card>

	<Card>
		<CardHeader>
			<CardTitle>Price alert</CardTitle>
		</CardHeader>
		<CardBody>
			<Stack gap={3}>
				<Text color="muted">
					I'll message you ~10 minutes before a half-hour slot drops below this threshold
					(inc VAT, p/kWh). Use <strong>0</strong> to alert only on negative prices.
				</Text>

				{#if alertError}
					<Alert color="danger">{alertError}</Alert>
				{/if}
				{#if alertSaved}
					<Alert color="success">Saved.</Alert>
				{/if}

				<Text>
					Currently:
					{#if alertEnabled}
						<strong>on</strong> — threshold {alertThreshold.toFixed(2)} p/kWh
					{:else}
						<strong>off</strong>
					{/if}
				</Text>

				<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_auto_auto] items-end">
					<Field label="Threshold (p/kWh)">
						<NumberInput bind:value={alertThreshold} step={0.1} />
					</Field>
					<Button onclick={saveAlert}>
						{alertEnabled ? 'Update' : 'Enable'}
					</Button>
					{#if alertEnabled}
						<Button color="danger" variant="outline" onclick={disableAlert}>Disable</Button>
					{/if}
				</div>
			</Stack>
		</CardBody>
	</Card>

	<Card>
		<CardHeader>
			<CardTitle>Octopus account</CardTitle>
		</CardHeader>
		<CardBody>
			<Stack gap={3}>
				<Text color="muted">
					Link your Octopus account to unlock account-scoped features (current tariff,
					consumption history). Find your API key and account number at
					<Link
						href="https://octopus.energy/dashboard/new/accounts/personal-details/api-access"
					>
						octopus.energy → API access
					</Link>.
				</Text>

				{#if octopusError}
					<Alert color="danger">{octopusError}</Alert>
				{/if}
				{#if octopusSaved}
					<Alert color="success">Saved.</Alert>
				{/if}

				{#if octopusLinked}
					<Text>
						Linked: <strong>{octopusAccount}</strong>
						{#if octopusInfo}
							— tariff {octopusInfo.current_tariff || 'unknown'}, MPAN
							{octopusInfo.mpan || '—'}
						{/if}
					</Text>
					<div>
						<Button color="danger" variant="outline" onclick={unlinkOctopus}>Unlink</Button>
					</div>
				{:else}
					<div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_auto] items-end">
						<Field label="Account number">
							<Input bind:value={octopusAccount} placeholder="A-XXXXXXXX" />
						</Field>
						<Field label="API key">
							<PasswordInput bind:value={octopusKey} placeholder="sk_live_…" />
						</Field>
						<Button onclick={linkOctopus}>Link</Button>
					</div>
				{/if}
			</Stack>
		</CardBody>
	</Card>
</Stack>
