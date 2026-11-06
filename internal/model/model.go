package model

import "time"

const SchemaVersion = 1

type Account struct {
	ServiceSessionReadOnly bool   `json:"-"`
	ServiceRunID           string `json:"-"`
	ServiceCallID          string `json:"-"`
	ServiceSessionID       string `json:"-"`
	ServiceProjectID       int64  `json:"-"`
	ID                     string `json:"id"`
	Username               string `json:"username"`
	PasswordHash           string `json:"passwordHash,omitempty"`
	Administrator          bool   `json:"administrator"`
	Suspended              bool   `json:"suspended"`
}
type Session struct {
	AccountID string    `json:"accountId"`
	Expires   time.Time `json:"expires"`
}
type Settings struct {
	Version       int64  `json:"version"`
	ExecutionMode string `json:"executionMode"`
	Agents        bool   `json:"agents"`
	Registrations bool   `json:"registrations"`
	Handbrake     bool   `json:"handbrake"`
}
type Phase struct {
	RequireReadyReview bool   `json:"requireReadyReview,omitempty"`
	ID                 string `json:"id"`
	Title              string `json:"title"`
	Role               string `json:"role"`
	Agentic            bool   `json:"agentic"`
	Instructions       string `json:"instructions"`
	Description        string `json:"description,omitempty"`
	Workers            *int   `json:"workers,omitempty"`
}
type Rule struct {
	ID         string            `json:"id"`
	Source     string            `json:"source"`
	Outcome    string            `json:"outcome"`
	Target     string            `json:"target,omitempty"`
	Action     string            `json:"action,omitempty"`
	Priority   int               `json:"priority"`
	Conditions map[string]string `json:"conditions,omitempty"`
}
type Workflow struct {
	ProjectID     int64            `json:"projectId"`
	DraftRevision int64            `json:"draftRevision"`
	ID            string           `json:"id"`
	Version       int64            `json:"version"`
	Title         string           `json:"title"`
	Family        string           `json:"family"`
	Published     bool             `json:"published"`
	Phases        []Phase          `json:"phases"`
	Rules         []Rule           `json:"rules"`
	Layout        map[string]Point `json:"layout,omitempty"`
}
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type State struct {
	TicketProposals     map[string]TicketProposal     `json:"ticketProposals,omitempty"`
	UsagePrices         map[string][]UsagePrice       `json:"usagePrices,omitempty"`
	PriceRecalculations map[string]PriceRecalculation `json:"priceRecalculations,omitempty"`
	AgentUsages         map[string]AgentUsage         `json:"agentUsages,omitempty"`
	AgentDefinitions    map[string]AgentDefinition    `json:"agentDefinitions,omitempty"`
	LocationWrites      int64                         `json:"locationWrites,omitempty"`
	LastLocations       map[string]RecentContext      `json:"lastLocations,omitempty"`
	NextScriptID        int64                         `json:"nextScriptId"`
	ScriptRuns          map[string]ScriptRun          `json:"scriptRuns"`
	NextLocalSessionID  int64                         `json:"nextLocalSessionId"`
	LocalSessions       map[string]LocalSession       `json:"localSessions"`
	LocalInputs         map[string]LocalInput         `json:"localInputs"`
	WelcomeSetups       map[string]WelcomeSetup       `json:"welcomeSetups"`
	GoalReviews         map[int64][]GoalReview        `json:"goalReviews"`
	GraphLayouts        map[string]GraphLayout        `json:"graphLayouts"`
	BacklogOrders       map[string]BacklogOrder       `json:"backlogOrders,omitempty"`
	Preferences         map[string]Preferences        `json:"preferences"`
	NextIdeaID          int64                         `json:"nextIdeaId"`
	Ideas               map[string]Idea               `json:"ideas"`
	IdeaHistory         map[string][]IdeaRevision     `json:"ideaHistory"`
	IdeaProposals       map[string]IdeaProposal       `json:"ideaProposals"`
	IdeaAcceptances     map[string]IdeaAcceptance     `json:"ideaAcceptances"`
	IdeaLinks           []IdeaLink                    `json:"ideaLinks"`
	Schema              int                           `json:"schema"`
	InstallationID      string                        `json:"installationId"`
	Revision            int64                         `json:"revision"`
	Accounts            map[string]Account            `json:"accounts"`
	Sessions            map[string]Session            `json:"sessions"`
	Settings            Settings                      `json:"settings"`
	Workflows           map[string]Workflow           `json:"workflows"`
	NextTicketID        int64                         `json:"nextTicketId"`
	NextProjectID       int64                         `json:"nextProjectId"`
	Projects            map[int64]Project             `json:"projects"`
	Tickets             map[int64]Ticket              `json:"tickets"`
	History             map[int64][]Revision          `json:"history"`
	Receipts            map[string]Receipt            `json:"receipts"`
}

type Project struct {
	PersonalOwner      string             `json:"personalOwner,omitempty"`
	MaxAgents          *int               `json:"maxAgents,omitempty"`
	AllowedHarnesses   *[]string          `json:"allowedHarnesses,omitempty"`
	Simulation         bool               `json:"simulation,omitempty"`
	DemoSeed           string             `json:"demoSeed,omitempty"`
	Repository         *RepositoryBinding `json:"repository,omitempty"`
	RefineWorkers      int                `json:"refineWorkers"`
	DoingWorkers       int                `json:"doingWorkers"`
	CapacityConfigured bool               `json:"capacityConfigured"`
	LaneLocks          map[string]bool    `json:"laneLocks,omitempty"`
	ID                 int64              `json:"id"`
	Version            int64              `json:"version"`
	Key                string             `json:"key"`
	Name               string             `json:"name"`
	Description        string             `json:"description"`
	Root               string             `json:"root,omitempty"`
	Creator            string             `json:"creator"`
	Members            map[string]string  `json:"members"`
	UnassignedGoal     int64              `json:"unassignedGoal"`
	UnassignedEpic     int64              `json:"unassignedEpic"`
	HumanWorkflow      string             `json:"humanWorkflow"`
	AgentWorkflow      string             `json:"agentWorkflow"`
	Handbrake          bool               `json:"handbrake"`
}
type Agreement struct {
	RecordVersion        int64           `json:"recordVersion"`
	ProjectID            int64           `json:"projectId"`
	Workflow             string          `json:"workflow"`
	PrerequisiteVersions map[int64]int64 `json:"prerequisiteVersions,omitempty"`
	ChildVersions        map[int64]int64 `json:"childVersions,omitempty"`
	Commit               string          `json:"commit,omitempty"`
	Base                 string          `json:"base,omitempty"`
	Basis                string          `json:"basis,omitempty"`
	Kind                 string          `json:"kind"`
	Actor                string          `json:"actor"`
	At                   time.Time       `json:"at"`
	Criteria             string          `json:"criteria"`
	BasisVersion         int64           `json:"basisVersion"`
}
type Ticket struct {
	ProposalAttention  bool                        `json:"proposalAttention,omitempty"`
	Spellchecks        map[string]SpellcheckRecord `json:"spellchecks,omitempty"`
	ReadyApproval      *ReadyApproval              `json:"readyApproval,omitempty"`
	GoalPlan           *GoalPlan                   `json:"goalPlan,omitempty"`
	Delivery           *Delivery                   `json:"delivery,omitempty"`
	RoutingHold        string                      `json:"routingHold,omitempty"`
	PhaseEnteredAt     time.Time                   `json:"phaseEnteredAt"`
	AttemptLimit       int                         `json:"attemptLimit"`
	Attempts           int                         `json:"attempts"`
	StageVisit         int64                       `json:"stageVisit"`
	ResumeAttempt      string                      `json:"resumeAttempt,omitempty"`
	Attention          string                      `json:"attention,omitempty"`
	Refinement         *Refinement                 `json:"refinement,omitempty"`
	Question           *Question                   `json:"question,omitempty"`
	AgentReview        *AgentReview                `json:"agentReview,omitempty"`
	StageEvents        []StageEvent                `json:"stageEvents,omitempty"`
	ID                 int64                       `json:"id"`
	Version            int64                       `json:"version"`
	ProjectID          int64                       `json:"projectId"`
	ParentID           int64                       `json:"parentId"`
	Kind               string                      `json:"kind"`
	Title              string                      `json:"title"`
	Description        string                      `json:"description"`
	Criteria           string                      `json:"criteria"`
	Status             string                      `json:"status"`
	Phase              string                      `json:"phase"`
	Workflow           string                      `json:"workflow"`
	Agent              bool                        `json:"agent"`
	Assignee           string                      `json:"assignee,omitempty"`
	Creator            string                      `json:"creator"`
	Priority           string                      `json:"priority"`
	Labels             []string                    `json:"labels"`
	Start              string                      `json:"start,omitempty"`
	ETA                string                      `json:"eta,omitempty"`
	Due                string                      `json:"due,omitempty"`
	Progress           int                         `json:"progress"`
	Archived           bool                        `json:"archived"`
	Deleted            bool                        `json:"deleted"`
	Protected          bool                        `json:"protected"`
	Planning           bool                        `json:"planning"`
	Active             bool                        `json:"active"`
	Claim              string                      `json:"claim,omitempty"`
	Execution          string                      `json:"execution,omitempty"`
	StopRequested      bool                        `json:"stopRequested"`
	ResumeHold         bool                        `json:"resumeHold"`
	CancellationBatch  string                      `json:"cancellationBatch,omitempty"`
	CancellationReason string                      `json:"cancellationReason,omitempty"`
	Dependencies       []int64                     `json:"dependencies"`
	Agreements         []Agreement                 `json:"agreements"`
	CreatedAt          time.Time                   `json:"createdAt"`
	UpdatedAt          time.Time                   `json:"updatedAt"`
}
type Revision struct {
	ScriptRunID   string    `json:"scriptRunId,omitempty"`
	ScriptCallID  string    `json:"scriptCallId,omitempty"`
	TransactionID int64     `json:"transactionId"`
	Version       int64     `json:"version"`
	Actor         string    `json:"actor"`
	Action        string    `json:"action"`
	At            time.Time `json:"at"`
	Snapshot      Ticket    `json:"snapshot"`
}

type ReadyApproval struct {
	RecordVersion int64     `json:"recordVersion"`
	Basis         string    `json:"basis"`
	Actor         string    `json:"actor"`
	At            time.Time `json:"at"`
}
type Refinement struct {
	AcceptedBasis string    `json:"acceptedBasis,omitempty"`
	ID            string    `json:"id"`
	Text          string    `json:"text"`
	Criteria      string    `json:"criteria"`
	Basis         string    `json:"basis"`
	AcceptedBy    string    `json:"acceptedBy,omitempty"`
	AcceptedAt    time.Time `json:"acceptedAt,omitempty"`
	Simulated     bool      `json:"simulated"`
}
type Question struct {
	OriginWaitingSince time.Time `json:"originWaitingSince"`
	ID                 string    `json:"id"`
	Text               string    `json:"text"`
	OriginPhase        string    `json:"originPhase"`
	OriginAttempt      string    `json:"originAttempt"`
	Workflow           string    `json:"workflow"`
	Basis              string    `json:"basis"`
	Answer             string    `json:"answer,omitempty"`
	Resolved           bool      `json:"resolved"`
	Obsolete           bool      `json:"obsolete"`
	AskedAt            time.Time `json:"askedAt"`
	AnsweredAt         time.Time `json:"answeredAt,omitempty"`
}
type AgentReview struct {
	Basis     string    `json:"basis,omitempty"`
	ID        string    `json:"id"`
	Outcome   string    `json:"outcome"`
	Findings  string    `json:"findings"`
	Candidate string    `json:"candidate,omitempty"`
	Simulated bool      `json:"simulated"`
	At        time.Time `json:"at"`
}
type StageEvent struct {
	AttemptID string    `json:"attemptId"`
	Phase     string    `json:"phase"`
	Outcome   string    `json:"outcome"`
	Actor     string    `json:"actor"`
	At        time.Time `json:"at"`
	Continues bool      `json:"continues"`
	Simulated bool      `json:"simulated"`
}
type RepositoryBinding struct {
	Repository  string `json:"repository"`
	StorageRoot string `json:"storageRoot"`
	Version     int64  `json:"version"`
}
type Worktree struct {
	ConsoleID string `json:"consoleId,omitempty"`
	Owner     string `json:"owner"`
	TicketID  int64  `json:"ticketId"`
	Path      string `json:"path"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
}
type GitCandidate struct {
	Base         string `json:"base"`
	Commit       string `json:"commit"`
	Diff         string `json:"diff"`
	ChecksPassed bool   `json:"checksPassed"`
	CheckOutput  string `json:"checkOutput"`
	Fingerprint  string `json:"fingerprint"`
}
type GitOperation struct {
	BasisVersion  int64  `json:"basisVersion"`
	ControllerID  string `json:"controllerId"`
	CommandExited bool   `json:"commandExited"`
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	RequestID     string `json:"requestId"`
	Fingerprint   string `json:"fingerprint"`
	Actor         string `json:"actor"`
	State         string `json:"state"`
}
type Delivery struct {
	Worktree       Worktree      `json:"worktree"`
	Candidate      *GitCandidate `json:"candidate,omitempty"`
	Operation      *GitOperation `json:"operation,omitempty"`
	BindingVersion int64         `json:"bindingVersion"`
	ScopeBasis     string        `json:"scopeBasis"`
	SourcePhase    string        `json:"sourcePhase"`
	ReportedChecks string        `json:"reportedChecks,omitempty"`
	Source         string        `json:"source"`
}
type Receipt struct {
	Fingerprint string `json:"fingerprint"`
	Result      []byte `json:"result"`
}

func SeedWorkflows() map[string]Workflow {
	h := Workflow{ID: "human", Version: 1, Title: "Human", Family: "human", Published: true}
	for _, role := range []string{"Todo", "Doing", "Done"} {
		h.Phases = append(h.Phases, Phase{ID: role, Title: role, Role: role, Instructions: "Human-powered movement between phases."})
	}
	h.Rules = []Rule{{ID: "start", Source: "Todo", Outcome: "start", Target: "Doing"}, {ID: "complete", Source: "Doing", Outcome: "complete", Target: "Done"}, {ID: "complete-draft", Source: "Todo", Outcome: "complete", Target: "Done"}, {ID: "return", Source: "Doing", Outcome: "return", Target: "Todo"}, {ID: "reopen", Source: "Done", Outcome: "reopen", Target: "Todo"}}
	a := Workflow{ID: "agent", Version: 1, Title: "Agent delivery", Family: "agent", Published: true}
	for _, role := range []string{"Todo", "Refine", "Ready", "Doing", "Review", "Approval", "Done", "Decompose", "Rebase", "Cancelled"} {
		title := role
		if role == "Doing" {
			title = "Implementation"
		}
		agentic := role == "Refine" || role == "Doing" || role == "Review" || role == "Decompose" || role == "Rebase"
		a.Phases = append(a.Phases, Phase{ID: role, Title: title, Role: role, Agentic: agentic, Instructions: "Follow accepted scope and the phase's permission boundary."})
	}
	a.Rules = []Rule{{ID: "refine", Source: "Todo", Outcome: "requestRefinement", Target: "Refine"}, {ID: "proposal", Source: "Refine", Outcome: "success", Action: "wait"}, {ID: "refine-failed", Source: "Refine", Outcome: "failure", Target: "Todo"}, {ID: "scope", Source: "Refine", Outcome: "acceptScope", Target: "Ready"}, {ID: "plan", Source: "Refine", Outcome: "acceptPlan", Target: "Decompose"}, {ID: "start", Source: "Ready", Outcome: "start", Target: "Doing"}, {ID: "implemented", Source: "Doing", Outcome: "success", Target: "Review"}, {ID: "implementation-failed", Source: "Doing", Outcome: "failure", Target: "Ready"}, {ID: "reviewed", Source: "Review", Outcome: "success", Target: "Approval"}, {ID: "changes", Source: "Review", Outcome: "failure", Target: "Ready"}, {ID: "approve", Source: "Approval", Outcome: "approve", Target: "Done"}, {ID: "revise", Source: "Approval", Outcome: "revise", Target: "Ready"}, {ID: "rescope", Source: "Approval", Outcome: "rescope", Target: "Todo"}, {ID: "stale", Source: "Approval", Outcome: "staleBase", Target: "Rebase"}, {ID: "rebased", Source: "Rebase", Outcome: "success", Target: "Review"}, {ID: "rebase-failed", Source: "Rebase", Outcome: "failure", Target: "Ready"}, {ID: "planned", Source: "Decompose", Outcome: "success", Action: "halt"}, {ID: "planning-failed", Source: "Decompose", Outcome: "failure", Action: "halt"}}
	for _, p := range a.Phases {
		if p.Agentic {
			a.Rules = append(a.Rules, Rule{ID: "question-" + p.ID, Source: p.ID, Outcome: "question", Action: "wait"})
		}
	}
	return map[string]Workflow{"human@1": h, "agent@1": a}
}
