import { api, ApiError, type Me } from './api';

// A simple runes-based reactive store for the session. `loaded` flips to true once
// we've made at least one /api/me call so guards can distinguish "probably not
// logged in" from "we don't know yet".
export const session = $state<{
	me: Me | null;
	loaded: boolean;
}>({ me: null, loaded: false });

// tgInitData pulls the Telegram Mini App init string if the page is running inside
// Telegram (WebApp SDK injects window.Telegram.WebApp). Returns "" otherwise.
function tgInitData(): string {
	const t = (window as unknown as { Telegram?: { WebApp?: { initData?: string } } })
		.Telegram;
	return t?.WebApp?.initData ?? '';
}

export async function refreshSession() {
	try {
		session.me = await api.me();
	} catch (e) {
		if (!(e instanceof ApiError && e.status === 401)) {
			session.loaded = true;
			throw e;
		}
		// No existing cookie. If we're inside Telegram, auto-auth via initData so the
		// Mini App never shows the Login Widget.
		const init = tgInitData();
		if (init) {
			try {
				await api.telegramInitData(init);
				session.me = await api.me();
			} catch {
				session.me = null;
			}
		} else {
			session.me = null;
		}
	} finally {
		session.loaded = true;
	}
}

export async function logout() {
	await api.logout();
	session.me = null;
}
