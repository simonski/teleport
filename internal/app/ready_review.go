package app

import (
	"encoding/json"
	"teleport/internal/model"
)

func readyReviewRequired(st *model.State, t model.Ticket) bool {
	p, ok := phaseByID(st.Workflows[t.Workflow], t.Phase)
	return ok && p.Role == "Ready" && p.RequireReadyReview
}
func readyReviewBasis(st *model.State, t model.Ticket) string {
	versions := map[int64]int64{}
	for _, id := range effectiveDependencies(st, t) {
		versions[id] = st.Tickets[id].Version
	}
	encoded, _ := json.Marshal(struct {
		Scope, Workflow, Phase string
		Visit                  int64
		Dependencies           map[int64]int64
	}{executionBasis(st, t), t.Workflow, t.Phase, t.StageVisit, versions})
	return digest(string(encoded))
}
func readyReviewed(st *model.State, t model.Ticket) bool {
	return !readyReviewRequired(st, t) || t.ReadyApproval != nil && t.ReadyApproval.RecordVersion == t.Version && t.ReadyApproval.Basis == readyReviewBasis(st, t)
}
func requireReadyReview(st *model.State, t model.Ticket) error {
	if !readyReviewed(st, t) {
		return failure(409, "ready_review_required", "Final human review is required before implementation")
	}
	return nil
}

func reconcileReadyAttention(st *model.State, a model.Account) bool {
	changed := false
	for _, t := range st.Tickets {
		if readyReviewRequired(st, t) && !readyReviewed(st, t) && t.Attention == "" {
			t.Attention = "Final human review required"
			saveTicket(st, &t, a, "readyReviewRequired")
			changed = true
		}
	}
	return changed
}
