CREATE TABLE IF NOT EXISTS chats (
    chat_id    INTEGER PRIMARY KEY,
    region     TEXT    NOT NULL,
    timezone   TEXT    NOT NULL DEFAULT 'Europe/London',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS subscriptions (
    chat_id          INTEGER PRIMARY KEY REFERENCES chats(chat_id) ON DELETE CASCADE,
    duration_minutes INTEGER NOT NULL,
    notify_at_local  TEXT    NOT NULL,
    enabled          INTEGER NOT NULL DEFAULT 1,
    created_at       TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS charge_plans (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id            INTEGER NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    duration_minutes   INTEGER NOT NULL,
    window_start_local TEXT    NOT NULL,
    window_end_local   TEXT    NOT NULL,
    enabled            INTEGER NOT NULL DEFAULT 1,
    created_at         TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_charge_plans_chat ON charge_plans(chat_id);

CREATE TABLE IF NOT EXISTS rates (
    valid_from          TEXT NOT NULL,
    region              TEXT NOT NULL,
    tariff_code         TEXT NOT NULL,
    valid_to            TEXT NOT NULL,
    unit_rate_exc_vat   REAL NOT NULL,
    unit_rate_inc_vat   REAL NOT NULL,
    PRIMARY KEY (valid_from, region)
);
CREATE INDEX IF NOT EXISTS idx_rates_region_from ON rates(region, valid_from);

CREATE TABLE IF NOT EXISTS charge_dispatch_log (
    chat_id     INTEGER NOT NULL,
    plan_id     INTEGER NOT NULL,
    target_date TEXT    NOT NULL,
    dispatched_at TEXT  NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    PRIMARY KEY (chat_id, plan_id, target_date)
);
