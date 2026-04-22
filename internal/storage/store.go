// Package storage provides SQLite persistence for the bot. Migrations are embedded;
// connection uses the pure-Go modernc.org/sqlite driver so the binary has no CGO deps.
package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/zp-bots-telegram/octopus-agile-bot/internal/agile"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store is the SQLite-backed repository. It intentionally exposes every repository
// method on one struct — the `internal/service` package defines the focused interfaces
// it actually consumes, so switching out the backend later means one place to change.
type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')))`,
	); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		var v int
		if _, err := fmt.Sscanf(e.Name(), "%04d_", &v); err != nil {
			return fmt.Errorf("bad migration filename %q", e.Name())
		}

		var applied int
		if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE version=?`, v).Scan(&applied); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if applied == 1 {
			continue
		}

		sqlBytes, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s: %w", e.Name(), err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES(?)`, v); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// ---- chats ----------------------------------------------------------------

type Chat struct {
	ChatID   int64
	Region   string
	Timezone string
}

// UpsertChatRegion creates or updates the chat's region. Leaves the timezone alone if
// the chat already exists.
func (s *Store) UpsertChatRegion(ctx context.Context, chatID int64, region string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chats(chat_id, region) VALUES(?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET region=excluded.region
	`, chatID, region)
	return err
}

func (s *Store) SetChatTimezone(ctx context.Context, chatID int64, tz string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE chats SET timezone=? WHERE chat_id=?`, tz, chatID)
	return err
}

func (s *Store) GetChat(ctx context.Context, chatID int64) (Chat, bool, error) {
	var c Chat
	err := s.db.QueryRowContext(ctx,
		`SELECT chat_id, region, timezone FROM chats WHERE chat_id=?`, chatID,
	).Scan(&c.ChatID, &c.Region, &c.Timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return Chat{}, false, nil
	}
	if err != nil {
		return Chat{}, false, err
	}
	return c, true, nil
}

// DistinctRegions returns every region letter currently in use. Useful for the rate
// refresher so we only fetch what's needed.
func (s *Store) DistinctRegions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT region FROM chats ORDER BY region`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- subscriptions --------------------------------------------------------

type Subscription struct {
	ChatID        int64
	Duration      time.Duration
	NotifyAtLocal string // "HH:MM"
	Enabled       bool
}

func (s *Store) SetSubscription(ctx context.Context, sub Subscription) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO subscriptions(chat_id, duration_minutes, notify_at_local, enabled)
		VALUES(?, ?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
		  duration_minutes = excluded.duration_minutes,
		  notify_at_local  = excluded.notify_at_local,
		  enabled          = excluded.enabled
	`, sub.ChatID, int(sub.Duration/time.Minute), sub.NotifyAtLocal, boolInt(sub.Enabled))
	return err
}

func (s *Store) DeleteSubscription(ctx context.Context, chatID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE chat_id=?`, chatID)
	return err
}

func (s *Store) GetSubscription(ctx context.Context, chatID int64) (Subscription, bool, error) {
	var sub Subscription
	var mins int
	var en int
	err := s.db.QueryRowContext(ctx, `
		SELECT chat_id, duration_minutes, notify_at_local, enabled
		FROM subscriptions WHERE chat_id=?`, chatID,
	).Scan(&sub.ChatID, &mins, &sub.NotifyAtLocal, &en)
	if errors.Is(err, sql.ErrNoRows) {
		return Subscription{}, false, nil
	}
	if err != nil {
		return Subscription{}, false, err
	}
	sub.Duration = time.Duration(mins) * time.Minute
	sub.Enabled = en == 1
	return sub, true, nil
}

func (s *Store) ListEnabledSubscriptions(ctx context.Context) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT chat_id, duration_minutes, notify_at_local, enabled
		FROM subscriptions WHERE enabled=1 ORDER BY chat_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		var sub Subscription
		var mins, en int
		if err := rows.Scan(&sub.ChatID, &mins, &sub.NotifyAtLocal, &en); err != nil {
			return nil, err
		}
		sub.Duration = time.Duration(mins) * time.Minute
		sub.Enabled = en == 1
		out = append(out, sub)
	}
	return out, rows.Err()
}

// ---- charge plans ---------------------------------------------------------

type ChargePlan struct {
	ID               int64
	ChatID           int64
	Duration         time.Duration
	WindowStartLocal string // "HH:MM"
	WindowEndLocal   string // "HH:MM"
	Enabled          bool
}

func (s *Store) CreateChargePlan(ctx context.Context, p ChargePlan) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO charge_plans(chat_id, duration_minutes, window_start_local, window_end_local, enabled)
		VALUES(?, ?, ?, ?, 1)
	`, p.ChatID, int(p.Duration/time.Minute), p.WindowStartLocal, p.WindowEndLocal)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListChargePlans(ctx context.Context, chatID int64) ([]ChargePlan, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, chat_id, duration_minutes, window_start_local, window_end_local, enabled
		FROM charge_plans WHERE chat_id=? ORDER BY id`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChargePlans(rows)
}

func (s *Store) ListEnabledChargePlans(ctx context.Context) ([]ChargePlan, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, chat_id, duration_minutes, window_start_local, window_end_local, enabled
		FROM charge_plans WHERE enabled=1 ORDER BY chat_id, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChargePlans(rows)
}

func (s *Store) CancelChargePlan(ctx context.Context, chatID, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM charge_plans WHERE chat_id=? AND id=?`, chatID, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func scanChargePlans(rows *sql.Rows) ([]ChargePlan, error) {
	var out []ChargePlan
	for rows.Next() {
		var p ChargePlan
		var mins, en int
		if err := rows.Scan(&p.ID, &p.ChatID, &mins, &p.WindowStartLocal, &p.WindowEndLocal, &en); err != nil {
			return nil, err
		}
		p.Duration = time.Duration(mins) * time.Minute
		p.Enabled = en == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- rates ----------------------------------------------------------------

func (s *Store) UpsertRates(ctx context.Context, region, tariffCode string, rates []agile.HalfHour) error {
	if len(rates) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO rates(valid_from, region, tariff_code, valid_to, unit_rate_exc_vat, unit_rate_inc_vat)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(valid_from, region) DO UPDATE SET
		  tariff_code       = excluded.tariff_code,
		  valid_to          = excluded.valid_to,
		  unit_rate_exc_vat = excluded.unit_rate_exc_vat,
		  unit_rate_inc_vat = excluded.unit_rate_inc_vat
	`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r := range rates {
		if _, err := stmt.ExecContext(ctx,
			r.ValidFrom.UTC().Format(time.RFC3339), region, tariffCode,
			r.ValidTo.UTC().Format(time.RFC3339), r.UnitRateExcVAT, r.UnitRateIncVAT,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// Rates returns every half-hour for `region` whose [valid_from, valid_to] overlaps
// [from, to]. Sorted ascending.
func (s *Store) Rates(ctx context.Context, region string, from, to time.Time) ([]agile.HalfHour, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT valid_from, valid_to, unit_rate_exc_vat, unit_rate_inc_vat
		FROM rates
		WHERE region=? AND valid_to > ? AND valid_from < ?
		ORDER BY valid_from ASC
	`, region, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []agile.HalfHour
	for rows.Next() {
		var vf, vt string
		var exc, inc float64
		if err := rows.Scan(&vf, &vt, &exc, &inc); err != nil {
			return nil, err
		}
		f, err := time.Parse(time.RFC3339, vf)
		if err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, vt)
		if err != nil {
			return nil, err
		}
		out = append(out, agile.HalfHour{
			ValidFrom: f, ValidTo: t,
			UnitRateExcVAT: exc, UnitRateIncVAT: inc,
		})
	}
	return out, rows.Err()
}

// ---- price alerts ---------------------------------------------------------

type PriceAlert struct {
	ChatID          int64
	ThresholdIncVAT float64 // p/kWh inc VAT; fire when a rate is strictly less than this
	Enabled         bool
}

func (s *Store) SetPriceAlert(ctx context.Context, a PriceAlert) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO price_alerts(chat_id, threshold_inc_vat, enabled)
		VALUES(?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
		  threshold_inc_vat = excluded.threshold_inc_vat,
		  enabled           = excluded.enabled
	`, a.ChatID, a.ThresholdIncVAT, boolInt(a.Enabled))
	return err
}

func (s *Store) DeletePriceAlert(ctx context.Context, chatID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM price_alerts WHERE chat_id=?`, chatID)
	return err
}

func (s *Store) GetPriceAlert(ctx context.Context, chatID int64) (PriceAlert, bool, error) {
	var a PriceAlert
	var en int
	err := s.db.QueryRowContext(ctx, `
		SELECT chat_id, threshold_inc_vat, enabled FROM price_alerts WHERE chat_id=?`, chatID,
	).Scan(&a.ChatID, &a.ThresholdIncVAT, &en)
	if errors.Is(err, sql.ErrNoRows) {
		return PriceAlert{}, false, nil
	}
	if err != nil {
		return PriceAlert{}, false, err
	}
	a.Enabled = en == 1
	return a, true, nil
}

func (s *Store) ListEnabledPriceAlerts(ctx context.Context) ([]PriceAlert, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT chat_id, threshold_inc_vat, enabled FROM price_alerts WHERE enabled=1 ORDER BY chat_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PriceAlert
	for rows.Next() {
		var a PriceAlert
		var en int
		if err := rows.Scan(&a.ChatID, &a.ThresholdIncVAT, &en); err != nil {
			return nil, err
		}
		a.Enabled = en == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

// MarkPriceAlertDispatched returns true on first insert for this (chat_id, run_start),
// false if the run had already been notified.
func (s *Store) MarkPriceAlertDispatched(ctx context.Context, chatID int64, runStart time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO price_alert_log(chat_id, run_start)
		VALUES(?, ?)
	`, chatID, runStart.UTC().Format(time.RFC3339))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ---- dispatch log ---------------------------------------------------------

// MarkCharnPlanDispatched records that a plan has been dispatched for a target date.
// Returns true if this call inserted the row (i.e. was the first dispatch), false if a
// row already existed.
func (s *Store) MarkChargePlanDispatched(ctx context.Context, chatID, planID int64, targetDate time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO charge_dispatch_log(chat_id, plan_id, target_date)
		VALUES(?, ?, ?)
	`, chatID, planID, targetDate.UTC().Format("2006-01-02"))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
