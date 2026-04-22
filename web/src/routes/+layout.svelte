<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { session, refreshSession, logout } from '$lib/session.svelte';

	let { children } = $props();

	onMount(async () => {
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
		{#if session.me}
			<nav class="flex gap-3 text-sm">
				<a href="/" class="hover:underline">Home</a>
				<a href="/plans" class="hover:underline">Charge plans</a>
				<a href="/subscriptions" class="hover:underline">Subscriptions</a>
				<a href="/settings" class="hover:underline">Settings</a>
				<button
					class="text-red-600 hover:underline"
					onclick={async () => {
						await logout();
						await goto('/login');
					}}
				>
					Logout
				</button>
			</nav>
		{/if}
	</header>

	<main>
		{#if !session.loaded}
			<p class="text-slate-500">Loading…</p>
		{:else}
			{@render children()}
		{/if}
	</main>
</div>
