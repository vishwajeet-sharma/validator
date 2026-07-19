// Package models defines the core domain types shared across the database,
// API, worker, and scouts layers of the Validator platform.
package models

import "time"

// ScoutType is the polarity of a scout and the signals it harvests. It is used
// both as the scouts.scout_type and the market_signals.polarity.
type ScoutType string

const (
	ScoutTypePro ScoutType = "PRO"
	ScoutTypeCon ScoutType = "CON"
)

// IdeaStatus is the lifecycle state of an idea.
type IdeaStatus string

const (
	IdeaStatusInitialSweep IdeaStatus = "INITIAL_SWEEP"
	IdeaStatusActive       IdeaStatus = "ACTIVE"
)

// ScoutStatus is the tracking state of an individual scout.
type ScoutStatus string

const (
	ScoutStatusActive          ScoutStatus = "ACTIVE"
	ScoutStatusPendingMutation ScoutStatus = "PENDING_MUTATION"
	ScoutStatusStopped         ScoutStatus = "STOPPED"
)

// ProposalStatus is the human-in-the-loop state of a prompt proposal.
type ProposalStatus string

const (
	ProposalPending  ProposalStatus = "PENDING"
	ProposalApproved ProposalStatus = "APPROVED"
	ProposalRejected ProposalStatus = "REJECTED"
)

// Idea is a market hypothesis being continuously validated by two scouts.
type Idea struct {
	ID            string
	Title         string
	Description   string
	FrequencyDays int
	Status        IdeaStatus
	TotalPros     int
	TotalCons     int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Scout is a single Yutori scouting task (PRO or CON) attached to an idea.
type Scout struct {
	ID            string
	IdeaID        string
	YutoriScoutID string
	ScoutType     ScoutType
	CurrentPrompt string
	Status        ScoutStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PromptProposal is an AI-proposed search-radius expansion awaiting human review.
type PromptProposal struct {
	ID             string
	ScoutID        string
	ProposedPrompt string
	Status         ProposalStatus
	CreatedAt      time.Time
	ResolvedAt     *time.Time
}

// MarketSignal is a single pro/con finding harvested by a scout.
type MarketSignal struct {
	ID          string
	IdeaID      string
	ScoutID     string
	Polarity    ScoutType
	Platform    string
	Quote       string
	Reason      string
	SourceURL   string
	SourceTitle string
	CreatedAt   time.Time
}
