<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { ThemeSwitcher, themeManager } from '@immich/ui';
	import { session, refreshSession, logout } from '$lib/session.svelte';

	let { children } = $props();

	onMount(async () => {
		// themeManager persists the preference in localStorage but only syncs the DOM
		// on toggle; nudge it once so the correct `dark`/`light` class lands on <html>
		// on first paint.
		themeManager.setPreference(themeManager.preference);

		await refreshSession();
		const publicPaths = new Set(['/login']);
		if (!session.me && !publicPaths.has($page.url.pathname)) {
			await goto('/login');
		}
	});
</script>

<div class="mx-auto max-w-3xl p-4 sm:p-6">
	<header class="mb-6 flex items-center justify-between">
		<a href="/" class="text-lg font-semibold">⚡ Octopus Agile</a>
		<div class="flex items-center gap-3">
			{#if session.me}
				<nav class="flex gap-3 text-sm">
					<a href="/" class="hover:underline">Home</a>
					<a href="/plans" class="hover:underline">Charge plans</a>
					<a href="/subscriptions" class="hover:underline">Subscriptions</a>
					<a href="/settings" class="hover:underline">Settings</a>
					<button
						class="text-danger-700 dark:text-danger-400 hover:underline"
						onclick={async () => {
							await logout();
							await goto('/login');
						}}
					>
						Logout
					</button>
				</nav>
			{/if}
			<ThemeSwitcher size="small" />
		</div>
	</header>

	<main>
		{#if !session.loaded}
			<p class="text-dark/60 dark:text-light/60">Loading…</p>
		{:else}
			{@render children()}
		{/if}
	</main>
</div>
