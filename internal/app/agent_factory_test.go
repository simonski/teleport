package app

import (
	"encoding/json"
	"testing"

	"teleport/internal/model"
)

func TestAgentFactoryAuthorizationVersionAndProjectDiscovery(t *testing.T) {
	admin := model.Account{ID: "admin", Administrator: true}
	member := model.Account{ID: "member"}
	outsider := model.Account{ID: "outside"}
	st := &model.State{Receipts: map[string]model.Receipt{}, Projects: map[int64]model.Project{1: {ID: 1, Members: map[string]string{"member": "Contributor"}}}}
	input := SaveAgentDefinitionInput{Name: "Reviewer", Goal: "Review work", Prompt: "Ask questions", Harness: "codex", RequestID: "create"}
	raw, _ := json.Marshal(input)
	if _, _, err := agentFactoryCall(st, member, "saveAgentDefinition", raw); err == nil {
		t.Fatal("nonadministrator saved an agent")
	}
	result, changed, err := agentFactoryCall(st, admin, "saveAgentDefinition", raw)
	if err != nil || !changed {
		t.Fatalf("create: %v", err)
	}
	row := result.(model.AgentDefinition)
	duplicate, changed, err := agentFactoryCall(st, admin, "saveAgentDefinition", raw)
	if err != nil || changed || duplicate.(model.AgentDefinition).ID != row.ID {
		t.Fatal("retry duplicated agent")
	}
	input.ID = row.ID
	input.ExpectedVersion = 99
	input.RequestID = "stale"
	raw, _ = json.Marshal(input)
	if _, _, err = agentFactoryCall(st, admin, "saveAgentDefinition", raw); err == nil {
		t.Fatal("stale update accepted")
	}
	scope := json.RawMessage(`{"projectId":1}`)
	if _, _, err = agentFactoryCall(st, outsider, "listProjectAgents", scope); err == nil {
		t.Fatal("outsider saw agents")
	}
	listed, _, err := agentFactoryCall(st, member, "listProjectAgents", scope)
	if err != nil {
		t.Fatal(err)
	}
	visible := listed.(AgentDefinitionPage).Items
	if len(visible) != 1 || visible[0].ID != row.ID || visible[0].Prompt != "" || visible[0].Goal != "" {
		t.Fatal("project list exposed private definition")
	}
	if _, _, err = agentFactoryCall(st, member, "listAgentDefinitions", json.RawMessage(`{}`)); err == nil {
		t.Fatal("nonadministrator read factory definitions")
	}
}
