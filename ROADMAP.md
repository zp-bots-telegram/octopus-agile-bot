# Roadmap

What's still parked vs. what already shipped. Each open item is "designed-for" by the
v1 architecture (`internal/service` + `Notifier`/`OctopusClient` interfaces), so adding
one is mechanical rather than a rewrite.

## Shipped

- [x] **Web UI** — `internal/httpapi` plus a SvelteKit (runes) + @immich/ui + Tailwind v4 frontend, embedded into the binary via `//go:embed`. Same `service.Service` as the Telegram transport.
- [x] **Web auth** — Login Widget for browsers, `window.Telegram.WebApp.initData` verification for the Mini App. HMAC-signed cookie sessions.
- [x] **Mini App integration** — `setChatMenuButton` publishes the web URL as the bot's "Open app" chip; `/web` command sends both a Mini App button and a plain link; theme follows Telegram's `colorScheme` automatically inside the WebApp.
- [x] **Postcode region lookup** — `/region <postcode>` resolves via the Octopus GSP endpoint.
- [x] **Negative-price alerts** — `/alerts <threshold|off>`, dispatched ~10 min before the start of a contiguous run below threshold.
- [x] **One-shot planner** — `/plan <duration> [by HH:MM]`, web Home card "Plan a charge now".
- [x] **Consumption data** — Octopus API key linking (per-user, AES-256-GCM at rest), MPAN/serial captured from the account on link, consumption browser on `/consumption`.
- [x] **Flexible time input** — every user-facing time field accepts `07:00` / `7:00` / `7am` / `7 pm` / `7:30 pm` and is normalised to `HH:MM` before storage.
- [x] **Multi-arch Docker** — release workflow builds linux/amd64 + linux/arm64.
- [x] **Dark mode** — bot icon + clock-ring artwork + Immich theme tokens auto-invert; uPlot axis colours bind to live CSS variables and re-render on theme toggle.

## Parked

### More tariffs

- [ ] Generalise `service.Tariff` so Go, Cosy, Tracker can reuse the cheapest-window logic.
- [ ] Extend `internal/octopus.LatestAgileProduct` to a `LatestProduct(kind)` selector.
- [ ] Decide UX: pick per chat, or auto-detect from the linked account's tariff?

### Smart-plug / MQTT control

- [ ] `Controller` port alongside `Notifier`: `Start(ctx, chatID, deviceID)`, `Stop(...)`.
- [ ] Concrete impls in `internal/mqtt`, `internal/tapo`, `internal/shelly`, …
- [ ] `/device add`, `/device link <plan-id> <device-id>` commands tying a charge plan to a physical switch.

### Push to web sessions

- [ ] Second `service.Notifier` impl that fans out to active browser sessions over server-sent events or WebPush. Right now charge-plan / alert messages only go to Telegram.

### Octopus OAuth

We initially planned an OAuth flow for "connect my account". Octopus does not publish
a public OAuth server; the standard third-party pattern is the personal API key
which is what the Settings → Octopus account flow now uses (encrypted at rest with
`ENCRYPTION_KEY`). If Octopus ever ships a real OAuth surface, swapping the key flow
for OAuth is a `service.LinkOctopusAccount` change.

### Ops / hygiene

- [ ] Prometheus `/metrics` endpoint (rate-refresh outcomes, dispatch counts, alert volume).
- [ ] Structured audit log of every outgoing message for debugging — currently emitted at debug-level only.
- [ ] Make `golangci-lint run` mandatory in CI once the config stabilises.
- [ ] Recorded-fixture medium test that drives `service.RefreshRates` → `DispatchTodaysChargePlans` against pinned Octopus payloads (complement to the live-tagged smoke test).
- [ ] Scheduler test: `AddSubscriptionJob` / `RemoveSubscriptionJob` bookkeeping and the retry/backoff path of `runRefreshAndDispatch`.
