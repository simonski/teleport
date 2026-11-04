package model

import "time"

type LocalSession struct {
	ProposalRun        *ProposalRun          `json:"proposalRun,omitempty"`
	Attribution        *ExecutionAttribution `json:"attribution,omitempty"`
	Spellcheck         *SpellcheckRun        `json:"spellcheck,omitempty"`
	Persona            *AgentDefinition      `json:"persona,omitempty"`
	ContextTicketID    int64                 `json:"contextTicketId,omitempty"`
	ContextTicket      string                `json:"contextTicket,omitempty"`
	ProcessID          int                   `json:"processId,omitempty"`
	ReferenceRemovedBy string                `json:"referenceRemovedBy,omitempty"`
	ReferenceRemovedAt *time.Time            `json:"referenceRemovedAt,omitempty"`
	CapacityReleasedAt *time.Time            `json:"capacityReleasedAt,omitempty"`
	CapacityReleasedBy string                `json:"capacityReleasedBy,omitempty"`
	Managed            *ManagedLaunch        `json:"managed,omitempty"`
	CredentialHash     string                `json:"credentialHash,omitempty"`
	ReadCredentialHash string                `json:"readCredentialHash,omitempty"`
	CredentialExpires  time.Time             `json:"credentialExpires,omitempty"`
	StopRequested      bool                  `json:"stopRequested"`
	Number             int64                 `json:"number"`
	ID                 string                `json:"id"`
	Creator            string                `json:"creator"`
	ProjectID          int64                 `json:"projectId"`
	TicketID           int64                 `json:"ticketId"`
	Harness            string                `json:"harness"`
	LaunchID           string                `json:"launchId"`
	Generation         string                `json:"generation"`
	Fingerprint        string                `json:"fingerprint"`
	State              string                `json:"state"`
	ErrorCode          string                `json:"errorCode,omitempty"`
	Attention          string                `json:"attention,omitempty"`
	Worktree           Worktree              `json:"worktree"`
	BindingVersion     int64                 `json:"bindingVersion"`
	InputSequence      int64                 `json:"inputSequence"`
	CreatedAt          time.Time             `json:"createdAt"`
	ExitedAt           *time.Time            `json:"exitedAt,omitempty"`
	ExitCode           *int                  `json:"exitCode,omitempty"`
}

type ProposalRun struct {
	TimedOut        bool             `json:"timedOut,omitempty"`
	Usage           *TokenEvidence   `json:"usage,omitempty"`
	Valuations      []UsageValuation `json:"valuations,omitempty"`
	TicketID        int64            `json:"ticketId"`
	TicketVersion   int64            `json:"ticketVersion"`
	UsageID         string           `json:"usageId"`
	UsageVersion    int64            `json:"usageVersion"`
	PreviousID      string           `json:"previousId,omitempty"`
	PreviousVersion int64            `json:"previousVersion,omitempty"`
	ProposalID      string           `json:"proposalId,omitempty"`
}

// ExecutionAttribution records admission-time identity and hierarchy, without
// inferring delegation from an account that merely hosts a supervisor.
type ExecutionAttribution struct {
	ProjectID       int64  `json:"projectId"`
	ProjectVersion  int64  `json:"projectVersion"`
	CallerKind      string `json:"callerKind"`
	CallerID        string `json:"callerId,omitempty"`
	CallerCallID    string `json:"callerCallId,omitempty"`
	InitiatorID     string `json:"initiatorId,omitempty"`
	ParentSessionID string `json:"parentSessionId,omitempty"`
	AgentID         string `json:"agentId,omitempty"`
	AgentName       string `json:"agentName,omitempty"`
	AgentVersion    int64  `json:"agentVersion,omitempty"`
	UsageID         string `json:"usageId,omitempty"`
	UsageVersion    int64  `json:"usageVersion,omitempty"`
	TicketID        int64  `json:"ticketId,omitempty"`
	TicketVersion   int64  `json:"ticketVersion,omitempty"`
	EpicID          int64  `json:"epicId,omitempty"`
	EpicVersion     int64  `json:"epicVersion,omitempty"`
	GoalID          int64  `json:"goalId,omitempty"`
	GoalVersion     int64  `json:"goalVersion,omitempty"`
	HierarchyStatus string `json:"hierarchyStatus"`
}

type SpellcheckRun struct {
	Valuations    []UsageValuation `json:"valuations,omitempty"`
	Usage         *TokenEvidence   `json:"usage,omitempty"`
	UsageID       string           `json:"usageId"`
	UsageVersion  int64            `json:"usageVersion"`
	TicketID      int64            `json:"ticketId"`
	TicketVersion int64            `json:"ticketVersion"`
	RequestID     string           `json:"requestId"`
	Payload       string           `json:"payload"`
	State         string           `json:"state"`
	Error         string           `json:"error,omitempty"`
}

type UsagePrice struct {
	Harness      string    `json:"harness"`
	Model        string    `json:"model"`
	Version      int64     `json:"version"`
	Effective    string    `json:"effective"`
	Source       string    `json:"source"`
	InputMicros  int64     `json:"inputMicros"`
	CachedMicros int64     `json:"cachedMicros"`
	OutputMicros int64     `json:"outputMicros"`
	PublishedAt  time.Time `json:"publishedAt"`
	PublishedBy  string    `json:"publishedBy"`
}
type PriceRecalculation struct {
	Version int64 `json:"version"`
	Cursor  int64 `json:"cursor"`
	Done    bool  `json:"done"`
}
type UsageValuation struct {
	PriceVersion   int64     `json:"priceVersion"`
	EstimateMicros int64     `json:"estimateMicros"`
	PreviousMicros *int64    `json:"previousMicros,omitempty"`
	InputTokens    int64     `json:"inputTokens"`
	CachedTokens   int64     `json:"cachedTokens"`
	OutputTokens   int64     `json:"outputTokens"`
	Calculation    string    `json:"calculation"`
	At             time.Time `json:"at"`
	Effective      string    `json:"effective"`
	Source         string    `json:"source"`
}

// TokenEvidence holds provider-reported counters, not inferred billing.
type TokenEvidence struct {
	Input             int64  `json:"input"`
	Cached            *int64 `json:"cached,omitempty"`
	Output            int64  `json:"output"`
	EstimateUSDMicros *int64 `json:"estimateUsdMicros,omitempty"`
	Source            string `json:"source"`
}

type SpellcheckRecord struct {
	UsageID      string    `json:"usageId"`
	UsageVersion int64     `json:"usageVersion"`
	AgentID      string    `json:"agentId"`
	AgentVersion int64     `json:"agentVersion"`
	Actor        string    `json:"actor"`
	Basis        string    `json:"basis"`
	At           time.Time `json:"at"`
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
