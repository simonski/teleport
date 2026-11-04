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
	Enabled   bool      `json:"enabled"`
	UpdatedBy string    `json:"updatedBy"`
	UpdatedAt time.Time `json:"updatedAt"`
}
