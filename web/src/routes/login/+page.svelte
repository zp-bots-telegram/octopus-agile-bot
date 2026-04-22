<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, ApiError, type TelegramLoginPayload } from '$lib/api';
	import { session, refreshSession } from '$lib/session.svelte';

	const botUsername = import.meta.env.PUBLIC_TELEGRAM_BOT_USERNAME ?? 'octopus_energy_info_bot';

	let error = $state<string | null>(null);
	let widgetContainer = $state<HTMLDivElement | null>(null);

	onMount(() => {
		(window as unknown as { onTelegramAuth: (u: TelegramLoginPayload) => void }).onTelegramAuth =
			async (user) => {
				try {
					await api.telegramLogin(user);
					await refreshSession();
					await goto('/');
				} catch (e) {
					error = e instanceof ApiError ? e.message : String(e);
				}
			};

		const s = document.createElement('script');
		s.async = true;
		s.src = 'https://telegram.org/js/telegram-widget.js?22';
		s.setAttribute('data-telegram-login', botUsername);
		s.setAttribute('data-size', 'large');
		s.setAttribute('data-radius', '6');
		s.setAttribute('data-onauth', 'onTelegramAuth(user)');
		s.setAttribute('data-request-access', 'write');
		widgetContainer?.appendChild(s);
	});
</script>

<section class="mt-12 text-center">
	<h1 class="mb-2 text-3xl font-bold">Octopus Agile Bot</h1>
	<p class="mb-8 text-dark/80 dark:text-light/80">
		Find the cheapest times to use lots of electricity. Log in with the same Telegram account
		you use to chat with the bot.
	</p>

	{#if session.loaded && session.me}
		<p>
			Already signed in.
			<a class="text-primary-600 dark:text-primary-400 underline" href="/">Go home →</a>
		</p>
	{:else}
		<div class="flex flex-col items-center gap-4">
			<div bind:this={widgetContainer}></div>
			{#if error}
				<p class="text-sm text-danger-700 dark:text-danger-400">Login failed: {error}</p>
			{/if}
		</div>
	{/if}
</section>
