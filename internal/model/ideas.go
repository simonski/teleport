package model

import "time"

type Idea struct {
	ID            string    `json:"id"`
	Version       int64     `json:"version"`
	Placement     string    `json:"placement"`
	ProjectID     int64     `json:"projectId"`
	Creator       string    `json:"creator"`
	Title         string    `json:"title"`
	Text          string    `json:"text"`
	Archived      bool      `json:"archived"`
	Deleted       bool      `json:"deleted"`
	CurationState string    `json:"curationState,omitempty"`
	AcceptedCount int       `json:"acceptedCount"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}
type IdeaCreate struct {
	ProjectID   int64  `json:"projectId"`
	ParentID    int64  `json:"parentId,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Criteria    string `json:"criteria,omitempty"`
	Agent       *bool  `json:"agent,omitempty"`
}
type IdeaTarget struct {
	ExistingID      int64       `json:"existingId,omitempty"`
	ExistingVersion int64       `json:"existingVersion,omitempty"`
	Create          *IdeaCreate `json:"create,omitempty"`
	Unavailable     bool        `json:"unavailable,omitempty"`
}
type IdeaProposal struct {
	ID          string       `json:"id"`
	Version     int64        `json:"version"`
	IdeaID      string       `json:"ideaId"`
	IdeaVersion int64        `json:"ideaVersion"`
	Actor       string       `json:"actor"`
	At          time.Time    `json:"at"`
	Targets     []IdeaTarget `json:"targets"`
}
type IdeaAcceptance struct {
	ProposalID      string    `json:"proposalId"`
	ProposalVersion int64     `json:"proposalVersion"`
	IdeaID          string    `json:"ideaId"`
	IdeaVersion     int64     `json:"ideaVersion"`
	Actor           string    `json:"actor"`
	At              time.Time `json:"at"`
	TicketIDs       []int64   `json:"ticketIds"`
}
type IdeaLink struct {
	IdeaID          string    `json:"ideaId"`
	IdeaVersion     int64     `json:"ideaVersion"`
	ProposalID      string    `json:"proposalId"`
	ProposalVersion int64     `json:"proposalVersion"`
	TicketID        int64     `json:"ticketId"`
	TicketVersion   int64     `json:"ticketVersion"`
	Created         bool      `json:"created"`
	Actor           string    `json:"actor"`
	At              time.Time `json:"at"`
}
type IdeaRevision struct {
	Sequence      int64     `json:"sequence"`
	TransactionID int64     `json:"transactionId"`
	Actor         string    `json:"actor"`
	Action        string    `json:"action"`
	At            time.Time `json:"at"`
	Snapshot      Idea      `json:"snapshot"`
	ProposalID    string    `json:"proposalId,omitempty"`
}
