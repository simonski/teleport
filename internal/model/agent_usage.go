package model

import "time"

// AgentUsage binds a system persona to a project-owned interaction.
type AgentUsage struct {
	ID        string    `json:"id"`
	ProjectID int64     `json:"projectId"`
	Version   int64     `json:"version"`
	Name      string    `json:"name"`
	AgentID   string    `json:"agentId"`
	Kind      string    `json:"kind"`
	View      string    `json:"view,omitempty"`
	Workflow  string    `json:"workflow,omitempty"`
	Phase     string    `json:"phase,omitempty"`
	Enabled   bool      `json:"enabled"`
	Removed   bool      `json:"removed,omitempty"`
	UpdatedBy string    `json:"updatedBy"`
	UpdatedAt time.Time `json:"updatedAt"`
}
