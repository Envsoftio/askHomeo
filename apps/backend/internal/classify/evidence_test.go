package classify

import "testing"

func TestSuggestEvidenceSeparatesProtocolFromResults(t *testing.T) {
	tests := []struct {
		name, content, want string
	}{
		{"unstarted protocol", "Study protocol. This randomized controlled trial has not been initiated or registered. Methods will recruit participants in Mumbai.", "study_protocol"},
		{"completed trial", "Randomized controlled trial. Methods: participants were randomized. Results showed primary outcomes after treatment.", "trial_study"},
		{"systematic review", "Systematic review and meta-analysis. Databases were searched with a documented search strategy.", "systematic_review"},
		{"observational study", "Cross-sectional study. Methods and results describe 120 patients.", "observational_study"},
		{"ambiguous", "Homeopathic treatment of respiratory symptoms in India.", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SuggestEvidence(tt.content)
			if got.Category != tt.want {
				t.Fatalf("category = %q, want %q", got.Category, tt.want)
			}
		})
	}
}
