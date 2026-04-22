<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { Button, Link, ThemeSwitcher, themeManager } from '@immich/ui';
	import { session, refreshSession, logout } from '$lib/session.svelte';

	let { children } = $props();

	onMount(async () => {
		themeManager.setPreference(themeManager.preference);

		await refreshSession();
		const publicPaths = new Set(['/login']);
		if (!session.me && !publicPaths.has($page.url.pathname)) {
			await goto('/login');
		}
	});
</script>

<div class="mx-auto max-w-3xl p-4 sm:p-6">
	<header class="mb-6 flex flex-wrap items-center justify-between gap-3">
		<a href="/" class="text-lg font-semibold">⚡ Octopus Agile</a>
		<div class="flex items-center gap-2 flex-wrap">
			{#if session.me}
				<nav class="flex items-center gap-1 text-sm">
					<Link href="/" underline={false}>Home</Link>
					<Link href="/plans" underline={false}>Charge plans</Link>
					<Link href="/subscriptions" underline={false}>Subscriptions</Link>
					<Link href="/consumption" underline={false}>Consumption</Link>
					<Link href="/settings" underline={false}>Settings</Link>
				</nav>
				<Button
					size="small"
					color="danger"
					variant="ghost"
					onclick={async () => {
						await logout();
						await goto('/login');
					}}
				>
					Logout
				</Button>
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
