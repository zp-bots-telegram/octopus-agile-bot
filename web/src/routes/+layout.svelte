<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import {
		Button,
		ThemePreference,
		ThemeSwitcher,
		TooltipProvider,
		themeManager
	} from '@immich/ui';
	import { session, refreshSession, logout } from '$lib/session.svelte';

	let { children } = $props();

	// Telegram Mini App runtime shape — only the bits we use.
	type TelegramWebApp = {
		colorScheme?: 'light' | 'dark';
		onEvent?: (type: string, cb: () => void) => void;
	};

	function telegramWebApp(): TelegramWebApp | undefined {
		return (window as unknown as { Telegram?: { WebApp?: TelegramWebApp } }).Telegram?.WebApp;
	}

	// Inside the Mini App, mirror Telegram's colour scheme and follow its themeChanged
	// event. Outside Telegram this is a no-op; the user's own preference (persisted in
	// localStorage by themeManager) stays in charge.
	function bindTelegramTheme() {
		const tg = telegramWebApp();
		if (!tg) return;
		const apply = () => {
			themeManager.setPreference(
				tg.colorScheme === 'dark' ? ThemePreference.Dark : ThemePreference.Light
			);
		};
		apply();
		tg.onEvent?.('themeChanged', apply);
	}

	onMount(async () => {
		themeManager.setPreference(themeManager.preference);
		bindTelegramTheme();

		await refreshSession();
		const publicPaths = new Set(['/login']);
		if (!session.me && !publicPaths.has($page.url.pathname)) {
			await goto('/login');
		}
	});
</script>

<TooltipProvider>
	<div class="mx-auto max-w-3xl p-4 sm:p-6">
		<header class="mb-6 flex flex-wrap items-center justify-between gap-3">
			<a href="/" class="text-lg font-semibold">⚡ Octopus Agile</a>
			<div class="flex items-center gap-3 flex-wrap">
				{#if session.me}
					<nav class="flex gap-3 text-sm">
						<a href="/" class="hover:underline">Home</a>
						<a href="/plans" class="hover:underline">Charge plans</a>
						<a href="/subscriptions" class="hover:underline">Subscriptions</a>
						<a href="/consumption" class="hover:underline">Consumption</a>
						<a href="/settings" class="hover:underline">Settings</a>
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
				<p class="text-dark/60">Loading…</p>
			{:else}
				{@render children()}
			{/if}
		</main>
	</div>
</TooltipProvider>
