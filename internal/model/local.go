package model

import "time"

type LocalSession struct {
	CredentialHash    string     `json:"credentialHash,omitempty"`
	CredentialExpires time.Time  `json:"credentialExpires,omitempty"`
	StopRequested     bool       `json:"stopRequested"`
	Number            int64      `json:"number"`
	ID                string     `json:"id"`
	Creator           string     `json:"creator"`
	ProjectID         int64      `json:"projectId"`
	TicketID          int64      `json:"ticketId"`
	Harness           string     `json:"harness"`
	LaunchID          string     `json:"launchId"`
	Generation        string     `json:"generation"`
	Fingerprint       string     `json:"fingerprint"`
	State             string     `json:"state"`
	ErrorCode         string     `json:"errorCode,omitempty"`
	Attention         string     `json:"attention,omitempty"`
	Worktree          Worktree   `json:"worktree"`
	BindingVersion    int64      `json:"bindingVersion"`
	InputSequence     int64      `json:"inputSequence"`
	CreatedAt         time.Time  `json:"createdAt"`
	ExitedAt          *time.Time `json:"exitedAt,omitempty"`
	ExitCode          *int       `json:"exitCode,omitempty"`
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
