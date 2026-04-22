import { api, ApiError, type Me } from './api';

// A simple runes-based reactive store for the session. `loaded` flips to true once
// we've made at least one /api/me call so guards can distinguish "probably not
// logged in" from "we don't know yet".
export const session = $state<{
	me: Me | null;
	loaded: boolean;
}>({ me: null, loaded: false });

export async function refreshSession() {
	try {
		session.me = await api.me();
	} catch (e) {
		if (e instanceof ApiError && e.status === 401) {
			session.me = null;
		} else {
			throw e;
		}
	} finally {
		session.loaded = true;
	}
}

export async function logout() {
	await api.logout();
	session.me = null;
}
