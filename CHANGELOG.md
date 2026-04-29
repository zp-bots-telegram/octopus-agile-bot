# Changelog

## [1.0.3](https://github.com/zp-bots-telegram/octopus-agile-bot/compare/v1.0.2...v1.0.3) (2026-04-29)


### Bug Fixes

* switch runtime image to alpine with uid 1000 ([70dd140](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/70dd1407990ab108e05d94ea735720f3da4d1b0a))

## [1.0.2](https://github.com/zp-bots-telegram/octopus-agile-bot/compare/v1.0.1...v1.0.2) (2026-04-27)


### Bug Fixes

* rebuild lockfile against npm 11.13 + switch dockerfile to node:24-slim ([1269b27](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/1269b27601260d737046c044a131c32d5f564370))

## [1.0.1](https://github.com/zp-bots-telegram/octopus-agile-bot/compare/v1.0.0...v1.0.1) (2026-04-27)


### Bug Fixes

* trim whitespace from account number + api key on link ([9db8247](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/9db8247a3a72aa1f26e58bf44784a230bc7c2536))

## 1.0.0 (2026-04-26)


### Features

* /web command and bot-wide mini-app menu button ([46af6d6](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/46af6d627486b4bd1ba9afba46c51952ab2ccf29))
* accept flexible time input everywhere (7am, 7pm, 7:30 pm) ([e433b7b](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/e433b7bceaf3973b3e483b18e2362745d2fd8ef4))
* add instant charge planner (/plan) with optional deadline ([576ce5e](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/576ce5e09a5e479223bd1599da2c19b8a10630ee))
* add negative-price alerts ([1922995](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/192299513fcd259a57a93cdb9340f2348d9611d5))
* add telegram bot for octopus agile rates ([2f2bf62](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/2f2bf62e760733244c592a53ea75fa1b436ae780))
* add web ui with telegram login ([98308b7](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/98308b79f92e51df8756b861825797d28bbb49e1))
* adopt @immich/ui theme and add dark mode ([a2cb0a9](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/a2cb0a92ca3fff3ec318b36ef0514ea798793c06))
* bot ux improvements and web api scaffold ([e0c48bc](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/e0c48bc706e65cdd0193709e208411111d4611c3))
* follow telegram's color scheme inside the mini app ([aa2a6d7](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/aa2a6d7ab80f4b0f6556cccf915c57a9f80bf092))
* link octopus account via api key ([ed3c360](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/ed3c3603b6c7f9aac3e46b074fbfba8be1616828))
* migrate web ui to @immich/ui and add consumption page ([026df7b](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/026df7bd7dd2ccac410d088685b3ec8e6e7e0eb2))
* mini-app autologin via /api/auth/telegram/initdata ([f6bec7a](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/f6bec7ad450e7f352341b22629d3d03a74f7cb3c))


### Bug Fixes

* chart axes invisible in dark mode and inner scrollbar ([9518eb5](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/9518eb54097bc15e07aedee07246ed416cc741c2))
* dogfood-driven ui polish ([df578c8](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/df578c894653ebe37e388156cc462e964d9c3941))
* gofmt internal/httpapi/routes.go and internal/service/service_test.go ([c93c674](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/c93c674317d4ba733eb33c9aecb8b64af81c45a5))
* narrow @immich/ui usage to verified-working components ([cc9dec8](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/cc9dec83368c3205fec52afc359f9665487f8329))
* tell tailwind v4 to scan @immich/ui so its utility classes land in css ([e3091f1](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/e3091f123c25a6a03ab1c1327b66fad03837daee))
* warm rate cache when switching regions, skip if already fresh ([dc04a56](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/dc04a56214167d9e5d88b501dabf67d5d754218d))
* wrap layout in TooltipProvider to satisfy @immich/ui ([71537ab](https://github.com/zp-bots-telegram/octopus-agile-bot/commit/71537abb04da943fec165ed17f640563a8277997))
