CREATE TABLE IF NOT EXISTS price_alerts (
    chat_id           INTEGER PRIMARY KEY REFERENCES chats(chat_id) ON DELETE CASCADE,
    threshold_inc_vat REAL    NOT NULL DEFAULT 0,
    enabled           INTEGER NOT NULL DEFAULT 1,
    created_at        TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS price_alert_log (
    chat_id       INTEGER NOT NULL,
    run_start     TEXT    NOT NULL,
    dispatched_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    PRIMARY KEY (chat_id, run_start)
);
