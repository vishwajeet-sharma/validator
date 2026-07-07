package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"validator-backend/internal/models"
)

func TestBuildIdeaDTO_ContractMatchesFrontend(t *testing.T) {
	now := time.Now().UTC()
	idea := &models.Idea{
		ID:            "idea-1",
		Title:         "Cloud Kitchen SaaS",
		Description:   "Inventory for ghost kitchens.",
		FrequencyDays: 3,
		Keywords:      []string{"cloud kitchen", "inventory"},
		Channels:      []string{"reddit", "youtube", "social"},
		CustomChannels: []models.CustomChannel{
			{ID: "c1", URL: "https://competitor.com", Label: "Competitor"},
		},
		Status:        "expanded",
		StatusMessage: "Expanded: added 2 keywords",
		TotalPros:     5,
		TotalCons:     2,
		CreatedAt:     now.Add(-48 * time.Hour),
		UpdatedAt:     now.Add(-1 * time.Hour),
	}

	runs := []models.ScoutRun{
		{ID: "run-1", IdeaID: "idea-1", DayNumber: 0, Label: "Initial Scan", RunAt: now.Add(-24 * time.Hour)},
		{ID: "run-2", IdeaID: "idea-1", DayNumber: 3, Label: "Day 3 Scan", RunAt: now.Add(-30 * time.Minute)},
	}
	signals := []models.MarketSignal{
		{ID: "s1", ScoutRunID: "run-1", IdeaID: "idea-1", Polarity: models.PolarityPro,
			Platform: models.PlatformReddit, Quote: "old pro", Reason: "r", SourceURL: "u1", SourceTitle: "t1",
			CreatedAt: now.Add(-24 * time.Hour)},
		{ID: "s2", ScoutRunID: "run-2", IdeaID: "idea-1", Polarity: models.PolarityPro,
			Platform: models.PlatformYoutube, Quote: "new pro", Reason: "r", SourceURL: "u2", SourceTitle: "t2",
			CreatedAt: now.Add(-30 * time.Minute)},
		{ID: "s3", ScoutRunID: "run-2", IdeaID: "idea-1", Polarity: models.PolarityCon,
			Platform: models.PlatformNews, Quote: "new con", Reason: "r", SourceURL: "u3", SourceTitle: "t3",
			CreatedAt: now.Add(-20 * time.Minute)},
	}

	dto := BuildIdeaDTO(idea, runs, signals)

	// --- Frontend field names (camelCase) must be present in JSON ---
	raw, _ := json.Marshal(dto)
	blob := string(raw)
	for _, key := range []string{
		`"scoutingFrequencyDays"`, `"customChannels"`, `"createdAt"`, `"lastUpdated"`,
		`"totalPros"`, `"totalCons"`, `"newSignalsToday"`, `"statusMessage"`, `"cycles"`,
		`"sourceUrl"`, `"sourceTitle"`,
	} {
		if !strings.Contains(blob, key) {
			t.Errorf("JSON missing frontend key %s\n%s", key, blob)
		}
	}
	for _, banned := range []string{`"frequency_days"`, `"created_at"`, `"source_url"`, `"total_pros"`} {
		if strings.Contains(blob, banned) {
			t.Errorf("JSON must not contain snake_case key %s", banned)
		}
	}

	// --- Summary fields map correctly ---
	if dto.ScoutingFrequencyDays != 3 {
		t.Errorf("scoutingFrequencyDays = %d, want 3", dto.ScoutingFrequencyDays)
	}
	if dto.TotalPros != 5 || dto.TotalCons != 2 {
		t.Errorf("totals wrong: pros=%d cons=%d", dto.TotalPros, dto.TotalCons)
	}
	if len(dto.CustomChannels) != 1 || dto.CustomChannels[0].Label != "Competitor" {
		t.Errorf("custom channels not mapped: %+v", dto.CustomChannels)
	}

	// --- Cycles are embedded and ordered, with signals bucketed into pros/cons ---
	if len(dto.Cycles) != 2 {
		t.Fatalf("expected 2 cycles, got %d", len(dto.Cycles))
	}
	c0 := dto.Cycles[0]
	if c0.ID != "run-1" || c0.Day != 0 || c0.Label != "Initial Scan" {
		t.Errorf("cycle 0 metadata wrong: %+v", c0)
	}
	if len(c0.Pros) != 1 || c0.Pros[0].ID != "s1" {
		t.Errorf("cycle 0 pros wrong: %+v", c0.Pros)
	}

	c1 := dto.Cycles[1]
	if len(c1.Pros) != 1 || len(c1.Cons) != 1 {
		t.Errorf("cycle 1 pros/cons split wrong: %+v", c1)
	}
	if c1.Pros[0].SourceURL != "u2" {
		t.Errorf("pro sourceUrl not camelCase-mapped: %+v", c1.Pros[0])
	}
	if c1.Cons[0].Platform != "news" {
		t.Errorf("con platform wrong: %+v", c1.Cons[0])
	}

	// --- newSignalsToday counts signals since start of today UTC ---
	// Two signals (s2, s3) were created within the last hour -> today.
	if dto.NewSignalsToday != 2 {
		t.Errorf("newSignalsToday = %d, want 2", dto.NewSignalsToday)
	}
}

func TestBuildIdeaDTO_EmptyArraysNotNull(t *testing.T) {
	idea := &models.Idea{ID: "x", Title: "t", Description: "d", FrequencyDays: 1}
	dto := BuildIdeaDTO(idea, nil, nil)

	if dto.Keywords == nil || dto.Channels == nil {
		t.Error("keywords/channels must be [] not null for the UI")
	}
	if len(dto.Cycles) != 0 {
		t.Errorf("expected no cycles, got %d", len(dto.Cycles))
	}
}
