// Small typed client for the Go HTTP API. Every call sends the session cookie via
// credentials: 'include'. Throws ApiError with the server-provided message on non-2xx.

export class ApiError extends Error {
	status: number;
	constructor(status: number, message: string) {
		super(message);
		this.status = status;
	}
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
	const resp = await fetch(path, {
		method,
		credentials: 'include',
		headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
		body: body !== undefined ? JSON.stringify(body) : undefined
	});
	if (!resp.ok) {
		let msg = resp.statusText;
		try {
			const j = await resp.json();
			if (j?.error) msg = j.error;
		} catch {
			// not json
		}
		throw new ApiError(resp.status, msg);
	}
	if (resp.status === 204) return undefined as T;
	return (await resp.json()) as T;
}

// --- types ----------------------------------------------------------------

export type Me = { telegram_user_id: number };
export type RegionResp = { region: string; region_name: string; timezone?: string };

export type Slot = {
	valid_from: string;
	valid_to: string;
	inc_vat_p_per_kwh: number;
	exc_vat_p_per_kwh: number;
};

export type Window = {
	start: string;
	end: string;
	mean_inc_vat_p_per_kwh: number;
	slots: Slot[];
};

export type Subscription = { duration_minutes: number; notify_at_local: string } | null;

export type ChargePlan = {
	ID: number;
	ChatID: number;
	Duration: number; // nanoseconds from Go
	WindowStartLocal: string;
	WindowEndLocal: string;
	Enabled: boolean;
};

export type Status = {
	Region: string;
	Timezone: string;
	Subscription: { ChatID: number; Duration: number; NotifyAtLocal: string; Enabled: boolean } | null;
	ChargePlans: ChargePlan[];
};

export type TelegramLoginPayload = {
	id: number;
	first_name?: string;
	last_name?: string;
	username?: string;
	photo_url?: string;
	auth_date: number;
	hash: string;
};

// --- api ------------------------------------------------------------------

export const api = {
	me: () => request<Me>('GET', '/api/me'),
	logout: () => request<void>('POST', '/api/auth/logout'),
	telegramLogin: (p: TelegramLoginPayload) =>
		request<Me & { first_name?: string; username?: string }>(
			'POST',
			'/api/auth/telegram/callback',
			p
		),

	getRegion: () => request<RegionResp>('GET', '/api/region'),
	setRegion: (region: string) => request<RegionResp>('PUT', '/api/region', { region }),
	setRegionByPostcode: (postcode: string) =>
		request<RegionResp>('PUT', '/api/region', { postcode }),

	cheapest: (duration: string) => request<Window>('GET', `/api/cheapest?duration=${duration}`),
	next: (threshold: number) => request<Slot>('GET', `/api/next?threshold=${threshold}`),
	rates: (fromISO?: string, toISO?: string) => {
		const q = new URLSearchParams();
		if (fromISO) q.set('from', fromISO);
		if (toISO) q.set('to', toISO);
		const qs = q.toString();
		return request<Slot[]>('GET', `/api/rates${qs ? '?' + qs : ''}`);
	},
	status: () => request<Status>('GET', '/api/status'),

	getSubscription: () => request<Subscription>('GET', '/api/subscription'),
	putSubscription: (durationMinutes: number, notifyAtLocal: string) =>
		request<void>('PUT', '/api/subscription', {
			duration_minutes: durationMinutes,
			notify_at_local: notifyAtLocal
		}),
	deleteSubscription: () => request<void>('DELETE', '/api/subscription'),

	getAlert: () =>
		request<{ threshold_inc_vat: number; enabled: boolean } | null>('GET', '/api/alert'),
	putAlert: (thresholdIncVAT: number) =>
		request<void>('PUT', '/api/alert', { threshold_inc_vat: thresholdIncVAT }),
	deleteAlert: () => request<void>('DELETE', '/api/alert'),

	listChargePlans: () => request<ChargePlan[]>('GET', '/api/charge-plans'),
	createChargePlan: (
		durationMinutes: number,
		windowStartLocal: string,
		windowEndLocal: string
	) =>
		request<ChargePlan>('POST', '/api/charge-plans', {
			duration_minutes: durationMinutes,
			window_start_local: windowStartLocal,
			window_end_local: windowEndLocal
		}),
	cancelChargePlan: (id: number) => request<void>('DELETE', `/api/charge-plans/${id}`)
};
