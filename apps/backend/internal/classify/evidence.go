package classify

import "strings"

const EvidenceVersion = "evidence-rules-v1"

type EvidenceSuggestion struct {
	Category string `json:"category"`
	State    string `json:"state"`
	Reason   string `json:"reason"`
}

// SuggestEvidence uses document text, not the bibliographic type supplied by a
// search provider. Ambiguous and mixed documents stay unknown for review.
func SuggestEvidence(content string) EvidenceSuggestion {
	// The opening abstract and methods carry the article's own design. Later
	// references to other study types must not determine this source's label.
	if len(content) > 8000 {
		content = content[:8000]
	}
	text := strings.ToLower(content)
	has := func(cues ...string) bool {
		for _, cue := range cues {
			if strings.Contains(text, cue) {
				return true
			}
		}
		return false
	}
	suggest := func(category, reason string) EvidenceSuggestion {
		return EvidenceSuggestion{category, "suggested", reason}
	}
	unknown := EvidenceSuggestion{"unknown", "uncertain", "No sufficiently specific evidence-type cues in extracted content; review the source."}
	if strings.TrimSpace(text) == "" {
		return unknown
	}
	if has("not initiated or registered", "trial has not been initiated") || has("study protocol", "trial protocol", "protocol for a", "protocol of a") && !has("participants were randomized", "participants were randomised", "results showed") {
		return suggest("study_protocol", "Extracted content identifies a protocol or a trial that has not started; no outcomes are implied.")
	}
	if has("systematic review", "meta-analysis", "meta analysis") && has("search strategy", "database search", "databases were searched", "pooled estimate", "prisma") {
		return suggest("systematic_review", "Extracted content describes a systematic search or pooled analysis.")
	}
	if has("trial registration", "clinicaltrials.gov", "ctri/") && !has("participants were randomized", "participants were randomised", "results showed", "primary outcome was") {
		return suggest("trial_registration", "Extracted content identifies a trial registration without reported outcomes.")
	}
	if has("case report", "case series") && has("patient", "patients", "case presentation") {
		return suggest("published_case", "Extracted content contains a case report or case series with patient narrative.")
	}
	if has("observational study", "cohort study", "case-control study", "cross-sectional study") && has("participants", "patients", "methods", "results") {
		return suggest("observational_study", "Extracted methods identify an observational study.")
	}
	if has("randomized controlled trial", "randomised controlled trial", "randomized clinical trial", "randomised clinical trial") && has("results", "outcomes", "participants were randomized", "participants were randomised") {
		return suggest("trial_study", "Extracted content describes a completed randomized trial with outcomes or results.")
	}
	if has("clinical practice guideline", "practice guideline", "safety guideline") && has("recommendation", "recommendations") {
		return suggest("guideline_safety", "Extracted content identifies guidance with recommendations.")
	}
	return unknown
}
