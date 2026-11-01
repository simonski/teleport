package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestGoalReviewFreezesDeliveryAndMissingOutcomeEvidence(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Outcomes", RequestID: "p"}).(model.Project)
	human := false
	goal := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Kind: "goal", Title: "Reduce boarding delay", Agent: &human, RequestID: "g"}).(model.Ticket)
	objectives := []model.GoalObjective{{Title: "Median delay", Unit: "minutes", Direction: "atMost", Target: ptr(5.0)}}
	goal = call(t, svc, token, "configureGoalReview", app.ConfigureGoalInput{ID: goal.ID, ExpectedVersion: goal.Version, Objectives: &objectives, Cadence: ptr("monthly")}).(model.Ticket)
	epic := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, ParentID: goal.ID, Kind: "epic", Title: "Boarding work", Agent: &human, RequestID: "e"}).(model.Ticket)
	task := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, ParentID: epic.ID, Title: "Deliver checklist", Agent: &human, RequestID: "t"}).(model.Ticket)
	task = call(t, svc, token, "transitionTicket", app.TransitionInput{ID: task.ID, ExpectedVersion: task.Version, Target: "Done", Confirm: true}).(model.Ticket)
	goal = call(t, svc, token, "getTicket", app.IDInput{ID: goal.ID}).(model.Ticket)
	if goal.Status != "Done" {
		t.Fatal("delivery rollup did not complete")
	}
	changedObjectives := append([]model.GoalObjective{}, goal.GoalPlan.Objectives...)
	changedObjectives[0].Target = ptr(4.0)
	locked, _ := json.Marshal(app.ConfigureGoalInput{ID: goal.ID, ExpectedVersion: goal.Version, Objectives: &changedObjectives})
	if _, err := svc.Call(context.Background(), token, "configureGoalReview", locked); err == nil {
		t.Fatal("Done goal objectives were rewritten")
	}
	input := app.GoalReviewInput{ID: goal.ID, ExpectedVersion: goal.Version, RequestID: "review", ReviewDate: "2026-01-31", Assessment: "unknown", Note: "No production observations yet"}
	review := call(t, svc, token, "reviewGoal", input).(model.GoalReview)
	repeated := call(t, svc, token, "reviewGoal", input).(model.GoalReview)
	if review.ID != repeated.ID || review.NextReview != "2026-02-28" || review.DeliveryStatus != "Done" || review.Assessment != "unknown" || len(review.MissingObjectives) != 1 {
		t.Fatal("review inferred an outcome or repeated a decision")
	}
	call(t, svc, token, "reopenTicket", app.VersionInput{ID: task.ID, ExpectedVersion: task.Version})
	current := call(t, svc, token, "getGoalSummary", app.IDInput{ID: goal.ID}).(app.GoalSummary)
	if current.Goal.Status == "Done" || current.LatestReview == nil || current.LatestReview.DeliveryStatus != "Done" || current.LatestReview.GoalVersion != review.GoalVersion {
		t.Fatal("later delivery changed historical assessment")
	}
	reviews := call(t, svc, token, "listGoalReviews", app.IDInput{ID: goal.ID}).(app.Page[model.GoalReview])
	if len(reviews.Items) != 1 || reviews.Items[0].Objectives[0].Target == nil || *reviews.Items[0].Objectives[0].Target != 5 {
		t.Fatal("objective basis was not retained")
	}
	raw, _ := json.Marshal(app.ConfigureGoalInput{ID: goal.ID, ExpectedVersion: current.Goal.Version - 1, Objectives: &objectives})
	if _, err := svc.Call(context.Background(), token, "configureGoalReview", raw); err == nil {
		t.Fatal("stale goal write succeeded")
	}
	goal = call(t, svc, token, "configureGoalReview", app.ConfigureGoalInput{ID: goal.ID, ExpectedVersion: current.Goal.Version, Cadence: ptr("quarterly")}).(model.Ticket)
	quarter := call(t, svc, token, "reviewGoal", app.GoalReviewInput{ID: goal.ID, ExpectedVersion: goal.Version, RequestID: "quarter", ReviewDate: "2026-11-30", Assessment: "unknown"}).(model.GoalReview)
	if quarter.NextReview != "2027-02-28" {
		t.Fatal("quarterly arithmetic did not clamp February")
	}
}
