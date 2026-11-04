package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

type AgentModelChoice struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Thinking []string `json:"thinking"`
}
type AgentHarnessInventory struct {
	ID     string             `json:"id"`
	Models []AgentModelChoice `json:"models"`
	Source string             `json:"source"`
}
type AgentInventory struct {
	Items []AgentHarnessInventory `json:"items"`
}

func agentInventory() AgentInventory {
	out := AgentInventory{Items: []AgentHarnessInventory{}}
	if _, err := exec.LookPath("codex"); err == nil {
		entry := AgentHarnessInventory{ID: "codex", Models: []AgentModelChoice{}, Source: "No local model inventory available"}
		home, _ := os.UserHomeDir()
		root := os.Getenv("CODEX_HOME")
		if root == "" {
			root = filepath.Join(home, ".codex")
		}
		path := filepath.Join(root, "models_cache.json")
		if info, err := os.Stat(path); err == nil && info.Size() <= 4<<20 {
			if data, err := os.ReadFile(path); err == nil {
				var cache struct {
					Models []struct {
						Slug        string `json:"slug"`
						DisplayName string `json:"display_name"`
						Visibility  string `json:"visibility"`
						Levels      []struct {
							Effort string `json:"effort"`
						} `json:"supported_reasoning_levels"`
					} `json:"models"`
				}
				if json.Unmarshal(data, &cache) == nil {
					for _, m := range cache.Models {
						if m.Visibility != "list" || m.Slug == "" || len(m.Slug) > 100 {
							continue
						}
						levels := []string{}
						for _, l := range m.Levels {
							if contains([]string{"low", "medium", "high", "xhigh", "max", "ultra"}, l.Effort) {
								levels = append(levels, l.Effort)
							}
						}
						entry.Models = append(entry.Models, AgentModelChoice{ID: m.Slug, Title: m.DisplayName, Thinking: levels})
						if len(entry.Models) >= 100 {
							break
						}
					}
					entry.Source = "Installed Codex model cache"
				}
			}
		}
		out.Items = append(out.Items, entry)
	}
	if _, err := exec.LookPath("claude"); err == nil {
		out.Items = append(out.Items, AgentHarnessInventory{ID: "claude", Source: "Claude CLI model aliases; availability depends on the account", Models: []AgentModelChoice{{ID: "sonnet", Title: "Sonnet", Thinking: []string{"low", "medium", "high", "xhigh", "max"}}, {ID: "opus", Title: "Opus", Thinking: []string{"low", "medium", "high", "xhigh", "max"}}}})
	}
	return out
}
