package server

import (
	"teleport/internal/app"
	"teleport/internal/model"
)

func init() {
	entries := []struct {
		name, action, summary string
		input, output         any
	}{
		{"configureGoalReview", "ACT-0601", "Configure goal-owned criteria and review cadence", app.ConfigureGoalInput{}, model.Ticket{}},
		{"getGoalSummary", "ACT-0602", "Read delivery separately from observed goal outcome", app.IDInput{}, app.GoalSummary{}},
		{"reviewGoal", "ACT-0603", "Append an immutable human goal assessment", app.GoalReviewInput{}, model.GoalReview{}},
		{"listGoalReviews", "ACT-0604", "Read historical goal review evidence", app.IDInput{}, app.Page[model.GoalReview]{}},
		{"getGoalView", "ACT-0611", "List goals using shared ticket identities", app.ListInput{}, app.Page[model.Ticket]{}},
		{"getEpicView", "ACT-0611", "List epics using shared ticket identities", app.ListInput{}, app.Page[model.Ticket]{}},
	}
	for _, e := range entries {
		Operations = append(Operations, Operation{e.name, e.action, e.summary})
		contracts[e.name] = contract{e.input, e.output}
	}
}
