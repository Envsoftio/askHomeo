package httpapi

import (
	"context"
	"strings"
	"testing"
)

func TestResearchSectionsUseEachVerifiedFindingOnce(t *testing.T) {
	findings := "Nash describes restlessness with fear [E1]. Farrington notes restlessness at night [E2]. Nash contrasts the symptoms with weakness [E3]. Farrington adds a different setting [E4]."
	hits := []hit{{Author: "E. B. Nash"}, {Author: "Farrington, E. A."}, {Author: "E. B. Nash"}, {Author: "Farrington, E. A."}}
	sections, err := organizeResearchSections(context.Background(), "How do the authors describe restlessness?", "Nash and Farrington describe different settings [E1] [E2].", findings, hits, func(_ context.Context, _, user string) (string, error) {
		if !strings.Contains(user, "restlessness") {
			t.Fatal("question was not passed to section organizer")
		}
		return `{"groups":[{"title":"Fear and weakness","indices":[1,3]},{"title":"Timing and setting","indices":[2,4]}]}`, nil
	})
	if err != nil || len(sections) != 3 || sections[1].Title != "Fear and weakness" || !strings.Contains(sections[2].Body, "[E4]") {
		t.Fatalf("sections=%+v err=%v", sections, err)
	}
}

func TestResearchSectionsRejectInvalidOutlineWithoutLosingClaims(t *testing.T) {
	findings := "First finding [E1]. Second finding [E2]. Third finding [E3]. Fourth finding [E4]."
	hits := []hit{{Author: "E. B. Nash"}, {Author: "Farrington, E. A."}, {Author: "E. B. Nash"}, {Author: "Farrington, E. A."}}
	for _, raw := range []string{
		`{"groups":[{"title":"One","indices":[1,2]},{"title":"Two","indices":[2,3,4]}]}`,
		`{"groups":[{"title":"One","indices":[1,2]},{"title":"Two","indices":[3]}]}`,
		`{"groups":[{"title":"Unproved claim: cure","indices":[1,2]},{"title":"Two","indices":[3,4]}]}`,
		`not JSON`,
	} {
		sections, err := organizeResearchSections(context.Background(), "a question", "", findings, hits, func(context.Context, string, string) (string, error) { return raw, nil })
		if err != nil || len(sections) != 2 || sections[0].Title != "Findings from Nash" || sections[1].Title != "Findings from Farrington" {
			t.Fatalf("invalid outline %s produced %+v, %v", raw, sections, err)
		}
		combined := sections[0].Body + " " + sections[1].Body
		for _, label := range []string{"[E1]", "[E2]", "[E3]", "[E4]"} {
			if strings.Count(combined, label) != 1 {
				t.Fatalf("fallback lost or repeated %s: %+v", label, sections)
			}
		}
	}
}
