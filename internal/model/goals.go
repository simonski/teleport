package model

import "time"

type GoalObjective struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Unit      string   `json:"unit"`
	Direction string   `json:"direction"`
	Target    *float64 `json:"target,omitempty"`
}
type GoalPlan struct {
	Objectives []GoalObjective `json:"objectives"`
	Cadence    string          `json:"cadence"`
	NextReview string          `json:"nextReview,omitempty"`
}
type GoalObservation struct {
	ObjectiveID string   `json:"objectiveId"`
	Value       *float64 `json:"value,omitempty"`
	Note        string   `json:"note"`
	Source      string   `json:"source"`
	AsOf        string   `json:"asOf"`
}
type GoalReview struct {
	ID                int64             `json:"id"`
	GoalID            int64             `json:"goalId"`
	GoalVersion       int64             `json:"goalVersion"`
	ProjectID         int64             `json:"projectId"`
	GoalTitle         string            `json:"goalTitle"`
	GoalCriteria      string            `json:"goalCriteria"`
	Objectives        []GoalObjective   `json:"objectives"`
	ChildVersions     map[int64]int64   `json:"childVersions"`
	DeliveryStatus    string            `json:"deliveryStatus"`
	DeliveryProgress  int               `json:"deliveryProgress"`
	Assessment        string            `json:"assessment"`
	Observations      []GoalObservation `json:"observations"`
	MissingObjectives []string          `json:"missingObjectives"`
	MissingContext    []string          `json:"missingContext"`
	Note              string            `json:"note"`
	Cadence           string            `json:"cadence"`
	ReviewDate        string            `json:"reviewDate"`
	NextReview        string            `json:"nextReview"`
	Actor             string            `json:"actor"`
	At                time.Time         `json:"at"`
	TransactionID     int64             `json:"transactionId"`
}
