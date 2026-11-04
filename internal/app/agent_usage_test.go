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

func TestConversationUsageCanBeRemovedWithoutLosingHistory(t *testing.T) {
	st := &model.State{Receipts: map[string]model.Receipt{}, Projects: map[int64]model.Project{1: {ID: 1, Members: map[string]string{"owner": "Maintainer", "writer": "Contributor"}}}, AgentDefinitions: map[string]model.AgentDefinition{"agent:one": {ID: "agent:one", Name: "Reviewer"}}}
	owner, writer := model.Account{ID: "owner"}, model.Account{ID: "writer"}
	in := AgentUsageInput{ProjectID: 1, Name: "Review", AgentID: "agent:one", Kind: "todo-conversation", Enabled: true, RequestID: "create"}
	raw, _ := json.Marshal(in)
	created, _, err := agentUsageCall(st, owner, "saveAgentUsage", raw)
	if err != nil {
		t.Fatal(err)
	}
	usage := created.(model.AgentUsage)
	if usage.Kind != "todo-conversation" {
		t.Fatal("conversation not retained")
	}
	remove := RemoveAgentUsageInput{ID: usage.ID, ProjectID: 1, ExpectedVersion: usage.Version, RequestID: "remove"}
	data, _ := json.Marshal(remove)
	if _, _, err = agentUsageCall(st, writer, "removeAgentUsage", data); err == nil {
		t.Fatal("contributor removed usage")
	}
	removed, changed, err := agentUsageCall(st, owner, "removeAgentUsage", data)
	if err != nil || !changed || !removed.(model.AgentUsage).Removed {
		t.Fatal("remove failed", err)
	}
	if again, changed, err := agentUsageCall(st, owner, "removeAgentUsage", data); err != nil || changed || again.(model.AgentUsage).ID != usage.ID {
		t.Fatal("remove retry was not idempotent")
	}
	if !st.AgentUsages[usage.ID].Removed {
		t.Fatal("historical usage identity was discarded")
	}
	list, _, err := agentUsageCall(st, owner, "listAgentUsages", json.RawMessage(`{"projectId":1}`))
	if err != nil || len(list.(AgentUsageList).Items) != 0 {
		t.Fatal("removed action still discoverable")
	}
	in.RequestID = "recreate"
	raw, _ = json.Marshal(in)
	if _, _, err = agentUsageCall(st, owner, "saveAgentUsage", raw); err != nil {
		t.Fatal("removed name cannot be reused", err)
	}
}

func TestAgentUsageLanePlacement(t *testing.T) {
	st := &model.State{Receipts: map[string]model.Receipt{}, Projects: map[int64]model.Project{1: {ID: 1, Members: map[string]string{"owner": "Maintainer"}}}, AgentDefinitions: map[string]model.AgentDefinition{"agent:one": {ID: "agent:one"}}, Workflows: map[string]model.Workflow{"flow@1": {ID: "flow", Version: 1, ProjectID: 1, Published: true, Phases: []model.Phase{{ID: "doing"}, {ID: "done"}}}, "foreign@1": {ProjectID: 2, Published: true, Phases: []model.Phase{{ID: "doing"}}}}}
	actor := model.Account{ID: "owner"}
	in := AgentUsageInput{ProjectID: 1, Name: "Discuss", AgentID: "agent:one", Kind: "todo-conversation", View: "Board", Workflow: "flow@1", Phase: "doing", Enabled: true, RequestID: "create"}
	call := func() (any, bool, error) {
		raw, _ := json.Marshal(in)
		return agentUsageCall(st, actor, "saveAgentUsage", raw)
	}
	result, _, err := call()
	if err != nil {
		t.Fatal(err)
	}
	u := result.(model.AgentUsage)
	if u.View != "Board" || !usageMatchesTicket(u, model.Ticket{Workflow: "flow@1", Phase: "doing"}) || usageMatchesTicket(u, model.Ticket{Workflow: "flow@1", Phase: "done"}) || usageMatchesTicket(u, model.Ticket{Workflow: "flow@2", Phase: "doing"}) {
		t.Fatal("placement was not enforced")
	}
	in.RequestID, in.Workflow = "foreign", "foreign@1"
	if _, _, err = call(); err == nil {
		t.Fatal("foreign workflow accepted")
	}
	in.RequestID, in.Workflow, in.Phase = "missing", "flow@1", "missing"
	if _, _, err = call(); err == nil {
		t.Fatal("unknown lane accepted")
	}
}
