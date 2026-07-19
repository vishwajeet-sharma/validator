package api

import (
	"unicode/utf8"

	"validator-backend/internal/models"
)

// The DTO types in this file are the exact JSON contract the React UI consumes
// (see validator-ui/src/types/index.ts). They use camelCase and mirror the
// split PRO/CON scout + proposal model.

// ScoutDTO is one PRO/CON scout with its (optional) pending proposal.
type ScoutDTO struct {
	ID              string       `json:"id"`
	ScoutType       string       `json:"scoutType"`
	Status          string       `json:"status"`
	CurrentPrompt   string       `json:"currentPrompt"`
	PendingProposal *ProposalDTO `json:"pendingProposal,omitempty"`
}

// ProposalDTO is an AI-proposed search-radius expansion awaiting review.
type ProposalDTO struct {
	ID             string `json:"id"`
	ProposedPrompt string `json:"proposedPrompt"`
	Status         string `json:"status"`
	CreatedAt      string `json:"createdAt"`
}

// FindingDTO is a single pro/con signal.
type FindingDTO struct {
	ID          string `json:"id"`
	Polarity    string `json:"polarity"`
	Platform    string `json:"platform"`
	Quote       string `json:"quote"`
	Reason      string `json:"reason"`
	SourceURL   string `json:"sourceUrl"`
	SourceTitle string `json:"sourceTitle"`
	CreatedAt   string `json:"createdAt"`
}

// IdeaSummaryDTO is the lightweight shape used by the dashboard list.
type IdeaSummaryDTO struct {
	ID                    string `json:"id"`
	Title                 string `json:"title"`
	Description           string `json:"description"`
	ScoutingFrequencyDays int    `json:"scoutingFrequencyDays"`
	Status                string `json:"status"`
	TotalPros             int    `json:"totalPros"`
	TotalCons             int    `json:"totalCons"`
	ProScoutStatus        string `json:"proScoutStatus"`
	ConScoutStatus        string `json:"conScoutStatus"`
	CreatedAt             string `json:"createdAt"`
	LastUpdated           string `json:"lastUpdated"`
}

// IdeaDetailDTO is the full shape used by the idea detail board, including the
// two scouts and the most recent pro/con findings.
type IdeaDetailDTO struct {
	IdeaSummaryDTO
	Scouts     []ScoutDTO   `json:"scouts"`
	RecentPros []FindingDTO `json:"recentPros"`
	RecentCons []FindingDTO `json:"recentCons"`
}

// BuildScoutDTO assembles a scout DTO with its pending proposal (if any).
func BuildScoutDTO(scout models.Scout, pending *models.PromptProposal) ScoutDTO {
	out := ScoutDTO{
		ID:            scout.ID,
		ScoutType:     string(scout.ScoutType),
		Status:        string(scout.Status),
		CurrentPrompt: scout.CurrentPrompt,
	}
	if pending != nil {
		out.PendingProposal = &ProposalDTO{
			ID:             pending.ID,
			ProposedPrompt: pending.ProposedPrompt,
			Status:         string(pending.Status),
			CreatedAt:      rfc3339(pending.CreatedAt),
		}
	}
	return out
}

// BuildFindingDTO converts a market signal into a finding DTO.
func BuildFindingDTO(sig models.MarketSignal) FindingDTO {
	return FindingDTO{
		ID:          sig.ID,
		Polarity:    string(sig.Polarity),
		Platform:    sig.Platform,
		Quote:       sig.Quote,
		Reason:      sig.Reason,
		SourceURL:   sig.SourceURL,
		SourceTitle: sig.SourceTitle,
		CreatedAt:   rfc3339(sig.CreatedAt),
	}
}

// scoutStatusFor returns the status of a given scout type from a slice, or
// "UNDEPLOYED" if that scout hasn't been created yet (Day 0 still running).
func scoutStatusFor(scouts []models.Scout, t models.ScoutType) string {
	for _, sc := range scouts {
		if sc.ScoutType == t {
			return string(sc.Status)
		}
	}
	return "UNDEPLOYED"
}

// scoutsForDetail assembles the scout DTOs (with pending proposals) for an idea.
func scoutsForDetail(scouts []models.Scout, proposals map[string]*models.PromptProposal) []ScoutDTO {
	out := make([]ScoutDTO, 0, len(scouts))
	for _, sc := range scouts {
		out = append(out, BuildScoutDTO(sc, proposals[sc.ID]))
	}
	if len(out) == 0 {
		return []ScoutDTO{}
	}
	return out
}

// splitSignals partitions signals into recent pros/cons (capped for the UI).
func splitSignals(signals []models.MarketSignal, limit int) ([]FindingDTO, []FindingDTO) {
	pros := make([]FindingDTO, 0, limit)
	cons := make([]FindingDTO, 0, limit)
	for _, sig := range signals {
		f := BuildFindingDTO(sig)
		if sig.Polarity == models.ScoutTypePro {
			if len(pros) < limit {
				pros = append(pros, f)
			}
		} else {
			if len(cons) < limit {
				cons = append(cons, f)
			}
		}
	}
	return pros, cons
}

// deriveTitle produces a concise title from a raw description when the caller
// doesn't supply one explicitly.
func deriveTitle(description string) string {
	d := truncateRunes(description, 100)
	for i, r := range d {
		if r == '\n' {
			d = d[:i]
			break
		}
	}
	if d == "" {
		return "Untitled Idea"
	}
	return d
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	rs := []rune(s)
	return string(rs[:n]) + "…"
}
