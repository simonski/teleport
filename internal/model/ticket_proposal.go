package model

import "time"

type TicketProposalItem struct {
	Key         string `json:"key"`
	ParentKey   string `json:"parentKey,omitempty"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Criteria    string `json:"criteria,omitempty"`
}
type ProposalFeedback struct {
	Actor   string    `json:"actor"`
	Text    string    `json:"text"`
	ItemKey string    `json:"itemKey,omitempty"`
	At      time.Time `json:"at"`
}
type TicketProposal struct {
	Kind          string               `json:"kind"`
	Ready         bool                 `json:"ready"`
	SessionID     string               `json:"sessionId,omitempty"`
	UsageID       string               `json:"usageId,omitempty"`
	UsageVersion  int64                `json:"usageVersion,omitempty"`
	AgentID       string               `json:"agentId,omitempty"`
	AgentVersion  int64                `json:"agentVersion,omitempty"`
	ID            string               `json:"id"`
	Version       int64                `json:"version"`
	TicketID      int64                `json:"ticketId"`
	TicketVersion int64                `json:"ticketVersion"`
	ProjectID     int64                `json:"projectId"`
	Title         string               `json:"title"`
	Actor         string               `json:"actor"`
	At            time.Time            `json:"at"`
	PreviousID    string               `json:"previousId,omitempty"`
	Items         []TicketProposalItem `json:"items"`
	Feedback      []ProposalFeedback   `json:"feedback"`
	AcceptedBy    string               `json:"acceptedBy,omitempty"`
	AcceptedAt    *time.Time           `json:"acceptedAt,omitempty"`
	DiscardedBy   string               `json:"discardedBy,omitempty"`
	DiscardedAt   *time.Time           `json:"discardedAt,omitempty"`
	CreatedIDs    []int64              `json:"createdIds,omitempty"`
}
