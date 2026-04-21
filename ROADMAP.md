# Roadmap

Post-v1 work, grouped by theme. Each item is designed-for in the v1 architecture
(`internal/service` + `Notifier` interface): adding one should be mechanical, not a
rewrite.

## Web UI

- [ ] Add `internal/httpapi` with HTTP handlers that call the same `service.Service` methods the Telegram bot uses.
- [ ] Static frontend in `web/` (SvelteKit, matching the `discord-rpg-summariser` pattern) served by the Go binary.
- [ ] Telegram Login Widget + HMAC-signed session cookie for auth — avoids a second identity system.
- [ ] Push notifications: add a second `service.Notifier` implementation that fans out to active browser sessions (server-sent events or WebPush).

Open questions:
- Do we need a separate deployment for the web UI, or serve it from the same binary?
- How do we rate-limit the public web endpoints?

## More tariffs

- [ ] Expose a `service.Tariff` abstraction so Go, Cosy, Tracker can reuse the cheapest-window logic.
- [ ] Re-use `internal/octopus.Products()`; extend `LatestAgileProduct` to a generic `LatestProduct(kind)` or similar.
- [ ] Decide whether users pick a tariff per chat, or we auto-detect from their account (needs meter-point data — see below).

## Consumption data

- [ ] Add nullable `mpan`, `meter_serial` columns to `chats`.
- [ ] Add `GET /v1/electricity-meter-points/{mpan}/meters/{serial}/consumption/` to `internal/octopus`.
- [ ] Service methods: `ActualSpend(ctx, chatID, from, to)`, `MissedWindowAnalysis(...)`.
- [ ] Requires the Octopus API key to have account scope — document the difference between public and account-scoped keys.

## Smart-plug / MQTT control

- [ ] Define a `Controller` port alongside `Notifier`: `Start(ctx, chatID, deviceID)`, `Stop(ctx, chatID, deviceID)`.
- [ ] Implementations in `internal/mqtt`, `internal/tapo`, `internal/shelly`, …
- [ ] New commands `/device add`, `/device link <plan-id> <device-id>` to tie a charge plan to a physical switch.

## Packaging / ops

- [x] Multi-arch Docker image (linux/amd64, linux/arm64) — already covered by the release workflow.
- [ ] Prometheus metrics endpoint (request counts, dispatch successes/failures).
- [ ] Structured audit log of every outgoing message for debugging.

## Hygiene

- [ ] Full golangci-lint pass in CI (currently an optional local step — once the config stabilises, make it required).
- [ ] Replay-style integration test that drives `service.RefreshRates` then `service.DispatchTodaysChargePlans` against recorded Octopus fixtures.
