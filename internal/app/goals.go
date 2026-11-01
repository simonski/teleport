package app

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"time"

	"teleport/internal/model"
)

type ConfigureGoalInput struct {
	ID              int64                  `json:"id"`
	ExpectedVersion int64                  `json:"expectedVersion"`
	Objectives      *[]model.GoalObjective `json:"objectives,omitempty"`
	Cadence         *string                `json:"cadence,omitempty"`
}
type GoalReviewInput struct {
	ID              int64                   `json:"id"`
	ExpectedVersion int64                   `json:"expectedVersion"`
	RequestID       string                  `json:"requestId"`
	ReviewDate      string                  `json:"reviewDate"`
	Assessment      string                  `json:"assessment"`
	Note            string                  `json:"note"`
	Observations    []model.GoalObservation `json:"observations"`
}
type GoalSummary struct {
	Goal           model.Ticket      `json:"goal"`
	DirectChildren int               `json:"directChildren"`
	DoneChildren   int               `json:"doneChildren"`
	LatestReview   *model.GoalReview `json:"latestReview,omitempty"`
}

func goalFor(st *model.State, a model.Account, id int64, minimum string) (model.Ticket, error) {
	goal, err := ticketFor(st, a, id, minimum)
	if err != nil {
		return goal, err
	}
	if goal.Kind != "goal" {
		return model.Ticket{}, failure(422, "goal_required", "Select an existing goal ticket")
	}
	return goal, nil
}
func goalPlan(t model.Ticket) model.GoalPlan {
	if t.GoalPlan == nil {
		return model.GoalPlan{Cadence: "monthly", Objectives: []model.GoalObjective{}}
	}
	return *t.GoalPlan
}
func reviewCalendarDate(value string) (time.Time, error) {
	date, err := time.Parse("2006-01-02", value)
	if err != nil || date.Year() < 1 {
		return time.Time{}, failure(422, "invalid_date", "Use a YYYY-MM-DD calendar date")
	}
	return date, nil
}
func nextReviewDate(value, cadence string) (string, error) {
	date, err := reviewCalendarDate(value)
	if err != nil {
		return "", err
	}
	months := 1
	if cadence == "quarterly" {
		months = 3
	}
	first := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, months, 0)
	last := first.AddDate(0, 1, -1).Day()
	day := date.Day()
	if day > last {
		day = last
	}
	if first.Year() > 9999 {
		return "", failure(422, "invalid_date", "Next review must fit a four-digit year")
	}
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), nil
}
func goalOperations(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	if st.GoalReviews == nil {
		st.GoalReviews = map[int64][]model.GoalReview{}
	}
	switch operation {
	case "getGoalView", "getEpicView":
		var in ListInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		in.Kind = "goal"
		if operation == "getEpicView" {
			in.Kind = "epic"
		}
		body, _ := json.Marshal(in)
		return projectOperations(st, a, "listTickets", body)
	case "getGoalSummary", "listGoalReviews":
		var in IDInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		goal, err := goalFor(st, a, in.ID, "Viewer")
		if err != nil {
			return nil, false, err
		}
		if operation == "getGoalSummary" {
			summary := GoalSummary{Goal: goal}
			for _, child := range st.Tickets {
				if child.ParentID == goal.ID && !child.Deleted {
					summary.DirectChildren++
					if child.Status == "Done" {
						summary.DoneChildren++
					}
				}
			}
			reviews := st.GoalReviews[goal.ID]
			if len(reviews) > 0 {
				review := reviews[len(reviews)-1]
				summary.LatestReview = &review
			}
			return summary, false, nil
		}
		after, limit, err := pageBounds(in.Cursor, in.Limit)
		if err != nil {
			return nil, false, err
		}
		result := Page[model.GoalReview]{Items: []model.GoalReview{}}
		for _, review := range st.GoalReviews[goal.ID] {
			if review.ID <= after {
				continue
			}
			if len(result.Items) == limit {
				result.NextCursor = cursor(result.Items[len(result.Items)-1].ID)
				break
			}
			result.Items = append(result.Items, review)
		}
		return result, false, nil
	case "configureGoalReview":
		var in ConfigureGoalInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		goal, err := goalFor(st, a, in.ID, "Contributor")
		if err != nil {
			return nil, false, err
		}
		if err = expected(goal, in.ExpectedVersion); err != nil {
			return nil, false, err
		}
		if goal.Protected || busy(goal) {
			return nil, false, failure(409, "goal_locked", "Select an ordinary stopped goal for configuration")
		}
		plan := goalPlan(goal)
		previous := plan
		if in.Cadence != nil {
			if !contains([]string{"monthly", "quarterly"}, *in.Cadence) {
				return nil, false, failure(422, "invalid_cadence", "Choose monthly or quarterly reviews")
			}
			plan.Cadence = *in.Cadence
		}
		if in.Objectives != nil {
			if len(*in.Objectives) > 20 {
				return nil, false, failure(422, "too_many_objectives", "Use at most 20 goal-owned objectives")
			}
			existing := map[string]bool{}
			for _, objective := range plan.Objectives {
				existing[objective.ID] = true
			}
			seen := map[string]bool{}
			items := append([]model.GoalObjective{}, (*in.Objectives)...)
			for n, item := range items {
				item.Title = strings.TrimSpace(item.Title)
				if item.Title == "" || len(item.Title) > 300 || len(item.Unit) > 80 || !contains([]string{"atLeast", "atMost", "qualitative"}, item.Direction) {
					return nil, false, failure(422, "invalid_objective", "Use a title, bounded unit and numeric/qualitative direction")
				}
				if item.Direction == "qualitative" && item.Target != nil || item.Direction != "qualitative" && item.Target == nil || item.Target != nil && (math.IsNaN(*item.Target) || math.IsInf(*item.Target, 0) || math.Abs(*item.Target) > 1e15) {
					return nil, false, failure(422, "invalid_target", "Numeric measures need finite targets; qualitative measures have no numeric target")
				}
				if item.ID == "" {
					item.ID = "objective:" + randomID()[:24]
				} else if !existing[item.ID] {
					return nil, false, failure(422, "foreign_objective", "Objective IDs must belong to this goal")
				}
				if seen[item.ID] {
					return nil, false, failure(422, "duplicate_objective", "Use each objective once")
				}
				seen[item.ID] = true
				items[n] = item
			}
			if !reflect.DeepEqual(plan.Objectives, items) && goal.Status != "Todo" {
				return nil, false, failure(409, "content_locked", "Return to semantic Todo before changing original objective criteria")
			}
			plan.Objectives = items
		}
		if reflect.DeepEqual(previous, plan) {
			return goal, false, nil
		}
		goal.GoalPlan = &plan
		saveTicket(st, &goal, a, operation)
		return goal, true, nil
	case "reviewGoal":
		var in GoalReviewInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		goal, err := goalFor(st, a, in.ID, "Contributor")
		if err != nil {
			return nil, false, err
		}
		key, fingerprint, previous, err := receipt(st, a, operation, in.RequestID, in)
		if err != nil {
			return nil, false, err
		}
		if previous != nil {
			var review model.GoalReview
			json.Unmarshal(previous, &review)
			return review, false, nil
		}
		if err = expected(goal, in.ExpectedVersion); err != nil {
			return nil, false, err
		}
		if !contains([]string{"achieved", "notAchieved", "unknown"}, in.Assessment) || len(in.Note) > 10000 || len(in.Observations) > 20 {
			return nil, false, failure(422, "invalid_review", "Use a human assessment, note within 10,000 bytes and at most 20 observations")
		}
		plan := goalPlan(goal)
		next, err := nextReviewDate(in.ReviewDate, plan.Cadence)
		if err != nil {
			return nil, false, err
		}
		objectives := map[string]model.GoalObjective{}
		for _, objective := range plan.Objectives {
			objectives[objective.ID] = objective
		}
		seen := map[string]bool{}
		missingContext := []string{}
		for _, observation := range in.Observations {
			objective, ok := objectives[observation.ObjectiveID]
			if !ok || seen[observation.ObjectiveID] || len(observation.Note) > 5000 || len(observation.Source) > 1000 {
				return nil, false, failure(422, "invalid_observation", "Use current unique goal objectives and bounded evidence")
			}
			seen[observation.ObjectiveID] = true
			if observation.Value != nil && (objective.Direction == "qualitative" || math.IsNaN(*observation.Value) || math.IsInf(*observation.Value, 0) || math.Abs(*observation.Value) > 1e15) {
				return nil, false, failure(422, "invalid_value", "Use finite numeric observations only for numeric measures")
			}
			if observation.AsOf != "" {
				if _, err = reviewCalendarDate(observation.AsOf); err != nil {
					return nil, false, err
				}
			} else {
				missingContext = append(missingContext, observation.ObjectiveID+":asOf")
			}
			if strings.TrimSpace(observation.Source) == "" {
				missingContext = append(missingContext, observation.ObjectiveID+":source")
			}
		}
		missing := []string{}
		for _, objective := range plan.Objectives {
			hasEvidence := false
			for _, observation := range in.Observations {
				if observation.ObjectiveID == objective.ID && (observation.Value != nil || strings.TrimSpace(observation.Note) != "") {
					hasEvidence = true
				}
			}
			if !hasEvidence {
				missing = append(missing, objective.ID)
			}
		}
		children := map[int64]int64{}
		for _, child := range st.Tickets {
			if child.ParentID == goal.ID && !child.Deleted {
				children[child.ID] = child.Version
			}
		}
		review := model.GoalReview{
			ID: int64(len(st.GoalReviews[goal.ID]) + 1), GoalID: goal.ID, GoalVersion: goal.Version, ProjectID: goal.ProjectID,
			GoalTitle: goal.Title, GoalCriteria: goal.Criteria, Objectives: append([]model.GoalObjective{}, plan.Objectives...),
			ChildVersions: children, DeliveryStatus: goal.Status, DeliveryProgress: goal.Progress, Assessment: in.Assessment,
			Observations: append([]model.GoalObservation{}, in.Observations...), MissingObjectives: missing, MissingContext: missingContext,
			Note: in.Note, Cadence: plan.Cadence, ReviewDate: in.ReviewDate, NextReview: next, Actor: a.ID, At: time.Now().UTC(), TransactionID: st.Revision + 1,
		}
		st.GoalReviews[goal.ID] = append(st.GoalReviews[goal.ID], review)
		plan.NextReview = next
		goal.GoalPlan = &plan
		saveTicket(st, &goal, a, operation)
		remember(st, key, fingerprint, review)
		return review, true, nil
	}
	return nil, false, failure(404, "unsupported_operation", "Operation is not available in this build")
}
