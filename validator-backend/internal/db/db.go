// Package db wraps a PostgreSQL connection pool and exposes transaction-backed
// helpers for the three core tables: ideas, scout_runs and market_signals.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"validator-backend/internal/models"
)

// ErrNotFound is returned when a single-row lookup yields no rows.
var ErrNotFound = errors.New("not found")

// schema defines the idempotent DDL applied on startup.
const schema = `
CREATE TABLE IF NOT EXISTS ideas (
    id             TEXT PRIMARY KEY,
    title          TEXT        NOT NULL,
    description    TEXT        NOT NULL,
    frequency_days INTEGER     NOT NULL DEFAULT 7,
    keywords       TEXT[]      NOT NULL DEFAULT '{}',
    channels       TEXT[]      NOT NULL DEFAULT '{}',
    custom_channels JSONB      NOT NULL DEFAULT '[]',
    status         TEXT        NOT NULL DEFAULT 'pending',
    status_message TEXT        NOT NULL DEFAULT '',
    total_pros     INTEGER     NOT NULL DEFAULT 0,
    total_cons     INTEGER     NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS scout_runs (
    id          TEXT        PRIMARY KEY,
    idea_id     TEXT        NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    day_number  INTEGER     NOT NULL,
    label       TEXT        NOT NULL,
    run_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_scout_runs_idea_run_at ON scout_runs (idea_id, run_at DESC);

CREATE TABLE IF NOT EXISTS market_signals (
    id           TEXT        PRIMARY KEY,
    scout_run_id TEXT        NOT NULL REFERENCES scout_runs(id) ON DELETE CASCADE,
    idea_id      TEXT        NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    polarity     TEXT        NOT NULL,
    platform     TEXT        NOT NULL,
    quote        TEXT        NOT NULL,
    reason       TEXT        NOT NULL,
    source_url   TEXT        NOT NULL,
    source_title TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_market_signals_run ON market_signals (scout_run_id);
CREATE INDEX IF NOT EXISTS idx_market_signals_idea ON market_signals (idea_id);
`

// Store is the database access facade.
type Store struct {
	db *sql.DB
}

// New opens a connection pool against databaseURL, pings it, configures pooling,
// and applies the schema.
func New(ctx context.Context, databaseURL string) (*Store, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	// Sensible production pooling defaults.
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	if _, err := db.ExecContext(ctx, schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	slog.Info("database connected and schema applied")
	return &Store{db: db}, nil
}

// Close releases the underlying connection pool.
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateIdea persists a brand-new tracking record inside a transaction.
func (s *Store) CreateIdea(ctx context.Context, idea *models.Idea) error {
	customJSON, err := json.Marshal(idea.CustomChannels)
	if err != nil {
		return fmt.Errorf("marshal custom channels: %w", err)
	}

	const q = `
INSERT INTO ideas
    (id, title, description, frequency_days, keywords, channels, custom_channels, status, status_message, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())`

	if _, err := s.db.ExecContext(ctx, q,
		idea.ID, idea.Title, idea.Description, idea.FrequencyDays,
		pq.Array(idea.Keywords), pq.Array(idea.Channels), customJSON,
		idea.Status, idea.StatusMessage,
	); err != nil {
		return fmt.Errorf("insert idea: %w", err)
	}
	return nil
}

// GetIdea loads a single idea by id.
func (s *Store) GetIdea(ctx context.Context, id string) (*models.Idea, error) {
	const q = `
SELECT id, title, description, frequency_days, keywords, channels, custom_channels,
       status, status_message, total_pros, total_cons, created_at, updated_at
FROM ideas WHERE id = $1`

	row := s.db.QueryRowContext(ctx, q, id)
	idea, err := scanIdea(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: idea %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("get idea: %w", err)
	}
	return idea, nil
}

// ListIdeas returns all tracked ideas, newest first.
func (s *Store) ListIdeas(ctx context.Context) ([]*models.Idea, error) {
	const q = `
SELECT id, title, description, frequency_days, keywords, channels, custom_channels,
       status, status_message, total_pros, total_cons, created_at, updated_at
FROM ideas
ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list ideas: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []*models.Idea
	for rows.Next() {
		idea, err := scanIdea(rows)
		if err != nil {
			return nil, fmt.Errorf("scan idea: %w", err)
		}
		out = append(out, idea)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ideas: %w", err)
	}
	return out, nil
}

// GetScoutRunsByIdeaIDs batch-loads scout runs for many ideas in a single query,
// grouped by idea id (each slice ordered oldest -> newest). Avoids N+1 queries
// when assembling a list of ideas with their cycles.
func (s *Store) GetScoutRunsByIdeaIDs(ctx context.Context, ideaIDs []string) (map[string][]models.ScoutRun, error) {
	out := make(map[string][]models.ScoutRun)
	if len(ideaIDs) == 0 {
		return out, nil
	}

	const q = `
SELECT id, idea_id, day_number, label, run_at
FROM scout_runs
WHERE idea_id = ANY($1)
ORDER BY run_at ASC`

	rows, err := s.db.QueryContext(ctx, q, pq.Array(ideaIDs))
	if err != nil {
		return nil, fmt.Errorf("query scout runs: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var run models.ScoutRun
		if err := rows.Scan(&run.ID, &run.IdeaID, &run.DayNumber, &run.Label, &run.RunAt); err != nil {
			return nil, fmt.Errorf("scan scout run: %w", err)
		}
		out[run.IdeaID] = append(out[run.IdeaID], run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scout runs: %w", err)
	}
	return out, nil
}

// GetSignalsByIdeaIDs batch-loads market signals for many ideas, grouped by
// idea id (each slice ordered oldest -> newest by created_at).
func (s *Store) GetSignalsByIdeaIDs(ctx context.Context, ideaIDs []string) (map[string][]models.MarketSignal, error) {
	out := make(map[string][]models.MarketSignal)
	if len(ideaIDs) == 0 {
		return out, nil
	}

	const q = `
SELECT id, scout_run_id, idea_id, polarity, platform, quote, reason, source_url, source_title, created_at
FROM market_signals
WHERE idea_id = ANY($1)
ORDER BY created_at ASC`

	rows, err := s.db.QueryContext(ctx, q, pq.Array(ideaIDs))
	if err != nil {
		return nil, fmt.Errorf("query market signals: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var sig models.MarketSignal
		if err := rows.Scan(&sig.ID, &sig.ScoutRunID, &sig.IdeaID, &sig.Polarity,
			&sig.Platform, &sig.Quote, &sig.Reason, &sig.SourceURL, &sig.SourceTitle, &sig.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan market signal: %w", err)
		}
		out[sig.IdeaID] = append(out[sig.IdeaID], sig)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate market signals: %w", err)
	}
	return out, nil
}

// GetLatestScoutRun returns the most recent scout run for an idea, or
// ErrNotFound if none exists yet.
func (s *Store) GetLatestScoutRun(ctx context.Context, ideaID string) (*models.ScoutRun, error) {
	const q = `
SELECT id, idea_id, day_number, label, run_at
FROM scout_runs
WHERE idea_id = $1
ORDER BY run_at DESC
LIMIT 1`

	row := s.db.QueryRowContext(ctx, q, ideaID)
	var run models.ScoutRun
	if err := row.Scan(&run.ID, &run.IdeaID, &run.DayNumber, &run.Label, &run.RunAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: scout run for idea %s", ErrNotFound, ideaID)
		}
		return nil, fmt.Errorf("get latest scout run: %w", err)
	}
	return &run, nil
}

// GetSignalsByRun returns all market signals attached to a scout run.
func (s *Store) GetSignalsByRun(ctx context.Context, scoutRunID string) ([]models.MarketSignal, error) {
	const q = `
SELECT id, scout_run_id, idea_id, polarity, platform, quote, reason, source_url, source_title, created_at
FROM market_signals
WHERE scout_run_id = $1
ORDER BY created_at`

	rows, err := s.db.QueryContext(ctx, q, scoutRunID)
	if err != nil {
		return nil, fmt.Errorf("query market signals: %w", err)
	}
	defer rows.Close()

	var out []models.MarketSignal
	for rows.Next() {
		var sig models.MarketSignal
		if err := rows.Scan(&sig.ID, &sig.ScoutRunID, &sig.IdeaID, &sig.Polarity,
			&sig.Platform, &sig.Quote, &sig.Reason, &sig.SourceURL, &sig.SourceTitle, &sig.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan market signal: %w", err)
		}
		out = append(out, sig)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate market signals: %w", err)
	}
	return out, nil
}

// SignalInput is the wire shape used by both the Yutori client and the DB layer
// when recording the results of a scout cycle.
type SignalInput struct {
	Platform    models.Platform
	Quote       string
	Reason      string
	SourceURL   string
	SourceTitle string
}

// RecordScoutRun writes the scout_run header and all pro/con signals, and bumps
// the idea rollup counters, all within a single transaction so a snapshot is
// never partially persisted.
func (s *Store) RecordScoutRun(
	ctx context.Context,
	ideaID string,
	dayNumber int,
	label string,
	pros []SignalInput,
	cons []SignalInput,
	status string,
	statusMessage string,
) (*models.ScoutRun, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck

	run := &models.ScoutRun{
		ID:        uuid.NewString(),
		IdeaID:    ideaID,
		DayNumber: dayNumber,
		Label:     label,
		RunAt:     time.Now().UTC(),
	}

	const insertRun = `
INSERT INTO scout_runs (id, idea_id, day_number, label, run_at)
VALUES ($1, $2, $3, $4, $5)`
	if _, err := tx.ExecContext(ctx, insertRun,
		run.ID, run.IdeaID, run.DayNumber, run.Label, run.RunAt); err != nil {
		return nil, fmt.Errorf("insert scout run: %w", err)
	}

	const insertSignal = `
INSERT INTO market_signals
    (id, scout_run_id, idea_id, polarity, platform, quote, reason, source_url, source_title, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())`

	insert := func(polarity models.Polarity, items []SignalInput) error {
		for _, it := range items {
			if _, err := tx.ExecContext(ctx, insertSignal,
				uuid.NewString(), run.ID, ideaID, polarity, it.Platform,
				it.Quote, it.Reason, it.SourceURL, it.SourceTitle); err != nil {
				return fmt.Errorf("insert %s signal: %w", polarity, err)
			}
		}
		return nil
	}
	if err := insert(models.PolarityPro, pros); err != nil {
		return nil, err
	}
	if err := insert(models.PolarityCon, cons); err != nil {
		return nil, err
	}

	const updateIdea = `
UPDATE ideas
SET total_pros    = total_pros + $2,
    total_cons    = total_cons + $3,
    status        = $4,
    status_message = $5,
    updated_at    = NOW()
WHERE id = $1`
	if _, err := tx.ExecContext(ctx, updateIdea,
		ideaID, len(pros), len(cons), status, statusMessage); err != nil {
		return nil, fmt.Errorf("update idea rollup: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit scout run: %w", err)
	}
	return run, nil
}

// scanner abstracts *sql.Row and *sql.Rows so scanIdea can serve both paths.
type scanner interface {
	Scan(dest ...any) error
}

func scanIdea(s scanner) (*models.Idea, error) {
	var idea models.Idea
	var customJSON []byte

	if err := s.Scan(
		&idea.ID, &idea.Title, &idea.Description, &idea.FrequencyDays,
		pq.Array(&idea.Keywords), pq.Array(&idea.Channels), &customJSON,
		&idea.Status, &idea.StatusMessage, &idea.TotalPros, &idea.TotalCons,
		&idea.CreatedAt, &idea.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if len(customJSON) > 0 && string(customJSON) != "null" {
		if err := json.Unmarshal(customJSON, &idea.CustomChannels); err != nil {
			return nil, fmt.Errorf("unmarshal custom channels: %w", err)
		}
	}
	return &idea, nil
}
