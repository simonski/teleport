package model

import "time"

const SchemaVersion = 1

type Account struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	PasswordHash  string `json:"passwordHash,omitempty"`
	Administrator bool   `json:"administrator"`
	Suspended     bool   `json:"suspended"`
}
type Session struct {
	AccountID string    `json:"accountId"`
	Expires   time.Time `json:"expires"`
}
type Settings struct {
	Agents        bool `json:"agents"`
	Registrations bool `json:"registrations"`
	Handbrake     bool `json:"handbrake"`
}
type Phase struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Role         string `json:"role"`
	Agentic      bool   `json:"agentic"`
	Instructions string `json:"instructions"`
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
	Schema         int                  `json:"schema"`
	InstallationID string               `json:"installationId"`
	Revision       int64                `json:"revision"`
	Accounts       map[string]Account   `json:"accounts"`
	Sessions       map[string]Session   `json:"sessions"`
	Settings       Settings             `json:"settings"`
	Workflows      map[string]Workflow  `json:"workflows"`
	NextTicketID   int64                `json:"nextTicketId"`
	NextProjectID  int64                `json:"nextProjectId"`
	Projects       map[int64]Project    `json:"projects"`
	Tickets        map[int64]Ticket     `json:"tickets"`
	History        map[int64][]Revision `json:"history"`
	Receipts       map[string]Receipt   `json:"receipts"`
}

type Project struct {
	ID             int64             `json:"id"`
	Version        int64             `json:"version"`
	Key            string            `json:"key"`
	Name           string            `json:"name"`
	Creator        string            `json:"creator"`
	Members        map[string]string `json:"members"`
	UnassignedGoal int64             `json:"unassignedGoal"`
	UnassignedEpic int64             `json:"unassignedEpic"`
	HumanWorkflow  string            `json:"humanWorkflow"`
	AgentWorkflow  string            `json:"agentWorkflow"`
	Handbrake      bool              `json:"handbrake"`
}
type Agreement struct {
	Kind         string    `json:"kind"`
	Actor        string    `json:"actor"`
	At           time.Time `json:"at"`
	Criteria     string    `json:"criteria"`
	BasisVersion int64     `json:"basisVersion"`
}
type Ticket struct {
	ID                 int64       `json:"id"`
	Version            int64       `json:"version"`
	ProjectID          int64       `json:"projectId"`
	ParentID           int64       `json:"parentId"`
	Kind               string      `json:"kind"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	Criteria           string      `json:"criteria"`
	Status             string      `json:"status"`
	Phase              string      `json:"phase"`
	Workflow           string      `json:"workflow"`
	Agent              bool        `json:"agent"`
	Assignee           string      `json:"assignee,omitempty"`
	Creator            string      `json:"creator"`
	Priority           string      `json:"priority"`
	Labels             []string    `json:"labels"`
	Start              string      `json:"start,omitempty"`
	ETA                string      `json:"eta,omitempty"`
	Due                string      `json:"due,omitempty"`
	Progress           int         `json:"progress"`
	Archived           bool        `json:"archived"`
	Deleted            bool        `json:"deleted"`
	Protected          bool        `json:"protected"`
	Planning           bool        `json:"planning"`
	Active             bool        `json:"active"`
	Claim              string      `json:"claim,omitempty"`
	Execution          string      `json:"execution,omitempty"`
	StopRequested      bool        `json:"stopRequested"`
	ResumeHold         bool        `json:"resumeHold"`
	CancellationBatch  string      `json:"cancellationBatch,omitempty"`
	CancellationReason string      `json:"cancellationReason,omitempty"`
	Dependencies       []int64     `json:"dependencies"`
	Agreements         []Agreement `json:"agreements"`
	CreatedAt          time.Time   `json:"createdAt"`
	UpdatedAt          time.Time   `json:"updatedAt"`
}
type Revision struct {
	Version  int64     `json:"version"`
	Actor    string    `json:"actor"`
	Action   string    `json:"action"`
	At       time.Time `json:"at"`
	Snapshot Ticket    `json:"snapshot"`
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
