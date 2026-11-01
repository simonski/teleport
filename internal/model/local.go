package model

import "time"

type LocalSession struct {
	Managed           *ManagedLaunch `json:"managed,omitempty"`
	CredentialHash    string         `json:"credentialHash,omitempty"`
	CredentialExpires time.Time      `json:"credentialExpires,omitempty"`
	StopRequested     bool           `json:"stopRequested"`
	Number            int64          `json:"number"`
	ID                string         `json:"id"`
	Creator           string         `json:"creator"`
	ProjectID         int64          `json:"projectId"`
	TicketID          int64          `json:"ticketId"`
	Harness           string         `json:"harness"`
	LaunchID          string         `json:"launchId"`
	Generation        string         `json:"generation"`
	Fingerprint       string         `json:"fingerprint"`
	State             string         `json:"state"`
	ErrorCode         string         `json:"errorCode,omitempty"`
	Attention         string         `json:"attention,omitempty"`
	Worktree          Worktree       `json:"worktree"`
	BindingVersion    int64          `json:"bindingVersion"`
	InputSequence     int64          `json:"inputSequence"`
	CreatedAt         time.Time      `json:"createdAt"`
	ExitedAt          *time.Time     `json:"exitedAt,omitempty"`
	ExitCode          *int           `json:"exitCode,omitempty"`
}

// ManagedLaunch freezes the exact phase authority and prompt at reservation.
type ManagedLaunch struct {
	RequestID      string         `json:"requestId"`
	RequestVersion int64          `json:"requestVersion"`
	Workflow       string         `json:"workflow"`
	Phase          string         `json:"phase"`
	Role           string         `json:"role"`
	Visit          int64          `json:"visit"`
	Basis          string         `json:"basis"`
	Attempt        string         `json:"attempt"`
	Continues      bool           `json:"continues"`
	Prompt         PromptSnapshot `json:"prompt"`
	Candidate      string         `json:"candidate,omitempty"`
	Result         *ManagedResult `json:"result,omitempty"`
	TurnID         string         `json:"turnId,omitempty"`
	Error          string         `json:"error,omitempty"`
	Applied        bool           `json:"applied"`
}
type PromptContribution struct {
	Source  string `json:"source"`
	Version string `json:"version"`
	Text    string `json:"text"`
}
type PromptSnapshot struct {
	Text          string               `json:"text"`
	Contributions []PromptContribution `json:"contributions"`
}
type ManagedResult struct {
	Outcome  string `json:"outcome"`
	Proposal string `json:"proposal,omitempty"`
	Criteria string `json:"criteria,omitempty"`
	Findings string `json:"findings,omitempty"`
	Question string `json:"question,omitempty"`
}
type LocalInput struct {
	SessionID   string    `json:"sessionId"`
	RequestID   string    `json:"requestId"`
	Actor       string    `json:"actor"`
	Sequence    int64     `json:"sequence"`
	Fingerprint string    `json:"fingerprint"`
	State       string    `json:"state"`
	Written     int       `json:"written"`
	At          time.Time `json:"at"`
}
