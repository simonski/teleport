package app

import (
	"encoding/json"
	"testing"

	"teleport/internal/model"
)

func TestAgentUsageProjectAuthorizationAndVersion(t *testing.T) {
	st := &model.State{Receipts: map[string]model.Receipt{}, Projects: map[int64]model.Project{1: {ID: 1, Members: map[string]string{"owner": "Maintainer", "writer": "Contributor"}}, 2: {ID: 2, Members: map[string]string{"other": "Maintainer"}}}, AgentDefinitions: map[string]model.AgentDefinition{"agent:one": {ID: "agent:one", Name: "Spellchecker"}}}
	owner, writer, other := model.Account{ID: "owner"}, model.Account{ID: "writer"}, model.Account{ID: "other"}
	in := AgentUsageInput{ProjectID: 1, Name: "Spellcheck", AgentID: "agent:one", Enabled: true, RequestID: "create"}
	encode := func() json.RawMessage { data, _ := json.Marshal(in); return data }
	if _, _, err := agentUsageCall(st, writer, "saveAgentUsage", encode()); err == nil {
		t.Fatal("contributor configured usage")
	}
	result, changed, err := agentUsageCall(st, owner, "saveAgentUsage", encode())
	if err != nil || !changed {
		t.Fatalf("create: %v", err)
	}
	row := result.(model.AgentUsage)
	duplicate, changed, err := agentUsageCall(st, owner, "saveAgentUsage", encode())
	if err != nil || changed || duplicate.(model.AgentUsage).ID != row.ID {
		t.Fatal("retry duplicated usage")
	}
	if _, _, err := agentUsageCall(st, other, "listAgentUsages", json.RawMessage(`{"projectId":1}`)); err == nil {
		t.Fatal("other project accessed usage")
	}
	list, _, err := agentUsageCall(st, writer, "listAgentUsages", json.RawMessage(`{"projectId":1}`))
	if err != nil || len(list.(AgentUsageList).Items) != 1 {
		t.Fatal("project contributor cannot discover usage")
	}
	in.ID, in.ExpectedVersion, in.RequestID = row.ID, 99, "stale"
	if _, _, err := agentUsageCall(st, owner, "saveAgentUsage", encode()); err == nil {
		t.Fatal("stale usage update accepted")
	}
	in.ProjectID, in.ExpectedVersion, in.RequestID = 2, row.Version, "cross-project"
	if _, _, err := agentUsageCall(st, other, "saveAgentUsage", encode()); err == nil {
		t.Fatal("usage moved between projects")
	}
}
