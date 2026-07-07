package workflow

import (
	"regexp"
	"strings"
	"unicode"

	"validator-backend/internal/db"
)

// maxNewKeywordsPerCycle caps runaway watchlist growth so the search radius
// expands steadily without exploding into hundreds of terms.
const maxNewKeywordsPerCycle = 5

var (
	// hashtagRe captures "#cloud-kitchens", "#AI", etc.
	hashtagRe = regexp.MustCompile(`#[\p{L}\p{N}_-]{2,40}`)
	// properNounRe captures sequences of Capitalized Words like "DoorDash",
	// "Ghost Kitchen", "Series A".
	properNounRe = regexp.MustCompile(`\b(?:[A-Z][\p{L}'-]+)(?:\s+[A-Z][\p{L}'-]+){0,2}\b`)
	// quotedTermRe captures "quoted phrases" which scouts often use to name
	// concrete concepts, products or pain points.
	quotedTermRe = regexp.MustCompile(`"([^"]{3,60})"`)
)

// englishStopwords is a small, deliberately conservative stoplist used to avoid
// promoting generic terms ("The", "This", "Market") into the watchlist.
var englishStopwords = map[string]bool{
	"the": true, "this": true, "that": true, "these": true, "those": true,
	"there": true, "their": true, "they": true, "them": true, "then": true,
	"than": true, "thus": true, "and": true, "but": true, "for": true,
	"with": true, "from": true, "into": true, "your": true, "you": true,
	"we": true, "our": true, "his": true, "her": true, "its": true,
	"are": true, "was": true, "were": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "will": true, "would": true,
	"could": true, "should": true, "may": true, "might": true, "can": true,
	"not": true, "any": true, "all": true, "some": true, "such": true,
	"what": true, "which": true, "who": true, "whom": true, "how": true,
	"why": true, "when": true, "where": true, "http": true, "https": true,
	"market": true, "markets": true, "people": true, "users": true,
	"user": true, "company": true, "companies": true, "product": true,
	"products": true, "service": true, "services": true, "software": true,
	"app": true, "apps": true, "tool": true, "tools": true, "platform": true,
	"platforms": true, "time": true, "year": true, "years": true, "day": true,
	"days": true, "week": true, "month": true, "video": true, "content": true,
	"first": true, "last": true, "new": true, "best": true, "top": true,
	"most": true, "more": true, "many": true, "much": true, "one": true,
	"two": true, "three": true, "revenue": true, "business": true,
}

// evaluateRadiusMutation inspects the raw pro/con quotes harvested in the latest
// scout cycle and returns newly discovered keywords (hashtags, proper nouns,
// quoted concepts) that are not already part of the active watchlist.
//
// These are appended to the Restate workflow state by the caller so the next
// cycle scours a broader, evidence-driven search radius.
func evaluateRadiusMutation(active []string, signals []db.SignalInput) []string {
	present := make(map[string]bool, len(active))
	for _, k := range active {
		present[normalizeKeyword(k)] = true
	}

	candidates := make([]string, 0)
	seen := make(map[string]bool, 0)

	add := func(term string) {
		norm := normalizeKeyword(term)
		if norm == "" || present[norm] || seen[norm] {
			return
		}
		if englishStopwords[norm] {
			return
		}
		seen[norm] = true
		candidates = append(candidates, term)
	}

	for _, sig := range signals {
		text := sig.Quote
		if strings.TrimSpace(sig.Reason) != "" {
			text += " " + sig.Reason
		}

		for _, m := range hashtagRe.FindAllString(text, -1) {
			add(strings.TrimPrefix(m, "#"))
		}
		for _, m := range properNounRe.FindAllString(text, -1) {
			add(m)
		}
		for _, m := range quotedTermRe.FindAllStringSubmatch(text, -1) {
			add(m[1])
		}
	}

	if len(candidates) > maxNewKeywordsPerCycle {
		candidates = candidates[:maxNewKeywordsPerCycle]
	}
	return candidates
}

// normalizeKeyword lower-cases, trims and collapses internal whitespace so that
// "Cloud Kitchen" and "cloud  kitchen" compare equal.
func normalizeKeyword(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	return strings.Join(strings.Fields(s), " ")
}
