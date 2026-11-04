package app

import (
	"testing"

	"teleport/internal/model"
)

func TestFinderRelevanceUsesProximityAndMatchingCase(t *testing.T) {
	near, _ := searchRank(model.Ticket{ID: 1, Title: "A UX issue"}, "UX")
	far, _ := searchRank(model.Ticket{ID: 2, Title: "A very distant UX issue"}, "UX")
	otherCase, _ := searchRank(model.Ticket{ID: 3, Title: "A ux issue"}, "UX")
	if near <= far || near <= otherCase || otherCase == 0 {
		t.Fatalf("unexpected proximity or case ordering: near=%d far=%d otherCase=%d", near, far, otherCase)
	}
	unicode, _ := searchRank(model.Ticket{ID: 4, Title: "Straße checklist"}, "STRASSE checklist")
	if unicode == 0 {
		t.Fatal("Unicode case-insensitive matching failed")
	}
	id, _ := searchRank(model.Ticket{ID: 101, Title: "Unrelated"}, "#101")
	if id != 10000 {
		t.Fatalf("exact ID rank: %d", id)
	}
}
