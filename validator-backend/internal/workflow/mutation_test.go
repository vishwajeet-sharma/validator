package workflow

import (
	"strings"
	"testing"

	"validator-backend/internal/db"
	"validator-backend/internal/models"
)

func sig(platform, quote, reason string) db.SignalInput {
	return db.SignalInput{
		Platform:    models.Platform(platform),
		Quote:       quote,
		Reason:      reason,
		SourceURL:   "https://example.com/" + platform,
		SourceTitle: platform,
	}
}

func TestEvaluateRadiusMutation_HashtagsAndProperNouns(t *testing.T) {
	active := []string{"cloud kitchen", "inventory management"}
	signals := []db.SignalInput{
		sig("reddit", "Loving #GhostKitchens and DoorDash for deliveries.", "Mentions competitors"),
		sig("news", "\"food waste analytics\" is a growing niche.", "New category signal"),
		sig("youtube", "The market is big and the time is now.", "Generic, should be filtered"),
	}

	got := evaluateRadiusMutation(active, signals)
	blob := strings.ToLower(strings.Join(got, " "))

	for _, want := range []string{"ghostkitchens", "doordash"} {
		if !strings.Contains(blob, want) {
			t.Errorf("expected new keywords to contain %q, got %v", want, got)
		}
	}
	for _, banned := range []string{"market", "cloud kitchen", "inventory management"} {
		if strings.Contains(blob, banned) {
			t.Errorf("watchlist should not be polluted with %q, got %v", banned, got)
		}
	}
}

func TestEvaluateRadiusMutation_RespectsCap(t *testing.T) {
	active := []string{"a"}
	signals := []db.SignalInput{
		sig("reddit", "#AlphaBeta #GammaDelta #EpsilonZeta #EtaTheta #IotaKappa #LambdaMu", "r"),
	}
	got := evaluateRadiusMutation(active, signals)
	if len(got) > maxNewKeywordsPerCycle {
		t.Fatalf("expected at most %d new keywords, got %d (%v)", maxNewKeywordsPerCycle, len(got), got)
	}
}

func TestEvaluateRadiusMutation_NoDuplicates(t *testing.T) {
	active := []string{"ai"}
	signals := []db.SignalInput{
		sig("reddit", "#AI is great, #AI rocks, mentions OpenAI and OpenAI again.", "dupes"),
	}
	got := evaluateRadiusMutation(active, signals)

	for _, g := range got {
		if normalizeKeyword(g) == "ai" {
			t.Errorf("already-active keyword 'ai' was re-added: %v", got)
		}
	}
	count := 0
	for _, g := range got {
		if normalizeKeyword(g) == "openai" {
			count++
		}
	}
	if count > 1 {
		t.Errorf("OpenAI duplicated %d times in %v", count, got)
	}
}
