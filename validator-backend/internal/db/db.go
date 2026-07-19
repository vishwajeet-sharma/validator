// Package db wraps a PostgreSQL connection pool and exposes transaction-backed
// helpers for the four core tables: ideas, scouts, prompt_proposals, and
// market_signals.
package db

import (
	"context"
	"database/sql"
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

// schema defines the idempotent DDL applied on startup. A guarded DO block
// drops the legacy keyword/channel-based schema exactly once so the new
// isolated PRO/CON schema can be created cleanly.
const schema = `
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_name = 'ideas' AND column_name = 'keywords') THEN
    DROP TABLE IF EXISTS market_signals CASCADE;
    DROP TABLE IF EXISTS scout_runs CASCADE;
    DROP TABLE IF EXISTS ideas CASCADE;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS ideas (
    id             UUID PRIMARY KEY NOT NULL,
    title          TEXT        NOT NULL,
    description    TEXT        NOT NULL,
    frequency_days INTEGER     NOT NULL DEFAULT 7,
    status         TEXT        NOT NULL DEFAULT 'INITIAL_SWEEP',
    total_pros     INTEGER     NOT NULL DEFAULT 0,
    total_cons     INTEGER     NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS scouts (
    id              UUID         PRIMARY KEY NOT NULL,
    idea_id         UUID         NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    yutori_scout_id VARCHAR(255) NOT NULL UNIQUE,
    scout_type      VARCHAR(4)   NOT NULL CHECK (scout_type IN ('PRO', 'CON')),
    current_prompt  TEXT         NOT NULL,
    status          VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE'
                    CHECK (status IN ('ACTIVE', 'PENDING_MUTATION', 'STOPPED')),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_scouts_idea  ON scouts (idea_id);
CREATE INDEX IF NOT EXISTS idx_scouts_yutori ON scouts (yutori_scout_id);

CREATE TABLE IF NOT EXISTS prompt_proposals (
    id              UUID        PRIMARY KEY NOT NULL,
    scout_id        UUID        NOT NULL REFERENCES scouts(id) ON DELETE CASCADE,
    proposed_prompt TEXT        NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'PENDING'
                    CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_proposals_scout    ON prompt_proposals (scout_id);
CREATE INDEX IF NOT EXISTS idx_proposals_pending
    ON prompt_proposals (scout_id) WHERE status = 'PENDING';

CREATE TABLE IF NOT EXISTS market_signals (
    id           UUID        PRIMARY KEY NOT NULL,
    idea_id      UUID        NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    scout_id     UUID        NOT NULL REFERENCES scouts(id) ON DELETE CASCADE,
    polarity     VARCHAR(4)  NOT NULL CHECK (polarity IN ('PRO', 'CON')),
    platform     TEXT        NOT NULL DEFAULT '',
    quote        TEXT        NOT NULL,
    reason       TEXT        NOT NULL DEFAULT '',
    source_url   TEXT        NOT NULL,
    source_title TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_signals_idea  ON market_signals (idea_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_signals_scout ON market_signals (scout_id);
`

// scoutStatusConstraintMigration widens the scouts.status CHECK constraint to
// include 'STOPPED' (added with the stop-scout feature). CREATE TABLE IF NOT
// EXISTS does not alter an existing table's constraints, so existing databases
// need this explicit drop+re-add. It is idempotent and safe to run on every
// startup.
const scoutStatusConstraintMigration = `
ALTER TABLE scouts DROP CONSTRAINT IF EXISTS scouts_status_check;
ALTER TABLE scouts ADD CONSTRAINT scouts_status_check
    CHECK (status IN ('ACTIVE', 'PENDING_MUTATION', 'STOPPED'));
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

	if _, err := db.ExecContext(ctx, scoutStatusConstraintMigration); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply scout status migration: %w", err)
	}

	slog.Info("database connected and schema applied")
	return &Store{db: db}, nil
}

// Close releases the underlying connection pool.
func (s *Store) Close() error { return s.db.Close() }

// --- Ideas ----------------------------------------------------------------

// CreateIdea persists a brand-new idea inside a transaction, returning the
// stored row (including generated timestamps).
func (s *Store) CreateIdea(ctx context.Context, idea *models.Idea) error {
	const q = `
INSERT INTO ideas (id, title, description, frequency_days, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
RETURNING created_at, updated_at`
	row := s.db.QueryRowContext(ctx, q,
		idea.ID, idea.Title, idea.Description, idea.FrequencyDays, idea.Status)
	if err := row.Scan(&idea.CreatedAt, &idea.UpdatedAt); err != nil {
		return fmt.Errorf("insert idea: %w", err)
	}
	return nil
}

// GetIdea loads a single idea by id.
func (s *Store) GetIdea(ctx context.Context, id string) (*models.Idea, error) {
	const q = `
SELECT id, title, description, frequency_days, status, total_pros, total_cons, created_at, updated_at
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
SELECT id, title, description, frequency_days, status, total_pros, total_cons, created_at, updated_at
FROM ideas ORDER BY created_at DESC`
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

// ActivateIdea flips an idea from INITIAL_SWEEP to ACTIVE once its Day 0 scouts
// have been deployed.
func (s *Store) ActivateIdea(ctx context.Context, ideaID string) error {
	const q = `UPDATE ideas SET status = $2, updated_at = NOW() WHERE id = $1`
	res, err := s.db.ExecContext(ctx, q, ideaID, models.IdeaStatusActive)
	if err != nil {
		return fmt.Errorf("activate idea: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: idea %s", ErrNotFound, ideaID)
	}
	return nil
}

// --- Scouts ---------------------------------------------------------------

// CreateScout persists a scout row, idempotent on the Yutori scout id. If a row
// with the same yutori_scout_id already exists (e.g. from a partial side-effect
// replay where the Yutori task was created but the journal entry didn't
// complete), it is updated in place instead of failing on the UNIQUE
// constraint. The actual DB id and timestamps are returned so callers always
// observe the real row identity.
func (s *Store) CreateScout(ctx context.Context, scout *models.Scout) error {
	const q = `
INSERT INTO scouts (id, idea_id, yutori_scout_id, scout_type, current_prompt, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
ON CONFLICT (yutori_scout_id) DO UPDATE
SET current_prompt = EXCLUDED.current_prompt,
    status         = EXCLUDED.status,
    updated_at     = NOW()
RETURNING id, created_at, updated_at`
	row := s.db.QueryRowContext(ctx, q,
		scout.ID, scout.IdeaID, scout.YutoriScoutID, scout.ScoutType,
		scout.CurrentPrompt, scout.Status)
	if err := row.Scan(&scout.ID, &scout.CreatedAt, &scout.UpdatedAt); err != nil {
		return fmt.Errorf("upsert scout: %w", err)
	}
	return nil
}

// GetScoutsByIdea returns all scouts for an idea.
func (s *Store) GetScoutsByIdea(ctx context.Context, ideaID string) ([]models.Scout, error) {
	const q = `
SELECT id, idea_id, yutori_scout_id, scout_type, current_prompt, status, created_at, updated_at
FROM scouts WHERE idea_id = $1 ORDER BY scout_type`
	rows, err := s.db.QueryContext(ctx, q, ideaID)
	if err != nil {
		return nil, fmt.Errorf("query scouts: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanScouts(rows)
}

// GetScoutsByIdeaIDs batch-loads scouts for many ideas, grouped by idea id.
func (s *Store) GetScoutsByIdeaIDs(ctx context.Context, ideaIDs []string) (map[string][]models.Scout, error) {
	out := make(map[string][]models.Scout)
	if len(ideaIDs) == 0 {
		return out, nil
	}
	const q = `
SELECT id, idea_id, yutori_scout_id, scout_type, current_prompt, status, created_at, updated_at
FROM scouts WHERE idea_id = ANY($1) ORDER BY scout_type`
	rows, err := s.db.QueryContext(ctx, q, pq.Array(ideaIDs))
	if err != nil {
		return nil, fmt.Errorf("query scouts by ideas: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	scouts, err := scanScouts(rows)
	if err != nil {
		return nil, err
	}
	for _, sc := range scouts {
		out[sc.IdeaID] = append(out[sc.IdeaID], sc)
	}
	return out, nil
}

// GetScoutByYutoriID loads a scout by its Yutori scout id (used by the webhook
// path to resolve incoming updates to an idea + polarity).
func (s *Store) GetScoutByYutoriID(ctx context.Context, yutoriScoutID string) (*models.Scout, error) {
	const q = `
SELECT id, idea_id, yutori_scout_id, scout_type, current_prompt, status, created_at, updated_at
FROM scouts WHERE yutori_scout_id = $1`
	row := s.db.QueryRowContext(ctx, q, yutoriScoutID)
	var sc models.Scout
	if err := row.Scan(&sc.ID, &sc.IdeaID, &sc.YutoriScoutID, &sc.ScoutType,
		&sc.CurrentPrompt, &sc.Status, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: scout %s", ErrNotFound, yutoriScoutID)
		}
		return nil, fmt.Errorf("get scout by yutori id: %w", err)
	}
	return &sc, nil
}

// GetScout loads a scout by its primary id.
func (s *Store) GetScout(ctx context.Context, scoutID string) (*models.Scout, error) {
	const q = `
SELECT id, idea_id, yutori_scout_id, scout_type, current_prompt, status, created_at, updated_at
FROM scouts WHERE id = $1`
	row := s.db.QueryRowContext(ctx, q, scoutID)
	var sc models.Scout
	if err := row.Scan(&sc.ID, &sc.IdeaID, &sc.YutoriScoutID, &sc.ScoutType,
		&sc.CurrentPrompt, &sc.Status, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: scout %s", ErrNotFound, scoutID)
		}
		return nil, fmt.Errorf("get scout: %w", err)
	}
	return &sc, nil
}

// UpdateScoutPrompt replaces a scout's current_prompt and restores ACTIVE
// status. Used when a proposal is approved.
func (s *Store) UpdateScoutPrompt(ctx context.Context, scoutID, newPrompt string) error {
	const q = `
UPDATE scouts SET current_prompt = $2, status = $3, updated_at = NOW() WHERE id = $1`
	res, err := s.db.ExecContext(ctx, q, scoutID, newPrompt, models.ScoutStatusActive)
	if err != nil {
		return fmt.Errorf("update scout prompt: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: scout %s", ErrNotFound, scoutID)
	}
	return nil
}

// SetScoutStatus flips a scout's tracking status.
func (s *Store) SetScoutStatus(ctx context.Context, scoutID string, status models.ScoutStatus) error {
	const q = `UPDATE scouts SET status = $2, updated_at = NOW() WHERE id = $1`
	res, err := s.db.ExecContext(ctx, q, scoutID, status)
	if err != nil {
		return fmt.Errorf("set scout status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: scout %s", ErrNotFound, scoutID)
	}
	return nil
}

// --- Proposals ------------------------------------------------------------

// CreateProposal inserts a new pending proposal and flips the owning scout to
// PENDING_MUTATION, atomically.
func (s *Store) CreateProposal(ctx context.Context, scoutID, proposedPrompt string) (*models.PromptProposal, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck

	proposal := &models.PromptProposal{
		ID:             uuid.NewString(),
		ScoutID:        scoutID,
		ProposedPrompt: proposedPrompt,
		Status:         models.ProposalPending,
	}

	const ins = `
INSERT INTO prompt_proposals (id, scout_id, proposed_prompt, status, created_at)
VALUES ($1, $2, $3, $4, NOW())
RETURNING created_at`
	if err := tx.QueryRowContext(ctx, ins,
		proposal.ID, proposal.ScoutID, proposal.ProposedPrompt, proposal.Status).Scan(&proposal.CreatedAt); err != nil {
		return nil, fmt.Errorf("insert proposal: %w", err)
	}

	const upd = `UPDATE scouts SET status = $2, updated_at = NOW() WHERE id = $1`
	if _, err := tx.ExecContext(ctx, upd, scoutID, models.ScoutStatusPendingMutation); err != nil {
		return nil, fmt.Errorf("mark scout pending: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit proposal: %w", err)
	}
	return proposal, nil
}

// GetProposal loads a proposal by id.
func (s *Store) GetProposal(ctx context.Context, proposalID string) (*models.PromptProposal, error) {
	const q = `
SELECT id, scout_id, proposed_prompt, status, created_at, COALESCE(resolved_at, '1970-01-01T00:00:00Z'), resolved_at
FROM prompt_proposals WHERE id = $1`
	row := s.db.QueryRowContext(ctx, q, proposalID)
	var p models.PromptProposal
	var resolvedFallback time.Time
	if err := row.Scan(&p.ID, &p.ScoutID, &p.ProposedPrompt, &p.Status, &p.CreatedAt, &resolvedFallback, &p.ResolvedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: proposal %s", ErrNotFound, proposalID)
		}
		return nil, fmt.Errorf("get proposal: %w", err)
	}
	return &p, nil
}

// ResolveProposal marks a proposal APPROVED or REJECTED and stamps resolved_at.
// It also restores the owning scout to ACTIVE (the approved prompt, if any, is
// applied separately by UpdateScoutPrompt).
func (s *Store) ResolveProposal(ctx context.Context, proposalID string, status models.ProposalStatus) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck

	const upd = `
UPDATE prompt_proposals SET status = $2, resolved_at = NOW() WHERE id = $1
RETURNING scout_id`
	var scoutID string
	if err := tx.QueryRowContext(ctx, upd, proposalID, status).Scan(&scoutID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: proposal %s", ErrNotFound, proposalID)
		}
		return fmt.Errorf("resolve proposal: %w", err)
	}

	// Restore the scout to ACTIVE regardless of approve/reject. On approve the
	// prompt is patched separately; on reject the original prompt is retained.
	if _, err := tx.ExecContext(ctx,
		`UPDATE scouts SET status = $2, updated_at = NOW() WHERE id = $1`,
		scoutID, models.ScoutStatusActive); err != nil {
		return fmt.Errorf("restore scout active: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit resolve: %w", err)
	}
	return nil
}

// GetPendingProposalByScout returns the most recent pending proposal for a
// scout, or ErrNotFound if none is pending.
func (s *Store) GetPendingProposalByScout(ctx context.Context, scoutID string) (*models.PromptProposal, error) {
	const q = `
SELECT id, scout_id, proposed_prompt, status, created_at, COALESCE(resolved_at, '1970-01-01T00:00:00Z'), resolved_at
FROM prompt_proposals
WHERE scout_id = $1 AND status = 'PENDING'
ORDER BY created_at DESC LIMIT 1`
	row := s.db.QueryRowContext(ctx, q, scoutID)
	var p models.PromptProposal
	var resolvedFallback time.Time
	if err := row.Scan(&p.ID, &p.ScoutID, &p.ProposedPrompt, &p.Status, &p.CreatedAt, &resolvedFallback, &p.ResolvedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: pending proposal for scout %s", ErrNotFound, scoutID)
		}
		return nil, fmt.Errorf("get pending proposal: %w", err)
	}
	return &p, nil
}

// GetPendingProposalsByScoutIDs batch-loads the active pending proposal (if any)
// for each scout id.
func (s *Store) GetPendingProposalsByScoutIDs(ctx context.Context, scoutIDs []string) (map[string]*models.PromptProposal, error) {
	out := make(map[string]*models.PromptProposal)
	if len(scoutIDs) == 0 {
		return out, nil
	}
	const q = `
SELECT DISTINCT ON (scout_id)
       id, scout_id, proposed_prompt, status, created_at, COALESCE(resolved_at, '1970-01-01T00:00:00Z'), resolved_at
FROM prompt_proposals
WHERE scout_id = ANY($1) AND status = 'PENDING'
ORDER BY scout_id, created_at DESC`
	rows, err := s.db.QueryContext(ctx, q, pq.Array(scoutIDs))
	if err != nil {
		return nil, fmt.Errorf("query pending proposals: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	for rows.Next() {
		var p models.PromptProposal
		var resolvedFallback time.Time
		if err := rows.Scan(&p.ID, &p.ScoutID, &p.ProposedPrompt, &p.Status, &p.CreatedAt, &resolvedFallback, &p.ResolvedAt); err != nil {
			return nil, fmt.Errorf("scan proposal: %w", err)
		}
		out[p.ScoutID] = &p
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate proposals: %w", err)
	}
	return out, nil
}

// --- Signals --------------------------------------------------------------

// SignalInput is the wire shape used when recording harvested findings.
type SignalInput struct {
	Platform    string
	Quote       string
	Reason      string
	SourceURL   string
	SourceTitle string
}

// RecordSignals persists a batch of signals for a scout and bumps the idea's
// rollup counters, all within a single transaction.
func (s *Store) RecordSignals(
	ctx context.Context,
	ideaID, scoutID string,
	polarity models.ScoutType,
	signals []SignalInput,
) error {
	if len(signals) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck

	const ins = `
INSERT INTO market_signals
    (id, idea_id, scout_id, polarity, platform, quote, reason, source_url, source_title, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())`
	for _, it := range signals {
		if _, err := tx.ExecContext(ctx, ins,
			uuid.NewString(), ideaID, scoutID, polarity, it.Platform,
			it.Quote, it.Reason, it.SourceURL, it.SourceTitle); err != nil {
			return fmt.Errorf("insert signal: %w", err)
		}
	}

	col := "total_pros"
	if polarity == models.ScoutTypeCon {
		col = "total_cons"
	}
	upd := fmt.Sprintf(`UPDATE ideas SET %s = %s + $2, updated_at = NOW() WHERE id = $1`, col, col)
	if _, err := tx.ExecContext(ctx, upd, ideaID, len(signals)); err != nil {
		return fmt.Errorf("update idea rollup: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit signals: %w", err)
	}
	return nil
}

// GetSignalsByIdea returns all signals for an idea, newest first.
func (s *Store) GetSignalsByIdea(ctx context.Context, ideaID string) ([]models.MarketSignal, error) {
	const q = `
SELECT id, idea_id, scout_id, polarity, platform, quote, reason, source_url, source_title, created_at
FROM market_signals WHERE idea_id = $1 ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, q, ideaID)
	if err != nil {
		return nil, fmt.Errorf("query signals: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []models.MarketSignal
	for rows.Next() {
		var sig models.MarketSignal
		if err := rows.Scan(&sig.ID, &sig.IdeaID, &sig.ScoutID, &sig.Polarity,
			&sig.Platform, &sig.Quote, &sig.Reason, &sig.SourceURL, &sig.SourceTitle, &sig.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan signal: %w", err)
		}
		out = append(out, sig)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate signals: %w", err)
	}
	return out, nil
}

// --- scanners -------------------------------------------------------------

type scanner interface {
	Scan(dest ...any) error
}

func scanIdea(s scanner) (*models.Idea, error) {
	var idea models.Idea
	if err := s.Scan(&idea.ID, &idea.Title, &idea.Description, &idea.FrequencyDays,
		&idea.Status, &idea.TotalPros, &idea.TotalCons, &idea.CreatedAt, &idea.UpdatedAt); err != nil {
		return nil, err
	}
	return &idea, nil
}

func scanScouts(rows *sql.Rows) ([]models.Scout, error) {
	var out []models.Scout
	for rows.Next() {
		var sc models.Scout
		if err := rows.Scan(&sc.ID, &sc.IdeaID, &sc.YutoriScoutID, &sc.ScoutType,
			&sc.CurrentPrompt, &sc.Status, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan scout: %w", err)
		}
		out = append(out, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scouts: %w", err)
	}
	return out, nil
}
