# octopus-agile-bot

A Telegram bot that helps you time high-load appliances (EV charging, dishwasher,
washing machine, heat pump, …) against Octopus Energy's Agile half-hourly tariff,
plus a small web app exposing the same features.

## Features

### From Telegram

| Command | What it does |
|---|---|
| `/start`, `/help` | Onboarding and command list. |
| `/region <letter>` or `/region <postcode>` | Set your DNO region. Postcode is resolved through Octopus's public Grid Supply Point lookup. Region letters get a friendly name in replies (e.g. `A — Eastern England`). |
| `/cheapest <duration>` | Cheapest contiguous window of `<duration>` anywhere in the published horizon. |
| `/plan <duration> [by HH:MM]` | One-shot charge planner: when should I start charging right now to be done by `HH:MM`? Time accepts `07:00` / `7am` / `7:30 pm`. |
| `/charge <duration> <HH:MM>-<HH:MM>` | **Recurring** daily EV-charge plan inside an allowed window (e.g. `/charge 4h 22:00-07:00`). Once the daily 16:15 rate refresh runs, the bot sends a "start charging at …" message. Multiple plans per chat. |
| `/charges` / `/cancelcharge <id>` | List or cancel charge plans. |
| `/subscribe <duration> <HH:MM>` / `/unsubscribe` | Daily push of the cheapest `<duration>` window in the next 24 h, sent at the chosen local time. |
| `/next <threshold>` | Next half-hour at or under `<threshold>` p/kWh. |
| `/alerts <threshold|off>` | Notify ~10 minutes before a half-hour drops below `<threshold>` p/kWh (default `0` = negative prices only). |
| `/web` | Open the web UI as a Telegram Mini App (or in the browser). |
| `/status` | Show your region, subscription, charge plans, alert config. |

The bot also publishes a global menu button so the chat composer's "Open app" chip
launches the web UI as a Mini App without typing a command.

### From the web UI

Same surface as Telegram, plus:

- **Plan a charge now** — duration + optional "finish by" deadline → start time and mean price.
- **Published rates chart** — uPlot line+area chart of the cached half-hour rates, axes that auto-invert in dark mode.
- **Consumption** — per-half-hour, hourly, daily, weekly, monthly usage table with kWh totals (requires a linked Octopus account).
- **Settings** — region (postcode lookup or letter), price alert, link/unlink Octopus account.

Auth: in a normal browser the page renders the [Telegram Login
Widget](https://core.telegram.org/widgets/login). Inside the Telegram Mini App
container we instead verify `window.Telegram.WebApp.initData` server-side and skip
the widget — login is silent. Sessions are HMAC-signed cookies (Secure when
`WEB_BASE_URL` starts with `https://`). The app follows Telegram's `colorScheme`
when it's a Mini App; otherwise it uses the user's manual choice (persisted to
`localStorage`).

## Architecture

Hexagonal-ish: the domain logic lives in `internal/service` and is the only thing
either transport (Telegram or HTTP) ever calls.

```
cmd/bot/main.go        → wires config → storage → service → scheduler → telegram → httpapi
internal/agile         → pure domain (cheapest-window algos, region/tariff helpers)
internal/octopus       → REST client (products, rates, accounts, consumption, GSP lookup)
internal/storage       → SQLite (modernc.org/sqlite, pure-Go, no CGO) + embedded migrations
internal/service       → use-cases — every side effect crosses an interface defined here
internal/scheduler     → gocron jobs: 16:15 rate refresh, per-minute price-alert sweep,
                         per-user subscription pushes, post-refresh charge-plan dispatch
internal/telegram      → bot.go-telegram handlers; Notifier impl that wraps SendMessage
internal/httpapi       → net/http server with session middleware, embeds the Svelte build
internal/session       → HMAC-signed cookie helper
internal/tgauth        → verifies Login Widget + Mini App initData payloads
internal/cryptobox     → AES-256-GCM at rest for per-user Octopus API keys
internal/app           → top-level App struct: Start/Stop, lifecycle wiring
web/                   → SvelteKit (runes) + Tailwind v4 + @immich/ui + uPlot
```

Notable deliberate choices:

- **Pure-Go SQLite** so the Docker image stays on `distroless/static-debian12:nonroot` (~16 MB) with no CGO surface.
- **Same binary serves the web UI** — the SvelteKit static build is `//go:embed`-ed into `internal/httpapi/webassets`. One container, one process, one deploy.
- **Config — one env-var struct** in `internal/config` parsed via `caarlos0/env`; the HTTP API stays disabled if `SESSION_SECRET` is unset, so a Telegram-only deployment is the same binary minus two env vars.

## Configuration

| Variable | Required | Default | Notes |
|---|---|---|---|
| `TELEGRAM_BOT_TOKEN` | yes | — | From [@BotFather](https://t.me/botfather). |
| `OCTOPUS_API_KEY` | yes | — | Personal API key from `octopus.energy/dashboard/new/accounts/personal-details/api-access`. Used for the global tariff/rate fetches. |
| `DEFAULT_REGION` | no | `C` | Single letter A–P. |
| `DATABASE_PATH` | no | `/data/bot.db` | SQLite file. |
| `LOG_LEVEL` | no | `info` | `debug` \| `info` \| `warn` \| `error`. |
| `LOG_FORMAT` | no | `json` | `json` \| `text`. |
| `TZ` | no | `Europe/London` | Used for the daily refresh window. |
| `ALLOWED_CHAT_IDS` | no | empty | Comma-separated; empty means public. |
| `HTTP_LISTEN_ADDR` | no | `:8080` | Listens only when `SESSION_SECRET` is set. |
| `WEB_BASE_URL` | no | `http://localhost:8080` | Public URL of the web app. Used for cookie security flags, the `/web` reply, and the bot's menu button. |
| `SESSION_SECRET` | no | unset | ≥ 16 bytes. Required to enable the web UI. |
| `ENCRYPTION_KEY` | no | unset | Exactly 32 bytes (AES-256-GCM). Required for the "link Octopus account" flow. |

Telegram setup details (one-time):

1. Register the bot's domain with BotFather: `/setdomain` → pick the bot → paste the `WEB_BASE_URL` host. Required by both the Login Widget and the Mini App.
2. Upload the bot icon (`assets/bot-icon.png`) via BotFather: `/setuserpic` → pick the bot.

## Quick start

Local-only run with the web UI on `:8080`:

```bash
cp config.example.env .env
# Fill in TELEGRAM_BOT_TOKEN, OCTOPUS_API_KEY, SESSION_SECRET, ENCRYPTION_KEY in .env
make build
DATABASE_PATH=./data/bot.db ./bot
```

The Telegram Login Widget needs a public HTTPS host — for dev, `cloudflared
tunnel --url http://localhost:8080`, `ngrok http 8080`, or Tailscale Funnel are all
fine. Whatever URL the tunnel prints, set as `WEB_BASE_URL` and register with
BotFather's `/setdomain`.

Production: `docker build` (multi-arch via the release workflow) or pull the image
from `ghcr.io/zp-bots-telegram/octopus-agile-bot`. Mount a volume at `/data` for
the SQLite file. The release workflow tags semver / `{major}.{minor}` / sha /
`latest`.

## Development

| Command | What it does |
|---|---|
| `make build` | Frontend (`web/`) + Go binary, end-to-end. |
| `make build-go` | Just the Go binary (no frontend rebuild). |
| `make dev-web` | Vite dev server with proxy → `:8080`. |
| `make test` | Unit + medium tests. |
| `make lint` | `gofmt`, `go vet`, `golangci-lint` if installed. |
| `make docker` | `octopus-agile-bot:local` image. |

Tests:
- `internal/agile` — pure-domain table tests (cheapest window, daily-range, DST, threshold search).
- `internal/octopus` — `httptest`-driven, plus opt-in live tests behind `//go:build live`.
- `internal/storage` — real SQLite in `t.TempDir()`; covers every repo method.
- `internal/service` — wires the real store with fake octopus + notifier; covers refresh / charge dispatch / price-alert lead-window dedup.
- `internal/httpapi` — `httptest`, exercises the auth flow and every feature endpoint with a real session cookie.
- `internal/session` / `internal/tgauth` / `internal/cryptobox` — round-trip + tamper detection.

## Roadmap

See [ROADMAP.md](ROADMAP.md) for what's deliberately out of scope today.
