package model

import "time"

// AgentDefinition is a system-owned persona, not an execution grant.
type AgentDefinition struct {
	ID        string    `json:"id"`
	Version   int64     `json:"version"`
	Name      string    `json:"name"`
	Goal      string    `json:"goal"`
	Prompt    string    `json:"prompt"`
	Skills    []string  `json:"skills"`
	Harness   string    `json:"harness"`
	Model     string    `json:"model,omitempty"`
	Thinking  string    `json:"thinking,omitempty"`
	Archived  bool      `json:"archived"`
	UpdatedBy string    `json:"updatedBy"`
	UpdatedAt time.Time `json:"updatedAt"`
}
