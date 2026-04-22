<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { Alert, Heading, Link, Stack, Text } from '@immich/ui';
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

<section class="mt-12">
	<Stack direction="column" align="center" gap={6}>
		<Heading size="large" tag="h1">Octopus Agile Bot</Heading>
		<Text color="muted" class="text-center">
			Find the cheapest times to use lots of electricity. Log in with the same Telegram account
			you use to chat with the bot.
		</Text>

		{#if session.loaded && session.me}
			<Text>Already signed in. <Link href="/">Go home →</Link></Text>
		{:else}
			<div bind:this={widgetContainer}></div>
			{#if error}
				<Alert color="danger" title="Login failed">{error}</Alert>
			{/if}
		{/if}
	</Stack>
</section>
