package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func mockQuoteCheck(user string, hits []hit) string {
	quotes := []map[string]string{}
	for i, h := range hits {
		label := fmt.Sprintf("E%d", i+1)
		if strings.Contains(user, "["+label+"] "+h.Text) {
			quotes = append(quotes, map[string]string{"id": label, "text": h.Text})
		}
	}
	result, _ := json.Marshal(map[string]any{"quotes": quotes})
	return string(result)
}

func TestGroupedClaimVerificationKeepsOnlyQuotedRelevantClaims(t *testing.T) {
	hits := []hit{
		{ID: uuid.New(), Text: "GELSEMIUM NITIDUM\nGelsemium headache is relieved by urine."},
		{ID: uuid.New(), Text: "Like\nAconite, Gelsemium, and Silicea, the headache ends with urine."},
	}
	calls := 0
	answer, labels, rejected, checks, err := verifyClaimSentencesBatched(context.Background(), "What does Nash say about headache relief?", "Nash says Gelsemium headache is relieved by urine [E1]. Gelsemium's headache ends with urine like Aconite and Silicea [E2].", hits, func(_ context.Context, _, user string) (string, error) {
		calls++
		if calls == 1 {
			return `{"checks":[{"claim":1,"quotes":[{"id":"E1","text":"Gelsemium headache is relieved by urine"}]},{"claim":2,"quotes":[{"id":"E2","text":"Like Aconite, Gelsemium, and Silicea"}]}]}`, nil
		}
		if !strings.Contains(user, `"text":"GELSEMIUM NITIDUM`) {
			t.Fatal("grouped verdict quotation omitted source heading")
		}
		return `{"checks":[{"claim":1,"supported":true,"relevant":true},{"claim":2,"supported":true,"relevant":true}]}`, nil
	})
	if err != nil || calls != 2 || rejected != 1 || len(labels) != 1 || labels[0] != "E1" || len(checks) != 2 || checks[0].Decision != "supported" || checks[1].Decision != "missing_excerpt" || len(checks[0].Supports) != 1 || !strings.HasPrefix(checks[0].Supports[0].Text, "GELSEMIUM NITIDUM") || !strings.Contains(answer, "Gelsemium") || strings.Contains(answer, "Silicea") {
		t.Fatalf("answer=%q labels=%v rejected=%d checks=%v calls=%d err=%v", answer, labels, rejected, checks, calls, err)
	}
}

func TestValidResearchQuestionNeedsSearchableTerm(t *testing.T) {
	for _, q := range []string{"", "g", "GI", "???", "g!"} {
		if validResearchQuestion(q) {
			t.Fatalf("accepted vague question %q", q)
		}
	}
	for _, q := range []string{"flu", "mood", "Nux", "What does Nash say about headaches?"} {
		if !validResearchQuestion(q) {
			t.Fatalf("rejected searchable question %q", q)
		}
	}
}

func TestSelectEvidenceBalancesAuthors(t *testing.T) {
	nash, farrington := uuid.New(), uuid.New()
	candidates := make([]hit, 0, 12)
	for i := 0; i < 8; i++ {
		candidates = append(candidates, hit{ID: uuid.New(), SourceID: farrington})
	}
	for i := 0; i < 4; i++ {
		candidates = append(candidates, hit{ID: uuid.New(), SourceID: nash})
	}
	selected := selectEvidence(candidates, 8)
	counts := map[uuid.UUID]int{}
	for _, h := range selected {
		counts[h.SourceID]++
	}
	if counts[nash] != 4 || counts[farrington] != 4 {
		t.Fatalf("unbalanced evidence: %v", counts)
	}
	if got := len(selectEvidence(candidates[:8], 8)); got != 8 {
		t.Fatalf("single-source evidence count = %d", got)
	}
}

func TestSupplementUnrepresentedSourceAddsVerifiedClaim(t *testing.T) {
	nash, farrington := uuid.New(), uuid.New()
	hits := []hit{
		{ID: uuid.New(), SourceID: nash, Author: "Nash", Text: "Nash links Aconite with anxiety and fear."},
		{ID: uuid.New(), SourceID: farrington, Author: "Farrington", Text: "Farrington links Bromine with anxiety."},
	}
	answer, cites, omitted, err := supplementUnrepresentedSources(context.Background(), "anxiety", "Farrington links Bromine with anxiety [E2].", []string{"E2"}, hits, func(_ context.Context, system, user string) (string, error) {
		if strings.Contains(system, "Add two or three") {
			return `{"answer":"Nash links Aconite with anxiety and fear [E1].","citations":["E1"]}`, nil
		}
		if strings.Contains(system, "For each cited passage") {
			return mockQuoteCheck(user, hits), nil
		}
		return `{"supported":true,"relevant":true}`, nil
	})
	if err != nil || omitted != 0 || len(cites) != 2 || !strings.Contains(answer, "Nash links Aconite") {
		t.Fatalf("supplement answer=%q cites=%v omitted=%d err=%v", answer, cites, omitted, err)
	}
}

func TestExtendAnswerUsesOnlyVerifiedUnusedPassage(t *testing.T) {
	source := uuid.New()
	hits := []hit{
		{ID: uuid.New(), SourceID: source, Author: "Nash", Text: "Nash links Aconite with anxiety."},
		{ID: uuid.New(), SourceID: source, Author: "Nash", Text: "Nash describes Arsenic anxiety at night."},
	}
	answer, cites, omitted, err := extendAnswerFromUnusedEvidence(context.Background(), "anxiety", "Nash links Aconite with anxiety [E1].", []string{"E1"}, hits, 2, func(_ context.Context, system, user string) (string, error) {
		if strings.Contains(system, "Write exactly one new") {
			return `{"answer":"Nash describes Arsenic anxiety at night [E2].","citations":["E2"]}`, nil
		}
		if strings.Contains(system, "For each cited passage") {
			return mockQuoteCheck(user, hits), nil
		}
		return `{"supported":true,"relevant":true}`, nil
	})
	if err != nil || omitted != 0 || len(cites) != 2 || !strings.Contains(answer, "Arsenic anxiety") {
		t.Fatalf("extension answer=%q cites=%v omitted=%d err=%v", answer, cites, omitted, err)
	}
}

func TestVerifierChecksAnInitialNameAgainstCitedWords(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Author: "Nash", Text: "Chimaphila umbellata is linked to anxiety and agitation."}}
	answer := "Chiaphila umbellata is linked to anxiety and agitation [E1]."
	got, _, omitted, err := verifyClaimSentences(context.Background(), "Chimaphila umbellata", answer, hits, func(_ context.Context, system, user string) (string, error) {
		if strings.Contains(system, "For each cited passage") {
			return mockQuoteCheck(user, hits), nil
		}
		return `{"supported":false,"relevant":true}`, nil
	})
	if err != nil || omitted != 1 || got != "" {
		t.Fatalf("misspelled initial name survived: %q omitted=%d err=%v", got, omitted, err)
	}
}

func TestValidateDraftRequiresCitationAtSentenceEnd(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "Nash describes anxiety and fear.", Author: "Nash"}}
	if _, ok := validateDraft("Nash describes anxiety [E1], and then discusses fear.", []string{"E1"}, hits); ok {
		t.Fatal("accepted a citation that did not cover the end of the sentence")
	}
	if _, ok := validateDraft("Nash describes anxiety [E1,E2] [E1].", []string{"E1"}, hits); ok {
		t.Fatal("accepted malformed combined citation")
	}
}

func TestSynthesizeResearchLeadUsesVerifiedCitations(t *testing.T) {
	hits := []hit{
		{ID: uuid.New(), SourceID: uuid.New(), Text: "Nash describes anxiety with fear and restlessness.", Author: "Nash"},
		{ID: uuid.New(), SourceID: uuid.New(), Text: "Farrington describes anxiety with fear and restlessness.", Author: "Farrington"},
	}
	lead, labels, err := synthesizeResearchLead(context.Background(), "anxiety", "Nash describes anxiety with fear [E1]. Farrington describes anxiety with restlessness [E2].", []string{"E1", "E2"}, hits, func(_ context.Context, system, user string) (string, error) {
		if strings.Contains(system, "research thesis") {
			return `{"answer":"Nash and Farrington describe anxiety with fear and restlessness.","citations":["E1","E2"]}`, nil
		}
		if strings.Contains(system, "For each cited passage") {
			return mockQuoteCheck(user, hits), nil
		}
		return `{"supported":true,"relevant":true}`, nil
	})
	if err != nil || !strings.Contains(lead, "[E1] [E2].") || len(labels) != 2 {
		t.Fatalf("lead=%q labels=%v err=%v", lead, labels, err)
	}
}

func TestSynthesizeResearchLeadRepairsListOpening(t *testing.T) {
	hits := []hit{
		{ID: uuid.New(), SourceID: uuid.New(), Text: "Nash describes anxiety with fear and restlessness.", Author: "Nash"},
		{ID: uuid.New(), SourceID: uuid.New(), Text: "Farrington describes anxiety with fear and restlessness.", Author: "Farrington"},
	}
	repairs := 0
	lead, labels, err := synthesizeResearchLead(context.Background(), "anxiety", "Nash describes anxiety with fear [E1]. Farrington describes anxiety with restlessness [E2].", []string{"E1", "E2"}, hits, func(_ context.Context, system, user string) (string, error) {
		if strings.Contains(system, "Write one research comparison") {
			repairs++
			return `{"answer":"Nash and Farrington describe anxiety with fear and restlessness.","citations":["E1","E2"]}`, nil
		}
		if strings.Contains(system, "research thesis") {
			return `{"answer":"Nash notes fear; Farrington notes restlessness.","citations":["E1","E2"]}`, nil
		}
		if strings.Contains(system, "For each cited passage") {
			return mockQuoteCheck(user, hits), nil
		}
		return `{"supported":true,"relevant":true}`, nil
	})
	if err != nil || repairs != 1 || strings.Contains(lead, ";") || len(labels) != 2 {
		t.Fatalf("lead=%q labels=%v repairs=%d err=%v", lead, labels, repairs, err)
	}
}

func TestVerifyClaimSentencesDropsUnsupportedComparison(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "Gelsemium headache is relieved by a profuse flow of urine. Lac defloratum has a profuse flow too, but its pain is not so markedly relieved.", Author: "Nash"}}
	answer := "Nash says Gelsemium headache is relieved by urine [E1]. Nash says Gelsemium is less relieved than Lac defloratum [E1]."
	calls := 0
	got, labels, omitted, err := verifyClaimSentences(context.Background(), "Gelsemium headache", answer, hits, func(_ context.Context, system, user string) (string, error) {
		calls++
		if strings.Contains(system, "For each cited passage") {
			return mockQuoteCheck(user, hits), nil
		}
		if strings.Contains(user, "less relieved") {
			return `{"supported":false,"relevant":true}`, nil
		}
		return `{"supported":true,"relevant":true}`, nil
	})
	if err != nil || calls != 4 || omitted != 1 || strings.Contains(got, "less relieved") || len(labels) != 1 || labels[0] != "E1" {
		t.Fatalf("claim check: answer=%q labels=%v omitted=%d calls=%d err=%v", got, labels, omitted, calls, err)
	}
}

func TestClaimRequiresExactQuotationFromEachCitation(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "The patient is restless and moves about."}, {ID: uuid.New(), Text: "The patient is anxious and fearful."}}
	answer := "The authors describe restlessness with anxiety [E1] [E2]."
	got, _, omitted, err := verifyClaimSentences(context.Background(), "restlessness and anxiety", answer, hits, func(_ context.Context, system, _ string) (string, error) {
		if strings.Contains(system, "For each cited passage") {
			return `{"quotes":[{"id":"E1","text":"The patient is restless and moves about."},{"id":"E2","text":"Invented words not in the passage."}]}`, nil
		}
		t.Fatal("unsupported quotation reached semantic verifier")
		return "", nil
	})
	if err != nil || omitted != 1 || got != "" {
		t.Fatalf("accepted unanchored quotation: %q omitted=%d err=%v", got, omitted, err)
	}
	if !exactPassageQuote("A patient is\nrestless and fearful.", "patient is restless and fearful") || exactPassageQuote("A patient is restless.", "patient is anxious") {
		t.Fatal("quote matcher did not check stored passage text")
	}
	if !exactPassageQuote("The patient is very rest-\nless, and moves about.", "patient is very restless and moves about") {
		t.Fatal("quote matcher rejected an OCR line break and punctuation")
	}
}

func TestClaimChecksShortParaphrasedTopicAgainstQuote(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "The patient is melancholy and avoids company."}}
	got, labels, omitted, err := verifyClaimSentences(context.Background(), "bad mood", "The author describes low mood and social withdrawal [E1].", hits, func(_ context.Context, system, user string) (string, error) {
		if strings.Contains(system, "For each cited passage") {
			return mockQuoteCheck(user, hits), nil
		}
		return `{"supported":true,"relevant":true}`, nil
	})
	if err != nil || omitted != 0 || got == "" || len(labels) != 1 {
		t.Fatalf("rejected paraphrased topic: %q labels=%v omitted=%d err=%v", got, labels, omitted, err)
	}
}

func TestValidateDraftRejectsUnsupportedCitation(t *testing.T) {
	h := []hit{{ID: uuid.New(), Text: "Nash compares two remedies.", Author: "Nash"}, {ID: uuid.New(), Text: "Farrington discusses other symptoms.", Author: "Farrington"}}
	if _, ok := validateDraft("Nash compares two remedies [E1].", []string{"E1"}, h); !ok {
		t.Fatal("valid citation rejected")
	}
	for _, tc := range []struct {
		answer string
		ids    []string
	}{{"Nash compares two remedies [E3].", []string{"E3"}}, {"Nash compares two remedies.", []string{"E1"}}, {"Nash compares two remedies [E1]. Another claim.", []string{"E1"}}, {"Nash compares two remedies [E1].", []string{"E2"}}} {
		if _, ok := validateDraft(tc.answer, tc.ids, h); ok {
			t.Fatalf("accepted unsupported draft: %q", tc.answer)
		}
	}
}
func TestTenthCitationIsAcceptedOnlyWhenPresent(t *testing.T) {
	hits := make([]hit, 10)
	for i := range hits {
		hits[i] = hit{ID: uuid.New(), Text: "Farrington describes Lachesis.", Author: "Farrington"}
	}
	if _, ok := validateDraft("Farrington describes Lachesis [E10].", []string{"E10"}, hits); !ok {
		t.Fatal("valid tenth passage rejected")
	}
	if _, ok := validateDraft("Farrington describes Lachesis [E11].", []string{"E11"}, hits); ok {
		t.Fatal("unknown passage accepted")
	}
}
func TestAuthorScope(t *testing.T) {
	for question, want := range map[string]string{
		"What does Nash say about Nux vomica?":     "nash",
		"What does Farrington say about Lachesis?": "farrington",
		"Compare Nash and Farrington on Lachesis":  "",
		"What is Lachesis?":                        "",
	} {
		if got := authorScope(question); got != want {
			t.Errorf("%q: got %q, want %q", question, got, want)
		}
	}
}
func TestSearchPlanNeedsTwoDistinctBoundedQueries(t *testing.T) {
	got, err := parseSearchPlan(`{"queries":["Nash Lachesis symptoms","Farrington Lachesis comparisons"]}`, "How do Nash and Farrington describe Lachesis?")
	if err != nil || len(got) != 2 {
		t.Fatalf("valid plan rejected: %v", err)
	}
	for _, raw := range []string{
		`{"queries":["same query","same query"]}`,
		`{"queries":["only one query"]}`,
		`{"queries":["How do Nash and Farrington describe Lachesis?","other query"]}`,
		`not JSON`,
	} {
		if _, err := parseSearchPlan(raw, "How do Nash and Farrington describe Lachesis?"); err == nil {
			t.Fatalf("invalid plan accepted: %s", raw)
		}
	}
}
func TestRetainCitedSentencesDropsUnattributedClaim(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "Farrington describes Lachesis.", Author: "Farrington"}, {ID: uuid.New(), Text: "Nash describes Lachesis.", Author: "Nash"}}
	answer, labels, omitted := retainSupportedSentences("An unsupported claim. Farrington describes Lachesis [E1]. Nash describes Lachesis [E2].", hits)
	if strings.Contains(answer, "unsupported") || len(labels) != 2 || omitted != 1 {
		t.Fatalf("failed to remove uncited claim: %q, %v", answer, labels)
	}
	if _, ok := validateDraft(answer, labels, hits); !ok {
		t.Fatalf("retained cited answer rejected: %q", answer)
	}
}
func TestRetainCitedSentencesNamesFirstAuthorAfterDroppedLead(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "Farrington describes Lachesis.", Author: "Farrington, E. A."}}
	answer, labels, omitted := retainSupportedSentences("Uncited introduction. He describes Lachesis [E1].", hits)
	if answer != "Farrington describes Lachesis [E1]." || len(labels) != 1 || labels[0] != "E1" || omitted != 1 {
		t.Fatalf("dangling pronoun remained: %q, %v", answer, labels)
	}
}
func TestRetainSupportedMoodClaimsDropsWrongRemedyCitation(t *testing.T) {
	hits := make([]hit, 8)
	for i := range hits {
		hits[i].ID = uuid.New()
	}
	hits[0].Author = "E. B. Nash"
	hits[0].Text = "On the mind it exerts a very depressing influence. Melaiicholy mood; sadness; hopelessness; is apt to look on the dark side of everything."
	hits[2].Author = "Farrington"
	hits[2].Text = "The Mercurius patient illustrates plethora with anxiety and restlessness. The patient becomes anxious and irritable."
	hits[7].Author = "Farrington"
	hits[7].Text = "Hepar develops a mood: Sadness, unpleasant events return to mind; the slightest thing makes him break out into violence. Only Hepar has such violent outbursts of passion."
	draft := "The text discusses bad moods. Nash mentions a depressive influence on the mind, with melancholy, sadness, hopelessness, and a tendency to see the dark side of things [E1]. Farrington notes that Ignatia and Nux vomica cause anxiety and restlessness [E3]. Hepar is described as having a mood of sadness, unpleasant events returning to mind, and violent outbursts of passion [E8]."
	answer, labels, omitted := retainSupportedSentences(draft, hits)
	if omitted != 2 || strings.Contains(answer, "Ignatia") || strings.Contains(answer, "Nux vomica") || len(labels) != 2 || labels[0] != "E1" || labels[1] != "E8" {
		t.Fatalf("wrongly cited mood claim survived: %q, %v, omitted=%d", answer, labels, omitted)
	}
	if _, ok := validateDraft(answer, labels, hits); !ok {
		t.Fatalf("retained mood claims failed final validation: %q", answer)
	}
}
func TestAnswerSentencesKeepRemedyAbbreviationAndInitials(t *testing.T) {
	text := "Natrum mur. produces a headache [E4]. E. B. Nash also describes Ignatia [E2]."
	parts := splitAnswerSentences(text)
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "Natrum mur. produces") || !strings.HasPrefix(parts[1], "E. B. Nash") {
		t.Fatalf("split within a remedy name or author initials: %#v", parts)
	}
	hits := make([]hit, 4)
	for i := range hits {
		hits[i].ID = uuid.New()
	}
	hits[3].Text = "Natrum mur. produces a headache, worse from use of the mind."
	hits[3].Author = "Farrington"
	answer, labels, omitted := retainSupportedSentences("Natrum mur. produces a headache worse from use of the mind [E4].", hits)
	if omitted != 0 || answer != "Natrum mur. produces a headache worse from use of the mind [E4]." || len(labels) != 1 || labels[0] != "E4" {
		t.Fatalf("lost remedy subject: %q, %v, omitted=%d", answer, labels, omitted)
	}
}
func TestCitationStructureAllowsOCRWrapAndParaphrase(t *testing.T) {
	passage := "Natrum mur. produces a headache, worse from any use of the mind. In the morning there is throbbing, mostly in the fore-\nhead, as if from little hammers beating in the head."
	sentence := "Natrum mur is linked to a headache that worsens with mental activity and is characterized by throbbing pain in the forehead [E4]."
	hits := make([]hit, 4)
	for i := range hits {
		hits[i].ID = uuid.New()
	}
	hits[3].Text = passage
	hits[3].Author = "Farrington"
	if _, ok := validateDraft(sentence, []string{"E4"}, hits); !ok {
		t.Fatal("citation validation rejected a supported Natrum mur claim")
	}
}
func TestValidateDraftRequiresNamesInCitedPassage(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "Carbo vegetabilis is compared with China and Lycopodium.", Title: "Nash", Author: "Nash"}, {ID: uuid.New(), Text: "Colchicumn is mentioned elsewhere.", Title: "Nash", Author: "Nash"}}
	if _, ok := validateDraft("Nash compares Carbo vegetabilis with China, Lycopodium, and Colchicumn [E1].", []string{"E1"}, hits); ok {
		t.Fatal("accepted remedy absent from cited passage")
	}
	if _, ok := validateDraft("Nash compares Carbo vegetabilis with China and Lycopodium [E1].", []string{"E1"}, hits); !ok {
		t.Fatal("rejected supported names")
	}
}

func TestParagraphFinalCitationIsCheckedPerSentence(t *testing.T) {
	passage := `KALI MURIATICUM Is one of the so-called " Bio-chemic " remedies, or one of the twelve tissue remedies, claimed by Schiissler to be able to cure all the ills that flesh is heir to. It has not been proven enough to know half its real value. Clinical use in the potencies, ranging from the 3d to the 30th, has proven that it is a remedy of undoubted great value. It is of use in the second stage of inflammations or the stage of interstitial exudation in any part of the body, and here it is not, so far as yet known, attended with the danger of Kali hydriodicum.`
	answer := `Kali muriaticum is one of the so-called 'Bio-chemic' remedies, or one of the twelve tissue remedies, claimed by Schiissler to be able to cure all the ills that flesh is heir to. It has not been proven enough to know half its real value. Clinical use in the potencies, ranging from the 3d to the 30th, has proven that it is a remedy of undoubted great value. It is of use in the second stage of inflammations or the stage of interstitial exudation in any part of the body, and here it is not, so far as yet known, attended with the danger of Kali hydriodicum.[E1]`
	hits := []hit{{ID: uuid.New(), Text: passage, Title: "Nash", Author: "Nash"}}
	if _, ok := validateDraft(answer, []string{"E1"}, hits); ok {
		t.Fatal("accepted uncited sentences before normalization")
	}
	expanded := normalizeDraftCitations(answer, []string{"E1"})
	if got := strings.Count(expanded, "[E1]"); got != 4 {
		t.Fatalf("expected four sentence citations, got %d: %s", got, expanded)
	}
	if _, ok := validateDraft(expanded, []string{"E1"}, hits); !ok {
		t.Fatal("rejected supported paragraph after citation expansion")
	}
}
func TestExactQuoteUsesSelectedPassage(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "Nash calls this a remedy of undoubted great value.", Title: "Nash", Author: "Nash"}}
	if _, ok := validateDraft(`Nash calls it “undoubted great value” [E1].`, []string{"E1"}, hits); !ok {
		t.Fatal("rejected exact quote from selected passage")
	}
	if _, ok := validateDraft(`Nash calls it “unproven great value” [E1].`, []string{"E1"}, hits); ok {
		t.Fatal("accepted quote absent from selected passage")
	}
}

func TestDeclaredSinglePassageCitesEachSentence(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "Kali muriaticum is a tissue remedy. Clinical use is described.", Title: "Nash", Author: "Nash"}}
	raw := "Kali muriaticum is a tissue remedy. Clinical use is described."
	expanded := normalizeDraftCitations(raw, []string{"E1"})
	if strings.Count(expanded, "[E1]") != 2 {
		t.Fatalf("citations were not expanded: %s", expanded)
	}
	if _, ok := validateDraft(expanded, []string{"E1"}, hits); !ok {
		t.Fatal("rejected declared passage after expansion")
	}
	if got := normalizeDraftCitations(raw, []string{"E1", "E2"}); got != raw {
		t.Fatal("assigned ambiguous multi-passage citation")
	}
}

func TestMixedPassageDraftFillsOnlyUnambiguousGaps(t *testing.T) {
	raw := `Nux vomica is adapted to complaints from drinks, debauchery, and broken rest. [E1] One thing is apt to be present in these cases; namely the rectal symptoms. We ought not to leave Nux vomica without mentioning headaches. The headaches occur with gastric affections. [E1] Frequent ineffectual desire for stool is a guiding symptom. [E5] Pain is worst two hours after meals. [E3] Nausea and sour taste are also mentioned. [E4] Patients may be easily offended. [E6] Nux vomica requires a specific indication. [E1]`
	got := normalizeDraftCitations(raw, []string{"E1", "E5", "E3", "E4", "E6"})
	if strings.Count(got, "[E1]") != 5 {
		t.Fatalf("expected E1 on five claims, got: %s", got)
	}
	if !strings.Contains(got, "rectal symptoms [E1].") || !strings.Contains(got, "mentioning headaches [E1].") {
		t.Fatalf("missed unambiguous claims: %s", got)
	}
	ambiguous := normalizeDraftCitations("First claim [E1]. Uncited claim. Last claim [E5].", []string{"E1", "E5"})
	if strings.Contains(ambiguous, "Uncited claim [") {
		t.Fatalf("assigned ambiguous claim: %s", ambiguous)
	}
}

func TestQuoteMustAppearInItsOwnCitation(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "Nash says the body is cold.", Title: "Nash", Author: "Nash"}, {ID: uuid.New(), Text: "Nash describes undoubted great value.", Title: "Nash", Author: "Nash"}}
	answer := `Nash calls it “undoubted great value” [E1]. A second passage is also cited [E2].`
	if _, ok := validateDraft(answer, []string{"E1", "E2"}, hits); ok {
		t.Fatal("accepted quote supported only by a different citation")
	}
}

func TestLocatePassageQuotePreservesOCRCharacterOffsets(t *testing.T) {
	passage := "Élan: fear some-\nthing is going to happen, ever present."
	start, end, ok := locatePassageQuote(passage, "fear something is going to happen ever present")
	if !ok {
		t.Fatal("quote span missing")
	}
	if got := string([]rune(passage)[start:end]); got != "fear some-\nthing is going to happen, ever present" {
		t.Fatalf("wrong original excerpt: %q", got)
	}
}
