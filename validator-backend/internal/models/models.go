// Package models defines the core domain types shared across the database,
// API, workflow, and AI-client layers of the Validator platform.
package models

import "time"

// Platform is a target surface that scouts search for market signals on.
type Platform string

const (
	PlatformReddit  Platform = "reddit"
	PlatformYoutube Platform = "youtube"
	PlatformSocial  Platform = "social"
	PlatformNews    Platform = "news"
	PlatformCustom  Platform = "custom"
)

// Polarity expresses whether a signal supports or undermines the idea.
type Polarity string

const (
	PolarityPro Polarity = "pro"
	PolarityCon Polarity = "con"
)

// CustomChannel is a user-supplied source (e.g. a competitor landing page).
type CustomChannel struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Label string `json:"label"`
}

// Idea is a market hypothesis being continuously validated.
type Idea struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	FrequencyDays  int             `json:"frequency_days"`
	Keywords       []string        `json:"keywords"`
	Channels       []string        `json:"channels"`
	CustomChannels []CustomChannel `json:"custom_channels"`
	Status         string          `json:"status"`
	StatusMessage  string          `json:"status_message"`
	TotalPros      int             `json:"total_pros"`
	TotalCons      int             `json:"total_cons"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// ScoutRun is the snapshot metadata for a single execution cycle of the
// tracking workflow against an idea.
type ScoutRun struct {
	ID        string    `json:"id"`
	IdeaID    string    `json:"idea_id"`
	DayNumber int       `json:"day_number"`
	Label     string    `json:"label"`
	RunAt     time.Time `json:"run_at"`
}

// MarketSignal is a single pro/con finding harvested by a scout.
type MarketSignal struct {
	ID          string    `json:"id"`
	ScoutRunID  string    `json:"scout_run_id"`
	IdeaID      string    `json:"idea_id"`
	Polarity    Polarity  `json:"polarity"`
	Platform    Platform  `json:"platform"`
	Quote       string    `json:"quote"`
	Reason      string    `json:"reason"`
	SourceURL   string    `json:"source_url"`
	SourceTitle string    `json:"source_title"`
	CreatedAt   time.Time `json:"created_at"`
}
