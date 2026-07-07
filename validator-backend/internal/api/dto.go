package api

import (
	"time"

	"validator-backend/internal/models"
)

// The DTO types in this file are the exact JSON contract the React UI consumes
// (see validator-ui/src/types/index.ts). They use camelCase and embed the nested
// cycles/findings shape the UI expects, so the database's normalized model can
// stay clean while the API speaks the frontend's dialect.

// FindingDTO is a single pro/con quote with its source metadata.
type FindingDTO struct {
	ID          string `json:"id"`
	Platform    string `json:"platform"`
	Quote       string `json:"quote"`
	Reason      string `json:"reason"`
	SourceURL   string `json:"sourceUrl"`
	SourceTitle string `json:"sourceTitle"`
}

// ScoutCycleDTO is one scout execution: its metadata plus the pro/con findings.
type ScoutCycleDTO struct {
	ID    string       `json:"id"`
	Day   int          `json:"day"`
	Label string       `json:"label"`
	Date  string       `json:"date"`
	Pros  []FindingDTO `json:"pros"`
	Cons  []FindingDTO `json:"cons"`
}

// CustomChannelDTO is a user-defined source URL/label.
type CustomChannelDTO struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Label string `json:"label"`
}

// IdeaDTO is the full idea payload the UI renders, including embedded cycles.
type IdeaDTO struct {
	ID                    string             `json:"id"`
	Title                 string             `json:"title"`
	Description           string             `json:"description"`
	Keywords              []string           `json:"keywords"`
	ScoutingFrequencyDays int                `json:"scoutingFrequencyDays"`
	Channels              []string           `json:"channels"`
	CustomChannels        []CustomChannelDTO `json:"customChannels"`
	CreatedAt             string             `json:"createdAt"`
	LastUpdated           string             `json:"lastUpdated"`
	TotalPros             int                `json:"totalPros"`
	TotalCons             int                `json:"totalCons"`
	NewSignalsToday       int                `json:"newSignalsToday"`
	Status                string             `json:"status"`
	StatusMessage         string             `json:"statusMessage"`
	Cycles                []ScoutCycleDTO    `json:"cycles"`
}

// BuildIdeaDTO assembles the UI-facing idea shape from the normalized DB models,
// grouping signals under their scout run and bucketing them into pros/cons. It
// also computes newSignalsToday (signals created since the start of today UTC).
func BuildIdeaDTO(idea *models.Idea, runs []models.ScoutRun, signals []models.MarketSignal) IdeaDTO {
	// Index signals by their scout run for O(n) assembly.
	byRun := make(map[string][]models.MarketSignal, len(runs))
	startOfToday := todayStartUTC()
	newToday := 0
	for _, sig := range signals {
		byRun[sig.ScoutRunID] = append(byRun[sig.ScoutRunID], sig)
		if !sig.CreatedAt.Before(startOfToday) {
			newToday++
		}
	}

	cycles := make([]ScoutCycleDTO, 0, len(runs))
	for _, run := range runs {
		cycle := ScoutCycleDTO{
			ID:    run.ID,
			Day:   run.DayNumber,
			Label: run.Label,
			Date:  run.RunAt.UTC().Format(time.RFC3339),
			Pros:  []FindingDTO{},
			Cons:  []FindingDTO{},
		}
		for _, sig := range byRun[run.ID] {
			finding := FindingDTO{
				ID:          sig.ID,
				Platform:    string(sig.Platform),
				Quote:       sig.Quote,
				Reason:      sig.Reason,
				SourceURL:   sig.SourceURL,
				SourceTitle: sig.SourceTitle,
			}
			if sig.Polarity == models.PolarityCon {
				cycle.Cons = append(cycle.Cons, finding)
			} else {
				cycle.Pros = append(cycle.Pros, finding)
			}
		}
		cycles = append(cycles, cycle)
	}

	custom := make([]CustomChannelDTO, 0, len(idea.CustomChannels))
	for _, c := range idea.CustomChannels {
		custom = append(custom, CustomChannelDTO{ID: c.ID, URL: c.URL, Label: c.Label})
	}

	keywords := idea.Keywords
	channels := idea.Channels
	if keywords == nil {
		keywords = []string{}
	}
	if channels == nil {
		channels = []string{}
	}

	return IdeaDTO{
		ID:                    idea.ID,
		Title:                 idea.Title,
		Description:           idea.Description,
		Keywords:              keywords,
		ScoutingFrequencyDays: idea.FrequencyDays,
		Channels:              channels,
		CustomChannels:        custom,
		CreatedAt:             idea.CreatedAt.UTC().Format(time.RFC3339),
		LastUpdated:           idea.UpdatedAt.UTC().Format(time.RFC3339),
		TotalPros:             idea.TotalPros,
		TotalCons:             idea.TotalCons,
		NewSignalsToday:       newToday,
		Status:                idea.Status,
		StatusMessage:         idea.StatusMessage,
		Cycles:                cycles,
	}
}

// todayStartUTC returns the midnight boundary of today in UTC.
func todayStartUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
